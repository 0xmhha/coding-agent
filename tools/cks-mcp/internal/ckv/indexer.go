package ckv

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// IndexMode controls how the indexer walks the project.
type IndexMode string

const (
	ModeFull        IndexMode = "full"
	ModeIncremental IndexMode = "incremental"
)

// IndexRequest is the structured input to the indexer.
type IndexRequest struct {
	Mode       IndexMode
	ProjectDir string
	// Modules optionally restricts a "full" run to the listed top-level dirs
	// so callers can prioritize the tickets's scope.modules (RI-09).
	Modules []string
	// SinceCommit is required for incremental runs. When empty in incremental
	// mode the indexer falls back to the previously-recorded index_commit.
	SinceCommit string
}

// ProgressFunc reports progress while indexing. processed grows monotonically;
// total may be 0 when not knowable up front (e.g., for incremental).
type ProgressFunc func(processed, total int, currentFile string)

// Indexer drives parsing + embedding + storage. It is the only writer to the
// store during an indexing run.
type Indexer struct {
	store    *Store
	embedder Embedder // may be nil — RI-08 fallback: index without vectors
	Progress ProgressFunc
	// ChunkOpts overrides the default chunker options.
	ChunkOpts ChunkOptions
}

// NewIndexer constructs an indexer. embedder may be nil.
func NewIndexer(store *Store, embedder Embedder) *Indexer {
	return &Indexer{
		store:     store,
		embedder:  embedder,
		ChunkOpts: DefaultOptions(),
	}
}

// Run executes the indexing pipeline and returns aggregate statistics.
func (idx *Indexer) Run(ctx context.Context, req IndexRequest) (types.IndexStats, error) {
	start := time.Now()
	stats := types.IndexStats{}

	if req.ProjectDir == "" {
		return stats, fmt.Errorf("ckv: project_dir is required")
	}
	absDir, err := filepath.Abs(req.ProjectDir)
	if err != nil {
		return stats, err
	}

	files, err := idx.collectFiles(ctx, absDir, req)
	if err != nil {
		return stats, err
	}
	total := len(files)

	for i, file := range files {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		stats.FilesProcessed++

		fileStats, err := idx.indexFile(ctx, absDir, file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[cks-mcp] indexer: skip %s: %v\n", file, err)
			continue
		}
		stats.ChunksCreated += fileStats.Created
		stats.ChunksUpdated += fileStats.Updated
		stats.ChunksReused += fileStats.Reused
		stats.ChunksDeleted += fileStats.Deleted

		if idx.Progress != nil {
			idx.Progress(i+1, total, file)
		}
	}

	// Record the new index_commit so incremental runs know where to resume.
	if commit, err := currentGitHead(absDir); err == nil && commit != "" {
		_ = idx.store.SetMeta(ctx, "index_commit", commit)
		stats.IndexCommit = commit
	}

	embedderMode := "bm25_fallback"
	dim := 0
	if idx.embedder != nil {
		embedderMode = "vector:" + idx.embedder.Name()
		dim = idx.embedder.Dimension()
	}
	_ = idx.store.SetMeta(ctx, "embedder_mode", embedderMode)
	_ = idx.store.SetMeta(ctx, "dimension", fmt.Sprintf("%d", dim))
	_ = idx.store.SetMeta(ctx, "indexed_at", time.Now().UTC().Format(time.RFC3339))

	stats.DurationMs = time.Since(start).Milliseconds()
	return stats, nil
}

// fileStats is internal per-file aggregation.
type fileStats struct {
	Created, Updated, Reused, Deleted int
}

// indexFile parses one file, reconciles its chunks with the store, and
// honors the code_hash cache (RI-23).
func (idx *Indexer) indexFile(ctx context.Context, rootDir, absPath string) (fileStats, error) {
	var stats fileStats
	chunks, err := ParseFile(rootDir, absPath)
	if err != nil {
		return stats, err
	}

	relPath := filepath.ToSlash(mustRel(rootDir, absPath))

	// Track what should remain after this pass; orphans get deleted.
	existing, err := idx.store.ListIDsByFile(ctx, relPath)
	if err != nil {
		return stats, err
	}
	keep := make(map[string]struct{}, len(chunks))

	for _, chunk := range chunks {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		keep[chunk.ID] = struct{}{}

		oldHash, err := idx.store.GetCodeHash(ctx, chunk.ID)
		if err != nil {
			return stats, err
		}
		if oldHash == chunk.CodeHash && oldHash != "" {
			stats.Reused++
			continue
		}

		vec, err := idx.embedChunk(ctx, chunk)
		if err != nil {
			// One bad embedding shouldn't kill the whole file. Store without vector.
			fmt.Fprintf(os.Stderr, "[cks-mcp] indexer: embed failure for %s: %v\n",
				chunk.ID, err)
			vec = nil
		}
		if err := idx.store.Upsert(ctx, chunk, vec); err != nil {
			return stats, err
		}
		if oldHash == "" {
			stats.Created++
		} else {
			stats.Updated++
		}
	}

	// Delete any chunks that no longer exist (e.g., function removed).
	var orphans []string
	for _, id := range existing {
		if _, ok := keep[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	if len(orphans) > 0 {
		n, err := idx.store.DeleteByIDs(ctx, orphans)
		if err != nil {
			return stats, err
		}
		stats.Deleted += n
	}
	return stats, nil
}

func (idx *Indexer) embedChunk(ctx context.Context, c types.CodeChunk) ([]float32, error) {
	if idx.embedder == nil {
		return nil, nil
	}
	text := FormatChunkForEmbedding(c)
	return idx.embedder.Embed(ctx, text)
}

// collectFiles enumerates Go source files for the run. In incremental mode
// it scopes the walk to files touched since SinceCommit (falling back to the
// previously-recorded index_commit). Modules optionally narrows full runs.
func (idx *Indexer) collectFiles(
	ctx context.Context, rootDir string, req IndexRequest,
) ([]string, error) {
	if req.Mode == ModeIncremental {
		commit := req.SinceCommit
		if commit == "" {
			commit, _ = idx.store.GetMeta(ctx, "index_commit")
		}
		if commit == "" {
			// No baseline — degrade to full to avoid silently doing nothing.
			fmt.Fprintln(os.Stderr,
				"[cks-mcp] indexer: incremental requested but no baseline commit; running full")
		} else {
			return gitChangedGoFiles(rootDir, commit)
		}
	}
	return idx.walkProject(rootDir, req.Modules)
}

// walkProject collects all .go files honoring ChunkOpts and the optional
// module filter (RI-09: priority indexing). When modules is non-empty,
// files outside those top-level dirs are skipped.
func (idx *Indexer) walkProject(rootDir string, modules []string) ([]string, error) {
	moduleSet := map[string]struct{}{}
	for _, m := range modules {
		moduleSet[strings.TrimPrefix(strings.TrimSuffix(m, "/"), "./")] = struct{}{}
	}

	var out []string
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == rootDir {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			if len(moduleSet) > 0 {
				rel, _ := filepath.Rel(rootDir, path)
				top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
				if _, ok := moduleSet[top]; !ok {
					return fs.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !idx.ChunkOpts.IncludeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, ex := range idx.ChunkOpts.Excludes {
			if strings.Contains(path, ex) {
				return nil
			}
		}
		out = append(out, path)
		return nil
	})
	sort.Strings(out)
	return out, err
}

// --- git helpers ---

func currentGitHead(repoDir string) (string, error) {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitChangedGoFiles(repoDir, sinceCommit string) ([]string, error) {
	// Range A..HEAD — newly changed files since the baseline.
	cmd := exec.Command(
		"git", "-C", repoDir, "diff", "--name-only",
		sinceCommit+"..HEAD", "--", "*.go",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, filepath.Join(repoDir, line))
	}
	// Also include unstaged + staged modifications so iterative dev sees fresh edits.
	cmd = exec.Command("git", "-C", repoDir, "status", "--porcelain", "--", "*.go")
	if statusOut, err := cmd.Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(statusOut)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Status lines look like "?? path/file.go" or " M path/file.go".
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				files = append(files, filepath.Join(repoDir, parts[len(parts)-1]))
			}
		}
	}
	return dedupStrings(files), nil
}

func mustRel(base, full string) string {
	r, err := filepath.Rel(base, full)
	if err != nil {
		return full
	}
	return r
}

func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
