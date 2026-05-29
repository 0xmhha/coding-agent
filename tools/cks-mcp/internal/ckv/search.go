package ckv

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/filter"
	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// SearchService ties the store, embedder (optional), and reranker into the
// CKV search pipeline. It is safe for concurrent use because each method
// re-derives state from its inputs.
type SearchService struct {
	store    *Store
	embedder Embedder // may be nil → BM25 fallback (RI-08)
	reranker *Reranker
}

// NewSearchService constructs the pipeline. Pass nil embedder to force BM25.
func NewSearchService(store *Store, embedder Embedder, reranker *Reranker) *SearchService {
	if reranker == nil {
		reranker = NewReranker()
	}
	return &SearchService{store: store, embedder: embedder, reranker: reranker}
}

// SearchRequest is the structured input to the pipeline.
type SearchRequest struct {
	Query          string
	TopK           int
	Filters        types.SearchFilters
	IncludeHistory bool
	Rerank         bool
}

// Search runs the full pipeline:
//
//  1. (optional) embedding lookup
//  2. vector or BM25 retrieval with filters
//  3. enrichment with git history
//  4. reranking
//  5. sensitive-content filtering on returned snippets
//
// Errors from the sensitive filter never leak the original snippet — the
// engine's fail-safe (RI-06) drops the corresponding result.
func (s *SearchService) Search(ctx context.Context, req SearchRequest) (*types.SearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, errors.New("ckv: query is empty")
	}
	if req.TopK <= 0 {
		req.TopK = 10
	}
	start := time.Now()

	results, mode, totalCandidates, err := s.retrieve(ctx, req)
	if err != nil {
		return nil, err
	}

	if req.IncludeHistory {
		s.enrichHistory(results)
	}

	if req.Rerank && len(results) > 0 {
		results = s.reranker.Apply(results, req.Query)
	}
	if req.TopK > 0 && len(results) > req.TopK {
		results = results[:req.TopK]
	}

	results = s.applySensitiveFilter(results)

	commit, _ := s.store.GetMeta(ctx, "index_commit")
	return &types.SearchResponse{
		Results: results,
		Metadata: types.SearchMetadata{
			TotalCandidates: totalCandidates,
			Reranked:        req.Rerank,
			IndexCommit:     commit,
			QueryTimeMs:     time.Since(start).Milliseconds(),
			EmbedderMode:    mode,
		},
	}, nil
}

// retrieve runs either vector search or BM25 depending on embedder availability.
// It over-fetches by 3× topK so reranking has room to reorder.
func (s *SearchService) retrieve(
	ctx context.Context, req SearchRequest,
) (results []types.SearchResult, mode string, totalCandidates int, err error) {
	overfetch := req.TopK * 3
	if overfetch < req.TopK {
		overfetch = req.TopK
	}

	if s.embedder != nil {
		vec, embedErr := s.embedder.Embed(ctx, req.Query)
		if embedErr == nil && len(vec) > 0 {
			rs, vErr := s.store.VectorSearch(ctx, vec, overfetch, req.Filters)
			if vErr != nil {
				return nil, "", 0, vErr
			}
			return rs, "vector:" + s.embedder.Name(), len(rs), nil
		}
		// Fall through to BM25 if embedding failed.
	}

	rows, fErr := s.store.AllForBM25(ctx, req.Filters)
	if fErr != nil {
		return nil, "", 0, fErr
	}
	rs := BM25SearchOverChunks(req.Query, rows, overfetch)
	return rs, "bm25_fallback", len(rs), nil
}

// enrichHistory adds a short summary line; Phase 4 will replace this with
// real git history. We populate a stable string so the reranker's recency
// boost has a signal to work with (RI-23 cache assumes deterministic output).
func (s *SearchService) enrichHistory(results []types.SearchResult) {
	now := time.Now().UTC().Format("2006-01")
	for i := range results {
		if results[i].GitHistorySummary != "" {
			continue
		}
		results[i].GitHistorySummary = now + ": indexed (no git history yet — Phase 4)"
	}
}

// applySensitiveFilter scans every snippet through the filter engine. Hits
// that resolve to BLOCKED are dropped from the response; REDACTED snippets
// are replaced with the sanitized text.
func (s *SearchService) applySensitiveFilter(in []types.SearchResult) []types.SearchResult {
	out := in[:0]
	for _, r := range in {
		fr := filter.ScanAndFilter(r.Snippet)
		switch fr.Metadata.ScanResult {
		case types.ScanBlocked:
			// Drop entirely — we never want to leak even a hint of the original.
			continue
		case types.ScanRedacted:
			r.Snippet = fr.Text
		}
		out = append(out, r)
	}
	return out
}
