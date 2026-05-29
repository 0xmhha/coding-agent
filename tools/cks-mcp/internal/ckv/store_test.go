package ckv

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "ckv.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func sampleChunk(id, file, sym string) types.CodeChunk {
	return types.CodeChunk{
		ID:          id,
		FilePath:    file,
		PackageName: "pkg",
		SymbolName:  sym,
		SymbolType:  types.SymbolFunction,
		Code:        "func " + sym + "() {}",
		Signature:   "func " + sym + "()",
		Godoc:       "doc for " + sym,
		StartLine:   1,
		EndLine:     1,
		CodeHash:    "hash:" + id,
		IndexedAt:   time.Now().UTC(),
	}
}

func TestStore_UpsertAndCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if err := s.Upsert(ctx, sampleChunk("a", "x.go", "Foo"), nil); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s.Upsert(ctx, sampleChunk("b", "x.go", "Bar"), nil); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	n, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("count = %d; want 2", n)
	}
}

func TestStore_CodeHashCache(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	c := sampleChunk("c1", "y.go", "Baz")
	if err := s.Upsert(ctx, c, nil); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.GetCodeHash(ctx, "c1")
	if err != nil {
		t.Fatalf("GetCodeHash: %v", err)
	}
	if got != c.CodeHash {
		t.Fatalf("code_hash = %q; want %q", got, c.CodeHash)
	}
	missing, err := s.GetCodeHash(ctx, "nope")
	if err != nil {
		t.Fatalf("GetCodeHash missing: %v", err)
	}
	if missing != "" {
		t.Fatalf("missing hash = %q; want empty", missing)
	}
}

func TestStore_DeleteByFile(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_ = s.Upsert(ctx, sampleChunk("a", "x.go", "A"), nil)
	_ = s.Upsert(ctx, sampleChunk("b", "x.go", "B"), nil)
	_ = s.Upsert(ctx, sampleChunk("c", "y.go", "C"), nil)

	n, err := s.DeleteByFile(ctx, "x.go")
	if err != nil {
		t.Fatalf("DeleteByFile: %v", err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d; want 2", n)
	}
	count, _ := s.Count(ctx)
	if count != 1 {
		t.Fatalf("remaining = %d; want 1", count)
	}
}

func TestStore_VectorSearch(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Put three chunks with different vectors; query closest to "consensus".
	chunks := []struct {
		id     string
		sym    string
		vec    []float32
	}{
		{"c1", "Finalize", []float32{0.9, 0.1, 0.1}},
		{"c2", "Block", []float32{0.1, 0.9, 0.1}},
		{"c3", "Pool", []float32{0.1, 0.1, 0.9}},
	}
	for _, c := range chunks {
		ch := sampleChunk(c.id, c.sym+".go", c.sym)
		if err := s.Upsert(ctx, ch, c.vec); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	query := []float32{0.95, 0.05, 0.05}
	results, err := s.VectorSearch(ctx, query, 2, types.SearchFilters{})
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results; want 2", len(results))
	}
	if results[0].Symbol != "Finalize" {
		t.Fatalf("top result = %s; want Finalize", results[0].Symbol)
	}
}

func TestStore_VectorSearch_FilterPackage(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	a := sampleChunk("a", "consensus/x.go", "Foo")
	a.PackageName = "consensus"
	b := sampleChunk("b", "core/y.go", "Bar")
	b.PackageName = "core"

	_ = s.Upsert(ctx, a, []float32{1, 0, 0})
	_ = s.Upsert(ctx, b, []float32{1, 0, 0})

	results, err := s.VectorSearch(ctx, []float32{1, 0, 0}, 10,
		types.SearchFilters{Package: "consensus"})
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d; want 1 (filtered by package)", len(results))
	}
	if results[0].Package != "consensus" {
		t.Fatalf("result package = %s; want consensus", results[0].Package)
	}
}
