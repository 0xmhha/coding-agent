package ckv

import (
	"testing"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

func fixedClock(s string) func() time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return func() time.Time { return t }
}

func TestReranker_SignatureBoost(t *testing.T) {
	r := &Reranker{Now: fixedClock("2026-05-29T00:00:00Z")}
	results := []types.SearchResult{
		{Symbol: "Plain", Score: 1.0, Signature: "func Plain()"},
		{Symbol: "Match", Score: 1.0, Signature: "func StakerInfo()"},
	}
	out := r.Apply(results, "staker info")
	if out[0].Symbol != "Match" {
		t.Fatalf("top = %s; want Match (signature boost)", out[0].Symbol)
	}
	if out[0].Score <= out[1].Score {
		t.Fatalf("expected boosted score to win")
	}
}

func TestReranker_PackageProximityBoost(t *testing.T) {
	r := &Reranker{Now: fixedClock("2026-05-29T00:00:00Z")}
	results := []types.SearchResult{
		{Symbol: "X", Score: 1.0, Package: "core"},
		{Symbol: "Y", Score: 1.0, Package: "consensus"},
	}
	out := r.Apply(results, "consensus block")
	if out[0].Symbol != "Y" {
		t.Fatalf("top = %s; want Y (package proximity)", out[0].Symbol)
	}
}

func TestReranker_RecencyBoost(t *testing.T) {
	r := &Reranker{Now: fixedClock("2026-05-29T00:00:00Z")}
	results := []types.SearchResult{
		{Symbol: "Old", Score: 1.0, GitHistorySummary: "2020-01: created"},
		{Symbol: "New", Score: 1.0, GitHistorySummary: "2026-05: refactor"},
	}
	out := r.Apply(results, "anything")
	if out[0].Symbol != "New" {
		t.Fatalf("top = %s; want New (recency)", out[0].Symbol)
	}
}

func TestReranker_StableForEmpty(t *testing.T) {
	r := NewReranker()
	if out := r.Apply(nil, "anything"); out != nil {
		t.Fatalf("nil input should return nil; got %v", out)
	}
}
