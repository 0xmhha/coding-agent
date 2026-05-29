package ckv

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// BM25 parameters chosen for code search:
//   k1=1.2 (term frequency saturation) and b=0.75 (length normalization).
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// BM25SearchOverChunks scores chunks lexically using BM25 over the
// (signature + godoc + code) text. It is the RI-08 fallback path when no
// embedder is available.
//
// chunks must be the full candidate set (already filtered if needed).
func BM25SearchOverChunks(query string, chunks []chunkRow, topK int) []types.SearchResult {
	if len(chunks) == 0 {
		return nil
	}
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Pre-tokenize every chunk.
	docTokens := make([][]string, len(chunks))
	docLens := make([]float64, len(chunks))
	var totalLen float64
	for i, c := range chunks {
		text := c.Signature + " " + c.Godoc + " " + c.Code
		docTokens[i] = tokenize(text)
		docLens[i] = float64(len(docTokens[i]))
		totalLen += docLens[i]
	}
	avgLen := totalLen / float64(len(chunks))
	if avgLen == 0 {
		avgLen = 1
	}

	// Document frequency per query term.
	df := make(map[string]int, len(queryTokens))
	uniqueQuery := uniqueStrings(queryTokens)
	for _, qt := range uniqueQuery {
		for _, dt := range docTokens {
			if containsToken(dt, qt) {
				df[qt]++
			}
		}
	}

	N := float64(len(chunks))
	scores := make([]float64, len(chunks))
	for i, dt := range docTokens {
		tf := termFreq(dt)
		var score float64
		for _, qt := range uniqueQuery {
			if df[qt] == 0 {
				continue
			}
			idf := math.Log(1 + (N-float64(df[qt])+0.5)/(float64(df[qt])+0.5))
			f := float64(tf[qt])
			denom := f + bm25K1*(1-bm25B+bm25B*docLens[i]/avgLen)
			if denom == 0 {
				continue
			}
			score += idf * (f * (bm25K1 + 1) / denom)
		}
		scores[i] = score
	}

	// Build result list sorted by score.
	type idx struct {
		i     int
		score float64
	}
	indices := make([]idx, len(chunks))
	for i, s := range scores {
		indices[i] = idx{i: i, score: s}
	}
	sort.SliceStable(indices, func(i, j int) bool { return indices[i].score > indices[j].score })

	limit := topK
	if limit <= 0 || limit > len(indices) {
		limit = len(indices)
	}

	out := make([]types.SearchResult, 0, limit)
	for _, p := range indices[:limit] {
		if p.score <= 0 {
			continue
		}
		out = append(out, toSearchResult(chunks[p.i], p.score))
	}
	return out
}

// tokenize lowercases and splits on non-letter/digit and underscore so that
// camelCase identifiers (e.g. GetStakerInfo) split into ["get", "staker", "info"]
// while remaining understandable for natural-language queries.
func tokenize(text string) []string {
	if text == "" {
		return nil
	}
	out := make([]string, 0, len(text)/4)
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, string(current))
		current = current[:0]
	}
	prevLower := false
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// camelCase split: lowercase→Uppercase boundary
			if prevLower && unicode.IsUpper(r) {
				flush()
			}
			current = append(current, unicode.ToLower(r))
			prevLower = unicode.IsLower(r) || unicode.IsDigit(r)
		default:
			flush()
			prevLower = false
		}
	}
	flush()

	// Filter very short tokens and trivial English stopwords that hurt code search.
	filtered := out[:0]
	for _, t := range out {
		if len(t) < 2 {
			continue
		}
		if isStopword(t) {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

var codeStopwords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "into": {},
	"that": {}, "this": {}, "these": {}, "those": {}, "are": {}, "was": {},
	"were": {}, "has": {}, "have": {}, "had": {}, "but": {}, "not": {},
}

func isStopword(t string) bool {
	_, ok := codeStopwords[strings.ToLower(t)]
	return ok
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func termFreq(tokens []string) map[string]int {
	freq := make(map[string]int, len(tokens))
	for _, t := range tokens {
		freq[t]++
	}
	return freq
}

func containsToken(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
