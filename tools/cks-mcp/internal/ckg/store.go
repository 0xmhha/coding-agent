// Package ckg implements the Code Knowledge Graph subsystem of cks-mcp:
// relation extraction, git history, concurrency analysis, and graph traversal.
package ckg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"

	_ "modernc.org/sqlite" // CGo-free SQLite driver
)

// Store persists GraphNode, GraphEdge, SymbolHistoryEntry, and ConcurrencyContext.
// CKG shares the SQLite file with CKV when given the same path (single index).
type Store struct {
	db   *sql.DB
	path string
}

// Open creates or opens the SQLite database at the given path. The parent
// directory must already exist. Schema is created on first use.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("ckg: store path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := "file:" + abs + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("ckg: open %s: %w", abs, err)
	}
	s := &Store{db: db, path: abs}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// FromExistingDB lets the caller reuse an *sql.DB owned elsewhere (e.g., CKV's
// store sharing the same file). The schema is still ensured on first use.
func FromExistingDB(db *sql.DB, path string) (*Store, error) {
	s := &Store{db: db, path: path}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the database file location.
func (s *Store) Path() string { return s.path }

func (s *Store) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS graph_nodes (
			id             TEXT PRIMARY KEY,
			file_path      TEXT NOT NULL,
			package_name   TEXT NOT NULL,
			symbol_name    TEXT NOT NULL,
			symbol_type    TEXT NOT NULL,
			qualified_name TEXT NOT NULL,
			signature      TEXT,
			code_snippet   TEXT,
			start_line     INTEGER NOT NULL,
			end_line       INTEGER NOT NULL,
			indexed_at     TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_package   ON graph_nodes(package_name)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_qualified ON graph_nodes(qualified_name)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_file      ON graph_nodes(file_path)`,
		`CREATE TABLE IF NOT EXISTS graph_edges (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			from_node     TEXT NOT NULL,
			to_node       TEXT NOT NULL,
			relation_type TEXT NOT NULL,
			confidence    TEXT NOT NULL,
			metadata      TEXT,
			UNIQUE(from_node, to_node, relation_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_from ON graph_edges(from_node)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_to   ON graph_edges(to_node)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_type ON graph_edges(relation_type)`,
		`CREATE TABLE IF NOT EXISTS symbol_history (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id        TEXT NOT NULL,
			commit_hash    TEXT NOT NULL,
			commit_message TEXT,
			commit_date    TEXT,
			author         TEXT,
			diff_summary   TEXT,
			change_type    TEXT,
			UNIQUE(node_id, commit_hash)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_history_node ON symbol_history(node_id)`,
		`CREATE TABLE IF NOT EXISTS concurrency_context (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id             TEXT NOT NULL UNIQUE,
			goroutine_context   TEXT,
			shared_resources    TEXT,
			channel_operations  TEXT,
			sync_mechanisms     TEXT,
			risk_assessment     TEXT,
			confidence          TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_concurrency_node ON concurrency_context(node_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("ckg: schema init: %w", err)
		}
	}
	return nil
}

// --- Node operations ---

// UpsertNode inserts or replaces a node.
func (s *Store) UpsertNode(ctx context.Context, n types.GraphNode) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO graph_nodes (
			id, file_path, package_name, symbol_name, symbol_type,
			qualified_name, signature, code_snippet, start_line, end_line, indexed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			file_path = excluded.file_path,
			package_name = excluded.package_name,
			symbol_name = excluded.symbol_name,
			symbol_type = excluded.symbol_type,
			qualified_name = excluded.qualified_name,
			signature = excluded.signature,
			code_snippet = excluded.code_snippet,
			start_line = excluded.start_line,
			end_line = excluded.end_line,
			indexed_at = excluded.indexed_at
	`,
		n.ID, n.FilePath, n.PackageName, n.SymbolName, string(n.SymbolType),
		n.QualifiedName, n.Signature, n.CodeSnippet, n.StartLine, n.EndLine,
		n.IndexedAt.UTC().Format("2006-01-02T15:04:05Z"),
	)
	if err != nil {
		return fmt.Errorf("ckg: upsert node %s: %w", n.ID, err)
	}
	return nil
}

// UpsertEdge inserts an edge or returns silently if (from, to, relation) exists.
func (s *Store) UpsertEdge(ctx context.Context, e types.GraphEdge) error {
	metaJSON := ""
	if len(e.Metadata) > 0 {
		b, err := json.Marshal(e.Metadata)
		if err == nil {
			metaJSON = string(b)
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO graph_edges (from_node, to_node, relation_type, confidence, metadata)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(from_node, to_node, relation_type) DO UPDATE SET
			confidence = excluded.confidence,
			metadata   = excluded.metadata
	`, e.FromNode, e.ToNode, string(e.RelationType), string(e.Confidence), metaJSON)
	if err != nil {
		return fmt.Errorf("ckg: upsert edge %s->%s/%s: %w",
			e.FromNode, e.ToNode, e.RelationType, err)
	}
	return nil
}

// DeleteEdgesFromNode removes all outgoing edges from node; used when a file
// is re-indexed so stale edges don't accumulate.
func (s *Store) DeleteEdgesFromNode(ctx context.Context, nodeID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM graph_edges WHERE from_node = ?`, nodeID)
	return err
}

// DeleteNodesByFile removes nodes and their outgoing edges for a file.
func (s *Store) DeleteNodesByFile(ctx context.Context, filePath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT id FROM graph_nodes WHERE file_path = ?`, filePath)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()

	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM graph_edges WHERE from_node = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM symbol_history WHERE node_id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM concurrency_context WHERE node_id = ?`, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM graph_nodes WHERE file_path = ?`, filePath); err != nil {
		return err
	}
	return tx.Commit()
}

// NodeByQualifiedName resolves a fully qualified symbol name to its node id.
// Returns "" if not found. Used by ckg_query to seed traversals.
func (s *Store) NodeByQualifiedName(ctx context.Context, qname string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM graph_nodes WHERE qualified_name = ? LIMIT 1`, qname).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// NodesByLikeName performs a LIKE search; used as a fallback for short names.
func (s *Store) NodesByLikeName(ctx context.Context, pattern string, limit int) ([]types.GraphNode, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, file_path, package_name, symbol_name, symbol_type,
			qualified_name, signature, code_snippet, start_line, end_line, indexed_at
		FROM graph_nodes
		WHERE symbol_name LIKE ? OR qualified_name LIKE ?
		ORDER BY symbol_name
		LIMIT ?
	`, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

// GetNodes returns the nodes by id, preserving the input order.
func (s *Store) GetNodes(ctx context.Context, ids []string) ([]types.GraphNode, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	q := fmt.Sprintf(`
		SELECT id, file_path, package_name, symbol_name, symbol_type,
			qualified_name, signature, code_snippet, start_line, end_line, indexed_at
		FROM graph_nodes WHERE id IN (%s)
	`, placeholders)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanNodes(rows)
	if err != nil {
		return nil, err
	}
	// Preserve caller's order so traversal depth/order is stable.
	byID := make(map[string]types.GraphNode, len(out))
	for _, n := range out {
		byID[n.ID] = n
	}
	ordered := make([]types.GraphNode, 0, len(ids))
	for _, id := range ids {
		if n, ok := byID[id]; ok {
			ordered = append(ordered, n)
		}
	}
	return ordered, nil
}

// EdgesByFromNodes lists outgoing edges for a set of nodes, optionally
// filtered by relation type.
func (s *Store) EdgesByFromNodes(ctx context.Context, ids []string, relTypes []string) ([]types.GraphEdge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	q := fmt.Sprintf(`
		SELECT from_node, to_node, relation_type, confidence, metadata
		FROM graph_edges WHERE from_node IN (%s)
	`, placeholders)
	args := make([]any, 0, len(ids)+len(relTypes))
	for _, id := range ids {
		args = append(args, id)
	}
	if len(relTypes) > 0 {
		rp := strings.Repeat("?,", len(relTypes))
		rp = strings.TrimSuffix(rp, ",")
		q += fmt.Sprintf(" AND relation_type IN (%s)", rp)
		for _, rt := range relTypes {
			args = append(args, rt)
		}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// EdgesByToNodes is the reverse direction used by ckg_impact (callers).
func (s *Store) EdgesByToNodes(ctx context.Context, ids []string, relTypes []string) ([]types.GraphEdge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	q := fmt.Sprintf(`
		SELECT from_node, to_node, relation_type, confidence, metadata
		FROM graph_edges WHERE to_node IN (%s)
	`, placeholders)
	args := make([]any, 0, len(ids)+len(relTypes))
	for _, id := range ids {
		args = append(args, id)
	}
	if len(relTypes) > 0 {
		rp := strings.Repeat("?,", len(relTypes))
		rp = strings.TrimSuffix(rp, ",")
		q += fmt.Sprintf(" AND relation_type IN (%s)", rp)
		for _, rt := range relTypes {
			args = append(args, rt)
		}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// --- History / Concurrency ---

// AppendHistory inserts a symbol_history entry (idempotent on (node_id, hash)).
func (s *Store) AppendHistory(ctx context.Context, h types.SymbolHistoryEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO symbol_history
			(node_id, commit_hash, commit_message, commit_date, author, diff_summary, change_type)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, h.NodeID, h.CommitHash, h.CommitMessage, h.CommitDate, h.Author, h.DiffSummary, h.ChangeType)
	return err
}

// HistoryForNode returns the recent history entries for a node, newest first.
func (s *Store) HistoryForNode(ctx context.Context, nodeID string, limit int) ([]types.SymbolHistoryEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT node_id, commit_hash, commit_message, commit_date, author, diff_summary, change_type
		FROM symbol_history WHERE node_id = ?
		ORDER BY commit_date DESC LIMIT ?
	`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.SymbolHistoryEntry
	for rows.Next() {
		var h types.SymbolHistoryEntry
		if err := rows.Scan(&h.NodeID, &h.CommitHash, &h.CommitMessage,
			&h.CommitDate, &h.Author, &h.DiffSummary, &h.ChangeType); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// UpsertConcurrencyContext replaces the concurrency record for a node.
func (s *Store) UpsertConcurrencyContext(ctx context.Context, cc types.ConcurrencyContext) error {
	grJSON, _ := json.Marshal(cc.GoroutineContext)
	srJSON, _ := json.Marshal(cc.SharedResources)
	chJSON, _ := json.Marshal(cc.ChannelOperations)
	smJSON, _ := json.Marshal(cc.SyncMechanisms)
	riskJSON, _ := json.Marshal(cc.Risk)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO concurrency_context
			(node_id, goroutine_context, shared_resources, channel_operations,
			 sync_mechanisms, risk_assessment, confidence)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			goroutine_context = excluded.goroutine_context,
			shared_resources = excluded.shared_resources,
			channel_operations = excluded.channel_operations,
			sync_mechanisms = excluded.sync_mechanisms,
			risk_assessment = excluded.risk_assessment,
			confidence = excluded.confidence
	`, cc.NodeID, string(grJSON), string(srJSON), string(chJSON),
		string(smJSON), string(riskJSON), string(cc.Confidence))
	return err
}

// ConcurrencyForNode returns the concurrency context for a node or zero value.
func (s *Store) ConcurrencyForNode(ctx context.Context, nodeID string) (types.ConcurrencyContext, error) {
	var cc types.ConcurrencyContext
	var grJSON, srJSON, chJSON, smJSON, riskJSON, conf string
	err := s.db.QueryRowContext(ctx, `
		SELECT goroutine_context, shared_resources, channel_operations,
			sync_mechanisms, risk_assessment, confidence
		FROM concurrency_context WHERE node_id = ?
	`, nodeID).Scan(&grJSON, &srJSON, &chJSON, &smJSON, &riskJSON, &conf)
	if errors.Is(err, sql.ErrNoRows) {
		return cc, nil
	}
	if err != nil {
		return cc, err
	}
	cc.NodeID = nodeID
	cc.Confidence = types.ConfidenceLevel(conf)
	_ = json.Unmarshal([]byte(grJSON), &cc.GoroutineContext)
	_ = json.Unmarshal([]byte(srJSON), &cc.SharedResources)
	_ = json.Unmarshal([]byte(chJSON), &cc.ChannelOperations)
	_ = json.Unmarshal([]byte(smJSON), &cc.SyncMechanisms)
	_ = json.Unmarshal([]byte(riskJSON), &cc.Risk)
	return cc, nil
}

// --- Counts (for diagnostics + tests) ---

// CountNodes returns the total node count.
func (s *Store) CountNodes(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_nodes`).Scan(&n)
	return n, err
}

// CountEdges returns the total edge count.
func (s *Store) CountEdges(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_edges`).Scan(&n)
	return n, err
}

// --- internal scanners ---

func scanNodes(rows *sql.Rows) ([]types.GraphNode, error) {
	var out []types.GraphNode
	for rows.Next() {
		var n types.GraphNode
		var indexedAt, symbolType string
		if err := rows.Scan(
			&n.ID, &n.FilePath, &n.PackageName, &n.SymbolName, &symbolType,
			&n.QualifiedName, &n.Signature, &n.CodeSnippet,
			&n.StartLine, &n.EndLine, &indexedAt,
		); err != nil {
			return nil, err
		}
		n.SymbolType = types.SymbolType(symbolType)
		_ = indexedAt // RFC3339 parsing is the consumer's job
		out = append(out, n)
	}
	return out, rows.Err()
}

func scanEdges(rows *sql.Rows) ([]types.GraphEdge, error) {
	var out []types.GraphEdge
	for rows.Next() {
		var e types.GraphEdge
		var rt, conf, metaJSON string
		if err := rows.Scan(&e.FromNode, &e.ToNode, &rt, &conf, &metaJSON); err != nil {
			return nil, err
		}
		e.RelationType = types.RelationType(rt)
		e.Confidence = types.ConfidenceLevel(conf)
		if metaJSON != "" {
			_ = json.Unmarshal([]byte(metaJSON), &e.Metadata)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
