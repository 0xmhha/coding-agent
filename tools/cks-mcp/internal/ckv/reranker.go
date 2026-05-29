package ckv

import (
	"sort"
	"strings"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// Reranker applies lightweight heuristic boosts on top of the base search
// score. Phase 3 §6.2 (specs/phase3-cks-mcp-ckv.md).
type Reranker struct {
	// Now lets tests inject a deterministic clock.
	Now func() time.Time
}

// NewReranker returns a reranker with the wall-clock time provider.
func NewReranker() *Reranker {
	return &Reranker{Now: func() time.Time { return time.Now().UTC() }}
}

// Apply re-scores results in place and returns them sorted by the new score.
// queryKeywords should be the same tokens used by the BM25 fallback so the
// rules behave consistently regardless of search mode.
func (r *Reranker) Apply(results []types.SearchResult, query string) []types.SearchResult {
	if len(results) == 0 {
		return results
	}
	kw := tokenize(query)
	now := r.Now()
	for i := range results {
		results[i].Score = r.boost(results[i], kw, now)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func (r *Reranker) boost(item types.SearchResult, queryKeywords []string, now time.Time) float64 {
	score := item.Score
	if score == 0 {
		score = 1.0 // give zero-score entries a baseline so multipliers still differentiate
	}

	// 1. Signature boost: ×1.5 if any query keyword appears in the signature.
	if textContainsAny(strings.ToLower(item.Signature), queryKeywords) {
		score *= 1.5
	}

	// 2. Godoc boost: ×1.3 if any query keyword appears in the godoc.
	if textContainsAny(strings.ToLower(item.Godoc), queryKeywords) {
		score *= 1.3
	}

	// 3. Recently modified boost: ×1.1 if git_modified within 30d.
	// SearchResult itself doesn't carry git_modified, but the symbol position
	// (line numbers) plus path can be used as a proxy; in the Phase 3 design,
	// recency is reported via GitHistorySummary. Since search.go enriches the
	// summary, we conservatively reuse it as the recency signal.
	if isRecentSummary(item.GitHistorySummary, now) {
		score *= 1.1
	}

	// 4. Package proximity boost: ×1.2 if the query mentions the package name.
	if item.Package != "" && containsToken(queryKeywords, strings.ToLower(item.Package)) {
		score *= 1.2
	}

	return score
}

func textContainsAny(haystack string, needles []string) bool {
	if haystack == "" {
		return false
	}
	for _, n := range needles {
		if n == "" {
			continue
		}
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// isRecentSummary returns true when the supplied summary text mentions a
// year/month that falls within the recency window. Used as a coarse proxy
// because SearchResult does not carry the raw git_modified timestamp.
func isRecentSummary(summary string, now time.Time) bool {
	if summary == "" {
		return false
	}
	threshold := now.AddDate(0, 0, -30)
	currentYear := now.Year()
	thresholdYear := threshold.Year()
	if strings.Contains(summary, formatYear(currentYear)) {
		return true
	}
	if thresholdYear != currentYear && strings.Contains(summary, formatYear(thresholdYear)) {
		return true
	}
	return false
}

func formatYear(y int) string {
	// Tiny custom formatter so we don't pull fmt for a hot path.
	digits := []byte{
		byte('0' + (y/1000)%10),
		byte('0' + (y/100)%10),
		byte('0' + (y/10)%10),
		byte('0' + y%10),
	}
	return string(digits)
}
