package ckv

import (
	"testing"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

func TestTokenize_CamelCaseSplit(t *testing.T) {
	tokens := tokenize("GetStakerInfo")
	want := map[string]bool{"get": true, "staker": true, "info": true}
	if len(tokens) != 3 {
		t.Fatalf("tokens = %v; want 3 items", tokens)
	}
	for _, tk := range tokens {
		if !want[tk] {
			t.Fatalf("unexpected token %q", tk)
		}
	}
}

func TestTokenize_DropsStopwordsAndShortTokens(t *testing.T) {
	tokens := tokenize("the a b consensus rule")
	for _, tk := range tokens {
		if tk == "the" || tk == "a" || tk == "b" {
			t.Fatalf("expected stopword/short token to be dropped, got %q", tk)
		}
	}
	gotConsensus := false
	gotRule := false
	for _, tk := range tokens {
		if tk == "consensus" {
			gotConsensus = true
		}
		if tk == "rule" {
			gotRule = true
		}
	}
	if !gotConsensus || !gotRule {
		t.Fatalf("expected consensus and rule in tokens, got %v", tokens)
	}
}

func TestBM25_RanksMatchingFirst(t *testing.T) {
	docs := []chunkRow{
		{
			ID: "d1", FilePath: "x.go", PackageName: "x", SymbolName: "Unrelated",
			Code: "func Unrelated() { return }", Signature: "func Unrelated()",
		},
		{
			ID: "d2", FilePath: "y.go", PackageName: "wbft", SymbolName: "Finalize",
			Code:      "func (e *Engine) Finalize() error { ... }",
			Signature: "func (e *Engine) Finalize() error",
			Godoc:     "Finalize seals a consensus block.",
		},
		{
			ID: "d3", FilePath: "z.go", PackageName: "core", SymbolName: "Block",
			Code: "type Block struct{}", Signature: "type Block",
		},
	}
	results := BM25SearchOverChunks("consensus finalize block", docs, 3)
	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}
	if results[0].Symbol != "Finalize" {
		t.Fatalf("top result = %s; want Finalize", results[0].Symbol)
	}
}

func TestBM25_EmptyInputs(t *testing.T) {
	if got := BM25SearchOverChunks("", []chunkRow{{ID: "x"}}, 5); got != nil {
		t.Fatalf("empty query should return nil; got %v", got)
	}
	if got := BM25SearchOverChunks("foo", nil, 5); got != nil {
		t.Fatalf("empty docs should return nil; got %v", got)
	}
}

func TestFormatChunkForEmbedding_IncludesContext(t *testing.T) {
	chunk := types.CodeChunk{
		PackageName: "wbft",
		FilePath:    "consensus/wbft/finalize.go",
		SymbolType:  types.SymbolFunction,
		Signature:   "func Finalize() error",
		Godoc:       "Finalize seals the consensus block.",
		Code:        "func Finalize() error { return nil }",
	}
	text := FormatChunkForEmbedding(chunk)
	for _, sub := range []string{"Package: wbft", "File: consensus/", "Signature: func Finalize", "seals the consensus block"} {
		if !contains(text, sub) {
			t.Fatalf("formatted text missing %q:\n%s", sub, text)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
