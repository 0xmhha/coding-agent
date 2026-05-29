package ckg

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
	s, err := Open(filepath.Join(dir, "ckg.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleNode(id, qname string) types.GraphNode {
	return types.GraphNode{
		ID:            id,
		FilePath:      "x.go",
		PackageName:   "x",
		SymbolName:    qname,
		SymbolType:    types.SymbolFunction,
		QualifiedName: qname,
		Signature:     "func " + qname,
		StartLine:     1,
		EndLine:       1,
		IndexedAt:     time.Now().UTC(),
	}
}

func TestStore_NodesEdgesRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	a := sampleNode("a1", "x.A")
	b := sampleNode("b1", "x.B")
	if err := s.UpsertNode(ctx, a); err != nil {
		t.Fatalf("UpsertNode A: %v", err)
	}
	if err := s.UpsertNode(ctx, b); err != nil {
		t.Fatalf("UpsertNode B: %v", err)
	}
	e := types.GraphEdge{
		FromNode: "a1", ToNode: "b1",
		RelationType: types.RelCalls, Confidence: types.ConfidenceHigh,
	}
	if err := s.UpsertEdge(ctx, e); err != nil {
		t.Fatalf("UpsertEdge: %v", err)
	}
	nc, _ := s.CountNodes(ctx)
	ec, _ := s.CountEdges(ctx)
	if nc != 2 || ec != 1 {
		t.Fatalf("counts = nodes=%d edges=%d; want 2/1", nc, ec)
	}
	edges, err := s.EdgesByFromNodes(ctx, []string{"a1"}, nil)
	if err != nil {
		t.Fatalf("EdgesByFromNodes: %v", err)
	}
	if len(edges) != 1 || edges[0].ToNode != "b1" {
		t.Fatalf("edges = %+v; want a1→b1", edges)
	}
}

func TestStore_BFS_DepthLimits(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, qn := range []string{"x.A", "x.B", "x.C", "x.D"} {
		_ = s.UpsertNode(ctx, sampleNode(qn, qn))
	}
	mkEdge := func(from, to string) types.GraphEdge {
		return types.GraphEdge{FromNode: from, ToNode: to,
			RelationType: types.RelCalls, Confidence: types.ConfidenceHigh}
	}
	_ = s.UpsertEdge(ctx, mkEdge("x.A", "x.B"))
	_ = s.UpsertEdge(ctx, mkEdge("x.B", "x.C"))
	_ = s.UpsertEdge(ctx, mkEdge("x.C", "x.D"))

	t1, err := s.BFS(ctx, TraversalRequest{StartNodes: []string{"x.A"}, Depth: 1})
	if err != nil {
		t.Fatalf("BFS depth=1: %v", err)
	}
	if len(t1.Nodes) != 2 {
		t.Fatalf("depth=1 nodes = %d; want 2 (A+B)", len(t1.Nodes))
	}

	t3, err := s.BFS(ctx, TraversalRequest{StartNodes: []string{"x.A"}, Depth: 3})
	if err != nil {
		t.Fatalf("BFS depth=3: %v", err)
	}
	if len(t3.Nodes) != 4 {
		t.Fatalf("depth=3 nodes = %d; want 4", len(t3.Nodes))
	}
}

func TestStore_BFS_MaxNodesTruncates(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, q := range []string{"a", "b", "c", "d", "e"} {
		_ = s.UpsertNode(ctx, sampleNode(q, q))
		if q != "a" {
			_ = s.UpsertEdge(ctx, types.GraphEdge{
				FromNode: "a", ToNode: q,
				RelationType: types.RelCalls, Confidence: types.ConfidenceHigh,
			})
		}
	}
	out, err := s.BFS(ctx, TraversalRequest{
		StartNodes: []string{"a"}, Depth: 1, MaxNodes: 3,
	})
	if err != nil {
		t.Fatalf("BFS: %v", err)
	}
	if !out.Truncated {
		t.Fatalf("expected truncation when max_nodes < reachable")
	}
}

func TestStore_DeleteNodesByFile(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	a := sampleNode("a", "x.A")
	a.FilePath = "alpha.go"
	b := sampleNode("b", "x.B")
	b.FilePath = "beta.go"
	_ = s.UpsertNode(ctx, a)
	_ = s.UpsertNode(ctx, b)
	_ = s.UpsertEdge(ctx, types.GraphEdge{
		FromNode: "a", ToNode: "b",
		RelationType: types.RelCalls, Confidence: types.ConfidenceHigh,
	})

	if err := s.DeleteNodesByFile(ctx, "alpha.go"); err != nil {
		t.Fatalf("DeleteNodesByFile: %v", err)
	}
	nc, _ := s.CountNodes(ctx)
	ec, _ := s.CountEdges(ctx)
	if nc != 1 || ec != 0 {
		t.Fatalf("after delete: nodes=%d edges=%d; want 1/0", nc, ec)
	}
}
