package ckv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, rel, contents string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func TestIndexer_FullRun_NoEmbedder(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeFile(t, root, "consensus/wbft/finalize.go",
		"package wbft\n// Finalize seals.\nfunc Finalize() error { return nil }\n")
	writeFile(t, root, "core/block.go",
		"package core\n// Block represents a block.\ntype Block struct{}\n")
	writeFile(t, root, "vendor/dep/x.go",
		"package dep\nfunc Vendored(){}\n")

	store, err := Open(filepath.Join(t.TempDir(), "ckv.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	indexer := NewIndexer(store, nil) // BM25 fallback
	stats, err := indexer.Run(ctx, IndexRequest{Mode: ModeFull, ProjectDir: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.FilesProcessed < 2 {
		t.Fatalf("files_processed = %d; want >= 2", stats.FilesProcessed)
	}
	if stats.ChunksCreated < 2 {
		t.Fatalf("chunks_created = %d; want >= 2", stats.ChunksCreated)
	}
}

func TestIndexer_CodeHashCacheReuse(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeFile(t, root, "x.go", "package x\nfunc A(){}\n")

	store, err := Open(filepath.Join(t.TempDir(), "ckv.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	indexer := NewIndexer(store, nil)
	first, err := indexer.Run(ctx, IndexRequest{Mode: ModeFull, ProjectDir: root})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.ChunksCreated == 0 {
		t.Fatalf("expected ChunksCreated > 0 on first run; got 0")
	}

	// Second run with no file changes should hit the cache.
	second, err := indexer.Run(ctx, IndexRequest{Mode: ModeFull, ProjectDir: root})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.ChunksReused == 0 {
		t.Fatalf("expected ChunksReused > 0 on no-change second run; got 0")
	}
	if second.ChunksCreated != 0 || second.ChunksUpdated != 0 {
		t.Fatalf("expected no create/update on cached re-run; got created=%d updated=%d",
			second.ChunksCreated, second.ChunksUpdated)
	}
}

func TestIndexer_ProgressCallback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeFile(t, root, "a.go", "package a\nfunc A(){}\n")
	writeFile(t, root, "b.go", "package b\nfunc B(){}\n")

	store, _ := Open(filepath.Join(t.TempDir(), "ckv.db"))
	defer store.Close()

	indexer := NewIndexer(store, nil)
	type call struct {
		processed, total int
		file             string
	}
	var calls []call
	indexer.Progress = func(p, total int, f string) {
		calls = append(calls, call{processed: p, total: total, file: f})
	}
	if _, err := indexer.Run(ctx, IndexRequest{Mode: ModeFull, ProjectDir: root}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("progress calls = %d; want 2", len(calls))
	}
	if calls[1].processed != 2 || calls[1].total != 2 {
		t.Fatalf("final progress = %+v; want processed=2 total=2", calls[1])
	}
}
