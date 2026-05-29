// Package server wires the CKS MCP tools (ckv_search, ckv_index) onto an
// mcp.Server. Phase 4 will extend this with ckg_query / ckg_impact.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/ckg"
	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/ckv"
	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Deps is the wiring point for handlers. Either fields may be nil — Search
// requires Store + (embedder optional), Index requires Store + (embedder optional).
type Deps struct {
	Store        *ckv.Store
	Embedder     ckv.Embedder // may be nil → BM25 fallback
	Search       *ckv.SearchService
	Indexer      *ckv.Indexer
	CKGStore     *ckg.Store
	CKGQuery     *ckg.QueryService
	CKGIndexer   *ckg.Indexer
}

// Register attaches all CKS tools to s.
func Register(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "ckv_search",
		Description: "Semantic + lexical code search over the indexed go-stablenet codebase. " +
			"Returns top-k matching chunks with snippet, signature, godoc, and score. " +
			"Falls back to BM25 when no embedder is configured.",
	}, makeSearchHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name: "ckv_index",
		Description: "Build or refresh the CKV code index. " +
			"Mode 'full' walks the project; 'incremental' uses git diff from the last indexed commit. " +
			"Provide modules to prioritize a subset of top-level directories.",
	}, makeIndexHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name: "ckg_query",
		Description: "Traverse the code knowledge graph from one or more seed symbols. " +
			"Returns nodes + edges; optionally include git history and concurrency context.",
	}, makeCKGQueryHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name: "ckg_impact",
		Description: "Analyze the blast radius of changing a symbol. Returns direct/indirect callers, " +
			"interface contracts, test files, concurrency risk, and a high-level risk classification.",
	}, makeCKGImpactHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name: "ckg_index",
		Description: "Build or refresh the CKG (relations + history + concurrency). " +
			"Runs the typed extractor with a Tier-2 AST-only fallback when packages.Load fails (RI-10).",
	}, makeCKGIndexHandler(deps))
}

// --- Inputs ---

type ckvSearchInput struct {
	Query          string `json:"query" jsonschema:"required,description=Natural language or code-identifier query"`
	TopK           int    `json:"top_k,omitempty" jsonschema:"description=Maximum results to return (default 10)"`
	Package        string `json:"package,omitempty"`
	FilePattern    string `json:"file_pattern,omitempty" jsonschema:"description=SQL LIKE pattern, e.g. consensus/wbft/%"`
	SymbolType     string `json:"symbol_type,omitempty" jsonschema:"description=function|method|struct|interface|const|var"`
	ModifiedSince  string `json:"modified_since,omitempty" jsonschema:"description=ISO datetime filter"`
	IncludeHistory bool   `json:"include_history,omitempty"`
	Rerank         *bool  `json:"rerank,omitempty" jsonschema:"description=Default true"`
}

type ckvIndexInput struct {
	Mode        string   `json:"mode" jsonschema:"required,enum=full,enum=incremental"`
	ProjectDir  string   `json:"project_dir" jsonschema:"required"`
	Modules     []string `json:"modules,omitempty" jsonschema:"description=Optional priority list for full mode"`
	SinceCommit string   `json:"since_commit,omitempty"`
}

// --- Handlers ---

func makeSearchHandler(deps Deps) mcp.ToolHandlerFor[ckvSearchInput, types.SearchResponse] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ckvSearchInput) (*mcp.CallToolResult, types.SearchResponse, error) {
		if deps.Search == nil {
			return errResult("UNINITIALIZED", "search service is not configured"), types.SearchResponse{}, nil
		}
		rerank := true
		if in.Rerank != nil {
			rerank = *in.Rerank
		}
		req := ckv.SearchRequest{
			Query: in.Query,
			TopK:  in.TopK,
			Filters: types.SearchFilters{
				Package:       in.Package,
				FilePattern:   in.FilePattern,
				SymbolType:    in.SymbolType,
				ModifiedSince: in.ModifiedSince,
			},
			IncludeHistory: in.IncludeHistory,
			Rerank:         rerank,
		}
		resp, err := deps.Search.Search(ctx, req)
		if err != nil {
			return errResult("INTERNAL_ERROR", err.Error()), types.SearchResponse{}, nil
		}
		return nil, *resp, nil
	}
}

func makeIndexHandler(deps Deps) mcp.ToolHandlerFor[ckvIndexInput, types.IndexStats] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ckvIndexInput) (*mcp.CallToolResult, types.IndexStats, error) {
		if deps.Indexer == nil {
			return errResult("UNINITIALIZED", "indexer is not configured"), types.IndexStats{}, nil
		}
		mode := ckv.IndexMode(in.Mode)
		if mode != ckv.ModeFull && mode != ckv.ModeIncremental {
			return errResult("INVALID_ARG", "mode must be 'full' or 'incremental'"), types.IndexStats{}, nil
		}
		// Progress logged to stderr so the agent sees it without polluting the result payload.
		deps.Indexer.Progress = func(processed, total int, currentFile string) {
			if total > 0 && processed%50 == 0 {
				fmt.Fprintf(os.Stderr, "[cks-mcp] indexing %d/%d: %s\n", processed, total, currentFile)
			}
		}
		stats, err := deps.Indexer.Run(ctx, ckv.IndexRequest{
			Mode:        mode,
			ProjectDir:  in.ProjectDir,
			Modules:     in.Modules,
			SinceCommit: in.SinceCommit,
		})
		if err != nil {
			return errResult("INTERNAL_ERROR", err.Error()), stats, nil
		}
		return nil, stats, nil
	}
}

// --- CKG inputs ---

type ckgQueryInput struct {
	Symbols            []string `json:"symbols" jsonschema:"required,description=Seed symbols (qualified or short name)"`
	Depth              int      `json:"depth,omitempty" jsonschema:"description=BFS depth (default 2)"`
	RelationTypes      []string `json:"relation_types,omitempty"`
	IncludeHistory     bool     `json:"include_history,omitempty"`
	IncludeConcurrency bool     `json:"include_concurrency,omitempty"`
	MaxNodes           int      `json:"max_nodes,omitempty"`
	MaxEdges           int      `json:"max_edges,omitempty"`
}

type ckgImpactInput struct {
	Symbol     string `json:"symbol" jsonschema:"required"`
	ChangeType string `json:"change_type,omitempty" jsonschema:"description=signature|logic|delete (default logic)"`
}

type ckgIndexInput struct {
	ProjectDir string `json:"project_dir" jsonschema:"required"`
}

// --- CKG handlers ---

func makeCKGQueryHandler(deps Deps) mcp.ToolHandlerFor[ckgQueryInput, types.CKGQueryResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ckgQueryInput) (*mcp.CallToolResult, types.CKGQueryResult, error) {
		if deps.CKGQuery == nil {
			return errResult("UNINITIALIZED", "ckg query service is not configured"), types.CKGQueryResult{}, nil
		}
		rels := make([]types.RelationType, 0, len(in.RelationTypes))
		for _, r := range in.RelationTypes {
			rels = append(rels, types.RelationType(r))
		}
		res, err := deps.CKGQuery.Query(ctx, ckg.QueryRequest{
			Symbols:            in.Symbols,
			Depth:              in.Depth,
			RelationTypes:      rels,
			IncludeHistory:     in.IncludeHistory,
			IncludeConcurrency: in.IncludeConcurrency,
			MaxNodes:           in.MaxNodes,
			MaxEdges:           in.MaxEdges,
		})
		if err != nil {
			return errResult("INTERNAL_ERROR", err.Error()), types.CKGQueryResult{}, nil
		}
		return nil, *res, nil
	}
}

func makeCKGImpactHandler(deps Deps) mcp.ToolHandlerFor[ckgImpactInput, types.CKGImpactResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ckgImpactInput) (*mcp.CallToolResult, types.CKGImpactResult, error) {
		if deps.CKGQuery == nil {
			return errResult("UNINITIALIZED", "ckg query service is not configured"), types.CKGImpactResult{}, nil
		}
		res, err := deps.CKGQuery.Impact(ctx, ckg.ImpactRequest{
			Symbol:     in.Symbol,
			ChangeType: in.ChangeType,
		})
		if err != nil {
			return errResult("INTERNAL_ERROR", err.Error()), types.CKGImpactResult{}, nil
		}
		return nil, *res, nil
	}
}

func makeCKGIndexHandler(deps Deps) mcp.ToolHandlerFor[ckgIndexInput, types.CKGIndexStats] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ckgIndexInput) (*mcp.CallToolResult, types.CKGIndexStats, error) {
		if deps.CKGIndexer == nil {
			return errResult("UNINITIALIZED", "ckg indexer is not configured"), types.CKGIndexStats{}, nil
		}
		stats, err := deps.CKGIndexer.Run(ctx, in.ProjectDir)
		if err != nil {
			return errResult("INTERNAL_ERROR", err.Error()), stats, nil
		}
		return nil, stats, nil
	}
}

func errResult(code, message string) *mcp.CallToolResult {
	payload, _ := json.Marshal(map[string]any{
		"error":       code,
		"message":     message,
		"recoverable": code != "UNINITIALIZED",
	})
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}},
	}
}
