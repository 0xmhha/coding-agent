package ckv

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"

	_ "modernc.org/sqlite" // CGo-free SQLite driver (RI-07: brute-force MVP)
)

// Store persists CodeChunks + their embeddings in a single SQLite database.
//
// Vector search uses brute-force cosine similarity over BLOB-encoded float32
// vectors (RI-07: sqlite-vss is not CGo-free, and the project size
// — ~20k chunks — fits comfortably in memory for linear scan).
type Store struct {
	db   *sql.DB
	path string
}

// Open creates or opens the SQLite database at the given path. The parent
// directory must already exist.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("ckv: store path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Enable WAL and reasonable timeouts via DSN.
	dsn := "file:" + abs + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("ckv: open %s: %w", abs, err)
	}
	s := &Store{db: db, path: abs}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the database file location.
func (s *Store) Path() string { return s.path }

func (s *Store) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS chunks (
			id            TEXT PRIMARY KEY,
			file_path     TEXT NOT NULL,
			package_name  TEXT NOT NULL,
			symbol_name   TEXT NOT NULL,
			symbol_type   TEXT NOT NULL,
			code          TEXT NOT NULL,
			signature     TEXT,
			godoc         TEXT,
			start_line    INTEGER NOT NULL,
			end_line      INTEGER NOT NULL,
			receiver_type TEXT,
			params        TEXT,
			returns       TEXT,
			imports       TEXT,
			code_hash     TEXT NOT NULL,
			indexed_at    TEXT NOT NULL,
			git_modified  TEXT,
			git_author    TEXT,
			embedding     BLOB,
			embed_dim     INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_package ON chunks(package_name)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_file    ON chunks(file_path)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_symbol  ON chunks(symbol_type)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_hash    ON chunks(code_hash)`,
		`CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("ckv: schema init: %w", err)
		}
	}
	return nil
}

// Upsert inserts or replaces a chunk and its embedding.
// vector may be nil when running in BM25 fallback mode (RI-08).
func (s *Store) Upsert(ctx context.Context, c types.CodeChunk, vector []float32) error {
	paramsJSON, _ := json.Marshal(c.Params)
	returnsJSON, _ := json.Marshal(c.Returns)
	importsJSON, _ := json.Marshal(c.Imports)
	embedding := encodeVector(vector)
	dim := len(vector)
	indexedAt := c.IndexedAt
	if indexedAt.IsZero() {
		indexedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chunks (
			id, file_path, package_name, symbol_name, symbol_type, code,
			signature, godoc, start_line, end_line, receiver_type,
			params, returns, imports, code_hash, indexed_at,
			git_modified, git_author, embedding, embed_dim
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			file_path     = excluded.file_path,
			package_name  = excluded.package_name,
			symbol_name   = excluded.symbol_name,
			symbol_type   = excluded.symbol_type,
			code          = excluded.code,
			signature     = excluded.signature,
			godoc         = excluded.godoc,
			start_line    = excluded.start_line,
			end_line      = excluded.end_line,
			receiver_type = excluded.receiver_type,
			params        = excluded.params,
			returns       = excluded.returns,
			imports       = excluded.imports,
			code_hash     = excluded.code_hash,
			indexed_at    = excluded.indexed_at,
			git_modified  = excluded.git_modified,
			git_author    = excluded.git_author,
			embedding     = excluded.embedding,
			embed_dim     = excluded.embed_dim
	`,
		c.ID, c.FilePath, c.PackageName, c.SymbolName, string(c.SymbolType), c.Code,
		c.Signature, c.Godoc, c.StartLine, c.EndLine, c.ReceiverType,
		string(paramsJSON), string(returnsJSON), string(importsJSON),
		c.CodeHash, indexedAt.UTC().Format(time.RFC3339Nano),
		c.GitModified, c.GitAuthor, embedding, dim,
	)
	if err != nil {
		return fmt.Errorf("ckv: upsert %s: %w", c.ID, err)
	}
	return nil
}

// GetCodeHash returns the stored code_hash for the given chunk id, or "" if
// the chunk does not exist. Used by the indexer to skip unchanged chunks (RI-23).
func (s *Store) GetCodeHash(ctx context.Context, chunkID string) (string, error) {
	var h string
	err := s.db.QueryRowContext(ctx,
		`SELECT code_hash FROM chunks WHERE id = ?`, chunkID,
	).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return h, nil
}

// DeleteByFile removes all chunks belonging to the given file.
func (s *Store) DeleteByFile(ctx context.Context, filePath string) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM chunks WHERE file_path = ?`, filePath)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteByIDs removes specific chunks. Used when re-parsing a file shrinks the chunk set.
func (s *Store) DeleteByIDs(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM chunks WHERE id = ?`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	total := 0
	for _, id := range ids {
		res, err := stmt.ExecContext(ctx, id)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, tx.Commit()
}

// ListIDsByFile returns the chunk ids currently stored for a file.
func (s *Store) ListIDsByFile(ctx context.Context, filePath string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM chunks WHERE file_path = ?`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Count returns the total number of chunks.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&n)
	return n, err
}

// SetMeta upserts a meta value.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetMeta returns the meta value, or "" when missing.
func (s *Store) GetMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// VectorSearch returns the top-k chunks ranked by cosine similarity against
// query. filters trim the candidate set up-front to avoid scanning everything
// for narrow queries.
func (s *Store) VectorSearch(
	ctx context.Context, query []float32, topK int, f types.SearchFilters,
) ([]types.SearchResult, error) {
	if len(query) == 0 {
		return nil, errors.New("ckv: query vector is empty")
	}
	rows, err := s.queryFiltered(ctx, f)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		row    chunkRow
		vector []float32
		score  float64
	}
	queryNorm := norm(query)
	if queryNorm == 0 {
		return nil, nil
	}

	results := make([]scored, 0, 256)
	for rows.Next() {
		row, vec, err := scanChunkRow(rows)
		if err != nil {
			return nil, err
		}
		if len(vec) == 0 || len(vec) != len(query) {
			continue
		}
		s := cosineSimilarity(query, vec, queryNorm)
		results = append(results, scored{row: row, vector: vec, score: s})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(results, func(i, j int) bool { return results[i].score > results[j].score })
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}

	out := make([]types.SearchResult, 0, len(results))
	for _, r := range results {
		out = append(out, toSearchResult(r.row, r.score))
	}
	return out, nil
}

// AllForBM25 streams every chunk's lexical fields (signature + godoc + code).
// Used by the BM25 fallback when no embedder is available.
func (s *Store) AllForBM25(ctx context.Context, f types.SearchFilters) ([]chunkRow, error) {
	rows, err := s.queryFiltered(ctx, f)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []chunkRow{}
	for rows.Next() {
		row, _, err := scanChunkRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ToSearchResult is exported for the BM25 path so it can score lexically and
// then build the same response shape.
func ToSearchResult(row chunkRow, score float64) types.SearchResult {
	return toSearchResult(row, score)
}

// --- internal row scanning ---

type chunkRow struct {
	ID, FilePath, PackageName, SymbolName, SymbolType string
	Code, Signature, Godoc, ReceiverType              string
	StartLine, EndLine                                int
	GitModified, GitAuthor                            string
	Imports                                           []string
}

func (s *Store) queryFiltered(ctx context.Context, f types.SearchFilters) (*sql.Rows, error) {
	q := `SELECT id, file_path, package_name, symbol_name, symbol_type, code,
		signature, godoc, start_line, end_line, receiver_type,
		imports, git_modified, git_author, embedding, embed_dim
		FROM chunks WHERE 1=1`
	args := []any{}
	if f.Package != "" {
		q += " AND package_name = ?"
		args = append(args, f.Package)
	}
	if f.FilePattern != "" {
		q += " AND file_path LIKE ?"
		args = append(args, f.FilePattern)
	}
	if f.SymbolType != "" {
		q += " AND symbol_type = ?"
		args = append(args, f.SymbolType)
	}
	if f.ModifiedSince != "" {
		q += " AND (git_modified >= ? OR indexed_at >= ?)"
		args = append(args, f.ModifiedSince, f.ModifiedSince)
	}
	return s.db.QueryContext(ctx, q, args...)
}

func scanChunkRow(rows *sql.Rows) (chunkRow, []float32, error) {
	var (
		row              chunkRow
		importsJSON      string
		embedding        []byte
		dim              int
	)
	if err := rows.Scan(
		&row.ID, &row.FilePath, &row.PackageName, &row.SymbolName, &row.SymbolType,
		&row.Code, &row.Signature, &row.Godoc, &row.StartLine, &row.EndLine,
		&row.ReceiverType, &importsJSON, &row.GitModified, &row.GitAuthor,
		&embedding, &dim,
	); err != nil {
		return row, nil, err
	}
	if importsJSON != "" {
		_ = json.Unmarshal([]byte(importsJSON), &row.Imports)
	}
	vec := decodeVector(embedding, dim)
	return row, vec, nil
}

func toSearchResult(row chunkRow, score float64) types.SearchResult {
	return types.SearchResult{
		FilePath:   row.FilePath,
		Package:    row.PackageName,
		Symbol:     row.SymbolName,
		SymbolType: row.SymbolType,
		Signature:  row.Signature,
		Snippet:    truncate(row.Code, 500),
		Godoc:      row.Godoc,
		Score:      score,
		StartLine:  row.StartLine,
		EndLine:    row.EndLine,
		Imports:    row.Imports,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// --- vector encoding & math ---

func encodeVector(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func decodeVector(b []byte, dim int) []float32 {
	if dim <= 0 || len(b) < dim*4 {
		return nil
	}
	out := make([]float32, dim)
	for i := 0; i < dim; i++ {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		out[i] = math.Float32frombits(bits)
	}
	return out
}

func norm(v []float32) float64 {
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	return math.Sqrt(sum)
}

func cosineSimilarity(a, b []float32, normA float64) float64 {
	if len(a) != len(b) || normA == 0 {
		return 0
	}
	var dot, sumB float64
	for i, av := range a {
		bv := float64(b[i])
		dot += float64(av) * bv
		sumB += bv * bv
	}
	if sumB == 0 {
		return 0
	}
	return dot / (normA * math.Sqrt(sumB))
}
