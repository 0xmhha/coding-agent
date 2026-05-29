package ckg

import (
	"context"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// Direction controls whether traversal follows outgoing or incoming edges.
type Direction string

const (
	DirectionOut Direction = "out" // follow from_node → to_node
	DirectionIn  Direction = "in"  // follow to_node ← from_node (callers)
)

// TraversalRequest controls one BFS run.
type TraversalRequest struct {
	StartNodes    []string         // node IDs to seed
	Depth         int              // 0 = start nodes only, 1 = +1 hop, ...
	RelationTypes []types.RelationType
	Direction     Direction
	MaxNodes      int // hard cap (default 200)
	MaxEdges      int // hard cap (default 500)
}

// TraversalResult is the structured BFS output.
type TraversalResult struct {
	Nodes     []types.GraphNode
	Edges     []types.GraphEdge
	NodeDepth map[string]int // node ID → BFS depth (0 for seeds)
	Truncated bool
}

// BFS performs a breadth-first traversal honoring depth, relation, and
// size caps. We run it in Go rather than as a single recursive CTE so we
// can apply per-step max_nodes / max_edges caps cleanly.
func (s *Store) BFS(ctx context.Context, req TraversalRequest) (*TraversalResult, error) {
	if req.MaxNodes <= 0 {
		req.MaxNodes = 200
	}
	if req.MaxEdges <= 0 {
		req.MaxEdges = 500
	}
	if req.Depth < 0 {
		req.Depth = 0
	}
	if req.Direction == "" {
		req.Direction = DirectionOut
	}

	relTypes := make([]string, 0, len(req.RelationTypes))
	for _, rt := range req.RelationTypes {
		relTypes = append(relTypes, string(rt))
	}

	visited := make(map[string]int, len(req.StartNodes))
	frontier := req.StartNodes
	for _, id := range frontier {
		visited[id] = 0
	}

	result := &TraversalResult{
		NodeDepth: visited,
	}

	for depth := 0; depth < req.Depth; depth++ {
		if len(frontier) == 0 {
			break
		}
		var edges []types.GraphEdge
		var err error
		if req.Direction == DirectionIn {
			edges, err = s.EdgesByToNodes(ctx, frontier, relTypes)
		} else {
			edges, err = s.EdgesByFromNodes(ctx, frontier, relTypes)
		}
		if err != nil {
			return nil, err
		}

		// Accumulate edges, respecting MaxEdges.
		nextFrontier := make([]string, 0, len(edges))
		for _, e := range edges {
			if len(result.Edges) >= req.MaxEdges {
				result.Truncated = true
				break
			}
			result.Edges = append(result.Edges, e)

			var nextID string
			if req.Direction == DirectionIn {
				nextID = e.FromNode
			} else {
				nextID = e.ToNode
			}
			if _, seen := visited[nextID]; seen {
				continue
			}
			if len(visited) >= req.MaxNodes {
				result.Truncated = true
				continue
			}
			visited[nextID] = depth + 1
			nextFrontier = append(nextFrontier, nextID)
		}
		frontier = nextFrontier
	}

	// Resolve nodes for the visited set.
	ids := make([]string, 0, len(visited))
	for id := range visited {
		ids = append(ids, id)
	}
	nodes, err := s.GetNodes(ctx, ids)
	if err != nil {
		return nil, err
	}
	result.Nodes = nodes
	return result, nil
}
