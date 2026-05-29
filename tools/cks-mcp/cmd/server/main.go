// Command server runs the CKS (Code Knowledge Search) MCP server over stdio.
//
// Required environment variables:
//
//	CKS_INDEX_PATH        path to the SQLite index file (e.g., .coding-agent/index/ckv.db)
//
// Optional environment variables:
//
//	CKS_PATTERNS_PATH     path to shared/patterns.json (auto-detected otherwise)
//	OLLAMA_BASE_URL       Ollama server URL (default http://localhost:11434)
//	OLLAMA_EMBED_MODEL    embedding model name (default nomic-embed-text)
//	CKS_DISABLE_OLLAMA    set to "1" to force BM25 fallback
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/ckv"
	srv "github.com/0xmhha/coding-agent/tools/cks-mcp/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[cks-mcp] fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	indexPath := os.Getenv("CKS_INDEX_PATH")
	if indexPath == "" {
		// Default to .coding-agent/index/ckv.db under CWD.
		cwd, _ := os.Getwd()
		indexPath = filepath.Join(cwd, ".coding-agent", "index", "ckv.db")
	}
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return fmt.Errorf("ensure index dir: %w", err)
	}

	store, err := ckv.Open(indexPath)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer store.Close()

	var embedder ckv.Embedder
	if os.Getenv("CKS_DISABLE_OLLAMA") != "1" {
		if e, perr := ckv.SelectEmbedder(ctx); perr == nil {
			embedder = e
		} else {
			fmt.Fprintf(os.Stderr,
				"[cks-mcp] Ollama unavailable, using BM25 fallback (RI-08): %v\n", perr)
		}
	}

	search := ckv.NewSearchService(store, embedder, ckv.NewReranker())
	indexer := ckv.NewIndexer(store, embedder)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "cks",
		Version: "0.1.0",
	}, nil)
	srv.Register(server, srv.Deps{
		Store:    store,
		Embedder: embedder,
		Search:   search,
		Indexer:  indexer,
	})

	transport := &mcp.StdioTransport{}
	return server.Run(ctx, transport)
}
