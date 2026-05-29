package ckg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	// Make it a valid Go module so packages.Load works.
	gomod := "module example.test\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return root
}

func hasEdge(edges []types.GraphEdge, from, to string, rt types.RelationType) bool {
	for _, e := range edges {
		if e.FromNode == from && e.ToNode == to && e.RelationType == rt {
			return true
		}
	}
	return false
}

func findNodeID(nodes []types.GraphNode, qname string) string {
	for _, n := range nodes {
		if n.QualifiedName == qname {
			return n.ID
		}
	}
	return ""
}

func TestExtract_CallsAndImplementsAndEmbeds(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.go": `package main

type Sealer interface { Seal() error }

type Engine struct{}
func (e *Engine) Seal() error { return nil }
func (e *Engine) Run() { e.Seal() }

func Drive(s Sealer) { _ = s.Seal() }
`,
	})
	res, err := Extract(context.Background(), root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Mode != ModeTyped {
		t.Fatalf("mode = %s; want typed", res.Mode)
	}

	engineID := findNodeID(res.Nodes, "main.Engine")
	sealerID := findNodeID(res.Nodes, "main.Sealer")
	runID := findNodeID(res.Nodes, "main.(Engine).Run")
	sealMethodID := findNodeID(res.Nodes, "main.(Engine).Seal")
	if engineID == "" || sealerID == "" || runID == "" || sealMethodID == "" {
		t.Fatalf("missing one of the expected nodes: engine=%q sealer=%q run=%q sealMethod=%q",
			engineID, sealerID, runID, sealMethodID)
	}

	if !hasEdge(res.Edges, engineID, sealerID, types.RelImplements) {
		t.Errorf("expected Engine implements Sealer edge")
	}
	if !hasEdge(res.Edges, runID, sealMethodID, types.RelCalls) {
		t.Errorf("expected Run calls Seal edge")
	}
}

func TestExtract_StructEmbeds(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.go": `package main

type Base struct{}
type Engine struct{ *Base }
`,
	})
	res, err := Extract(context.Background(), root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	engineID := findNodeID(res.Nodes, "main.Engine")
	baseID := findNodeID(res.Nodes, "main.Base")
	if engineID == "" || baseID == "" {
		t.Fatalf("missing nodes engine=%q base=%q", engineID, baseID)
	}
	if !hasEdge(res.Edges, engineID, baseID, types.RelEmbeds) {
		t.Errorf("expected Engine embeds Base edge")
	}
}
