package ckg

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// Indexer runs the full CKG extraction pipeline (relations → history →
// concurrency) and persists the result into Store.
type Indexer struct {
	Store          *Store
	HistoryLimit   int  // entries per node; 0 disables history
	HistoryFollow  bool // use --follow file-level history when symbol log is empty
	ProgressLogger func(processed, total int, current string)
}

// NewIndexer constructs an indexer with sensible defaults.
func NewIndexer(store *Store) *Indexer {
	return &Indexer{
		Store:         store,
		HistoryLimit:  5,
		HistoryFollow: true,
	}
}

// Run executes a full CKG indexing pass over projectDir.
func (idx *Indexer) Run(ctx context.Context, projectDir string) (types.CKGIndexStats, error) {
	stats := types.CKGIndexStats{}
	start := time.Now()

	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return stats, err
	}

	res, err := Extract(ctx, abs)
	if err != nil {
		return stats, err
	}
	stats.Mode = string(res.Mode)

	// Persist nodes and edges.
	nodesByQName := map[string]string{}
	for _, n := range res.Nodes {
		if err := idx.Store.UpsertNode(ctx, n); err != nil {
			return stats, err
		}
		nodesByQName[n.QualifiedName] = n.ID
	}
	stats.NodesCreated = len(res.Nodes)

	for _, e := range res.Edges {
		if err := idx.Store.UpsertEdge(ctx, e); err != nil {
			return stats, err
		}
	}
	stats.EdgesCreated = len(res.Edges)

	// History per node (if enabled).
	if idx.HistoryLimit > 0 {
		history := NewHistoryAnalyzer(abs, idx.HistoryLimit)
		for _, n := range res.Nodes {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			entries, _ := history.History(ctx, n.FilePath, n.StartLine, n.EndLine)
			if len(entries) == 0 && idx.HistoryFollow {
				entries, _ = history.HistoryByFile(ctx, n.FilePath)
			}
			for _, e := range entries {
				e.NodeID = n.ID
				if err := idx.Store.AppendHistory(ctx, e); err != nil {
					return stats, err
				}
				stats.HistoryEntries++
			}
		}
	}

	// Concurrency analysis (re-parse files; cheap because we cached AST inside Extract).
	if len(res.Nodes) > 0 {
		files, perr := loadFileASTs(ctx, abs)
		if perr == nil {
			ccs := AnalyzeConcurrency(files, nodesByQName)
			for _, cc := range ccs {
				if err := idx.Store.UpsertConcurrencyContext(ctx, cc); err != nil {
					return stats, err
				}
				stats.ConcurrencyContexts++
			}
		} else {
			fmt.Fprintf(os.Stderr,
				"[cks-mcp] ckg: concurrency analyzer parse failed: %v\n", perr)
		}
	}

	stats.DurationMs = time.Since(start).Milliseconds()
	return stats, nil
}

// loadFileASTs re-parses Go files for the concurrency analyzer. We separate
// this from Extract so the typed-mode path doesn't have to keep the AST
// for every file in memory once relations are computed.
func loadFileASTs(ctx context.Context, projectDir string) ([]FileAST, error) {
	var out []FileAST
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == projectDir {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") ||
			strings.Contains(path, "vendor/") ||
			strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		fset := token.NewFileSet()
		src, rerr := os.ReadFile(path) //nolint:gosec
		if rerr != nil {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, src, parser.ParseComments)
		if perr != nil {
			return nil
		}
		pkgName := ""
		if file.Name != nil {
			pkgName = file.Name.Name
		}
		rel, _ := filepath.Rel(projectDir, path)
		out = append(out, FileAST{
			FileSet: fset,
			File:    file,
			RelPath: filepath.ToSlash(rel),
			PkgName: pkgName,
		})
		return nil
	})
	return out, err
}
