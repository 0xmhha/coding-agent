package ckg

import (
	"context"
	"strings"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// QueryService implements ckg_query and ckg_impact.
type QueryService struct {
	store *Store
}

// NewQueryService constructs the service.
func NewQueryService(store *Store) *QueryService {
	return &QueryService{store: store}
}

// QueryRequest mirrors the ckg_query MCP input shape.
type QueryRequest struct {
	Symbols            []string
	Depth              int
	RelationTypes      []types.RelationType
	IncludeHistory     bool
	IncludeConcurrency bool
	MaxNodes           int
	MaxEdges           int
}

// Query resolves seed symbols, runs the BFS, and optionally enriches with
// history and concurrency context.
func (q *QueryService) Query(ctx context.Context, req QueryRequest) (*types.CKGQueryResult, error) {
	if req.Depth == 0 {
		req.Depth = 2
	}
	if req.MaxNodes <= 0 {
		req.MaxNodes = 200
	}
	if req.MaxEdges <= 0 {
		req.MaxEdges = 500
	}

	start := time.Now()
	seedIDs, err := q.resolveSeeds(ctx, req.Symbols)
	if err != nil {
		return nil, err
	}

	trav, err := q.store.BFS(ctx, TraversalRequest{
		StartNodes:    seedIDs,
		Depth:         req.Depth,
		RelationTypes: req.RelationTypes,
		Direction:     DirectionOut,
		MaxNodes:      req.MaxNodes,
		MaxEdges:      req.MaxEdges,
	})
	if err != nil {
		return nil, err
	}

	result := &types.CKGQueryResult{
		Nodes: trav.Nodes,
		Edges: trav.Edges,
		Metadata: types.CKGQueryMetadata{
			TotalNodes:  len(trav.Nodes),
			TotalEdges:  len(trav.Edges),
			Truncated:   trav.Truncated,
			QueryTimeMs: time.Since(start).Milliseconds(),
		},
	}

	if req.IncludeHistory {
		for _, n := range trav.Nodes {
			hist, _ := q.store.HistoryForNode(ctx, n.ID, 5)
			result.History = append(result.History, hist...)
		}
	}
	if req.IncludeConcurrency {
		for _, n := range trav.Nodes {
			cc, _ := q.store.ConcurrencyForNode(ctx, n.ID)
			if cc.NodeID == "" {
				continue
			}
			result.ConcurrencyImpact = append(result.ConcurrencyImpact, cc)
		}
	}
	return result, nil
}

// ImpactRequest mirrors ckg_impact input.
type ImpactRequest struct {
	Symbol     string
	ChangeType string // signature | logic | delete
}

// Impact analyzes the blast radius of modifying the given symbol.
func (q *QueryService) Impact(ctx context.Context, req ImpactRequest) (*types.CKGImpactResult, error) {
	seedIDs, err := q.resolveSeeds(ctx, []string{req.Symbol})
	if err != nil {
		return nil, err
	}
	result := &types.CKGImpactResult{Symbol: req.Symbol}
	if len(seedIDs) == 0 {
		return result, nil
	}

	// Direct callers: reverse traverse depth 1 on calls.
	directCallers, err := q.callersAtDepth(ctx, seedIDs, 1)
	if err != nil {
		return nil, err
	}
	result.DirectCallers = directCallers

	// Indirect callers: depth 2.
	allCallers, err := q.callersAtDepth(ctx, seedIDs, 3)
	if err != nil {
		return nil, err
	}
	directSet := map[string]struct{}{}
	for _, c := range directCallers {
		directSet[c] = struct{}{}
	}
	for _, c := range allCallers {
		if _, ok := directSet[c]; ok {
			continue
		}
		result.IndirectCallers = append(result.IndirectCallers, c)
	}

	// Interface contracts: outgoing implements edges.
	implEdges, err := q.store.EdgesByFromNodes(ctx, seedIDs, []string{string(types.RelImplements)})
	if err != nil {
		return nil, err
	}
	implTargetIDs := make([]string, 0, len(implEdges))
	for _, e := range implEdges {
		implTargetIDs = append(implTargetIDs, e.ToNode)
	}
	implNodes, err := q.store.GetNodes(ctx, implTargetIDs)
	if err != nil {
		return nil, err
	}
	for _, n := range implNodes {
		result.InterfaceContracts = append(result.InterfaceContracts, n.QualifiedName)
	}

	// Test files: callers whose file ends with _test.go.
	allCallerNodes, err := q.store.GetNodes(ctx, allCallers)
	if err != nil {
		return nil, err
	}
	testSet := map[string]struct{}{}
	for _, n := range allCallerNodes {
		if strings.HasSuffix(n.FilePath, "_test.go") {
			testSet[n.FilePath] = struct{}{}
		}
	}
	for f := range testSet {
		result.TestFiles = append(result.TestFiles, f)
	}

	// Concurrency risk.
	for _, id := range seedIDs {
		cc, _ := q.store.ConcurrencyForNode(ctx, id)
		if cc.NodeID == "" {
			continue
		}
		result.ConcurrencyRisk.AffectedGoroutines = append(
			result.ConcurrencyRisk.AffectedGoroutines, cc.GoroutineContext.Launches...)
		for _, sr := range cc.SharedResources {
			result.ConcurrencyRisk.SharedResourceConflicts = append(
				result.ConcurrencyRisk.SharedResourceConflicts, sr.Resource)
		}
	}

	// Recommended test scope: union of test file paths + caller packages.
	scope := map[string]struct{}{}
	for _, n := range allCallerNodes {
		scope[n.PackageName] = struct{}{}
	}
	for s := range scope {
		result.RecommendedTestScope = append(result.RecommendedTestScope, s)
	}

	// Risk level: weight by change type + caller count + concurrency.
	result.RiskLevel, result.RiskExplanation = classifyRisk(req.ChangeType, allCallers,
		result.ConcurrencyRisk)
	return result, nil
}

func (q *QueryService) callersAtDepth(ctx context.Context, seeds []string, depth int) ([]string, error) {
	trav, err := q.store.BFS(ctx, TraversalRequest{
		StartNodes:    seeds,
		Depth:         depth,
		RelationTypes: []types.RelationType{types.RelCalls},
		Direction:     DirectionIn,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(trav.Nodes))
	for _, n := range trav.Nodes {
		// Skip the seed nodes themselves.
		if isSeed(seeds, n.ID) {
			continue
		}
		out = append(out, n.QualifiedName)
	}
	return out, nil
}

func isSeed(seeds []string, id string) bool {
	for _, s := range seeds {
		if s == id {
			return true
		}
	}
	return false
}

func (q *QueryService) resolveSeeds(ctx context.Context, symbols []string) ([]string, error) {
	var ids []string
	for _, s := range symbols {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// Exact qualified name first.
		if id, err := q.store.NodeByQualifiedName(ctx, s); err != nil {
			return nil, err
		} else if id != "" {
			ids = append(ids, id)
			continue
		}
		// Short-name fallback: LIKE match top-1 candidate.
		nodes, err := q.store.NodesByLikeName(ctx, s+"%", 1)
		if err != nil {
			return nil, err
		}
		if len(nodes) > 0 {
			ids = append(ids, nodes[0].ID)
		}
	}
	return ids, nil
}

func classifyRisk(
	changeType string, allCallers []string, conc types.ConcurrencyImpact,
) (level, explanation string) {
	callerCount := len(allCallers)
	switch changeType {
	case "delete":
		if callerCount > 0 {
			return "critical", "delete change with callers will break the build"
		}
		return "medium", "delete change with no callers detected"
	case "signature":
		if callerCount > 5 {
			return "high", "signature change affects many callers; interface contracts likely broken"
		}
		if callerCount > 0 {
			return "medium", "signature change has callers; verify each"
		}
		return "low", "signature change with no callers detected"
	default: // "logic" or empty
		if len(conc.SharedResourceConflicts) > 0 {
			return "high", "logic change touches shared concurrent state"
		}
		if callerCount > 10 {
			return "medium", "logic change has broad call graph"
		}
		return "low", "logic change with limited blast radius"
	}
}
