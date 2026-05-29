package ckg

import (
	"context"
	"testing"

	internaltypes "github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

func TestConcurrency_GoroutineAndMutex(t *testing.T) {
	root := writeProject(t, map[string]string{
		"main.go": `package main

import "sync"

type Engine struct{
	mu sync.Mutex
	x  int
}

func (e *Engine) Set(v int) {
	e.mu.Lock()
	e.x = v
	e.mu.Unlock()
}

func (e *Engine) Launch() {
	go e.Set(1)
}
`,
	})
	res, err := Extract(context.Background(), root)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	files, err := loadFileASTs(context.Background(), root)
	if err != nil {
		t.Fatalf("loadFileASTs: %v", err)
	}
	nodesByQName := map[string]string{}
	for _, n := range res.Nodes {
		nodesByQName[n.QualifiedName] = n.ID
	}
	ccs := AnalyzeConcurrency(files, nodesByQName)

	var launchCC, setCC *internaltypes.ConcurrencyContext
	for i, c := range ccs {
		node := findNodeByID(res.Nodes, c.NodeID)
		if node == nil {
			continue
		}
		switch node.QualifiedName {
		case "main.(Engine).Launch":
			launchCC = &ccs[i]
		case "main.(Engine).Set":
			setCC = &ccs[i]
		}
	}
	if launchCC == nil || setCC == nil {
		t.Fatalf("missing concurrency contexts (launch=%v set=%v)", launchCC, setCC)
	}
	if len(launchCC.GoroutineContext.Launches) == 0 {
		t.Errorf("expected Launch.Launches to be non-empty")
	}
	gotMutex := false
	for _, sm := range setCC.SyncMechanisms {
		if sm.Type == "mutex" {
			gotMutex = true
			break
		}
	}
	if !gotMutex {
		t.Errorf("expected Set to have mutex sync mechanism: %+v", setCC.SyncMechanisms)
	}
}

func findNodeByID(nodes []internaltypes.GraphNode, id string) *internaltypes.GraphNode {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}
