package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SubGraph is the result of a neighborhood or path query.
type SubGraph struct {
	Nodes []NodeResult `json:"nodes"`
	Edges []EdgeResult `json:"edges"`
}

// NodeResult is a vertex returned from a graph query.
type NodeResult struct {
	ID         string            `json:"id"`
	Labels     []string          `json:"labels"`
	Properties map[string]any    `json:"properties"`
}

// EdgeResult is an edge returned from a graph query.
type EdgeResult struct {
	SourceID   string            `json:"source_id"`
	TargetID   string            `json:"target_id"`
	Type       string            `json:"type"`
	Properties map[string]any    `json:"properties"`
}

// Neighborhood returns all nodes and edges within maxHops of the given resource ID.
// Uses a PostgreSQL recursive CTE so it works even without AGE loaded.
func (g *Graph) Neighborhood(ctx context.Context, resourceID string, maxHops int) (*SubGraph, error) {
	if maxHops <= 0 {
		maxHops = 2
	}
	if maxHops > 5 {
		maxHops = 5 // cap to prevent runaway queries
	}

	// Collect all reachable node IDs via recursive CTE on the relationships table.
	rows, err := g.pool.Query(ctx, `
		WITH RECURSIVE reach AS (
			-- anchor: direct neighbours of the target node
			SELECT source_id, target_id, type, 1 AS depth
			FROM relationships
			WHERE source_id = $1 OR target_id = $1

			UNION ALL

			SELECT r.source_id, r.target_id, r.type, reach.depth + 1
			FROM relationships r
			JOIN reach ON r.source_id = reach.target_id
			WHERE reach.depth < $2
		)
		SELECT DISTINCT source_id, target_id, type FROM reach`,
		resourceID, maxHops,
	)
	if err != nil {
		return nil, fmt.Errorf("neighborhood query: %w", err)
	}
	defer rows.Close()

	nodeIDs := map[string]struct{}{resourceID: {}}
	var edges []EdgeResult

	for rows.Next() {
		var src, tgt, typ string
		if err := rows.Scan(&src, &tgt, &typ); err != nil {
			return nil, err
		}
		nodeIDs[src] = struct{}{}
		nodeIDs[tgt] = struct{}{}
		edges = append(edges, EdgeResult{SourceID: src, TargetID: tgt, Type: typ})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch the full resource records for all collected node IDs.
	ids := make([]string, 0, len(nodeIDs))
	for id := range nodeIDs {
		ids = append(ids, id)
	}

	nodeRows, err := g.pool.Query(ctx, `
		SELECT id, name, type, provider, region, tenant_id, properties
		FROM resources WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("fetch nodes: %w", err)
	}
	defer nodeRows.Close()

	var nodes []NodeResult
	for nodeRows.Next() {
		var id, name, resType, provider, region, tenantID string
		var propsJSON []byte
		if err := nodeRows.Scan(&id, &name, &resType, &provider, &region, &tenantID, &propsJSON); err != nil {
			return nil, err
		}
		props := map[string]any{
			"name": name, "type": resType, "provider": provider,
			"region": region, "tenant_id": tenantID,
		}
		if propsJSON != nil {
			var extra map[string]any
			if json.Unmarshal(propsJSON, &extra) == nil {
				for k, v := range extra {
					props[k] = v
				}
			}
		}
		nodes = append(nodes, NodeResult{
			ID:         id,
			Labels:     []string{"Resource", resType},
			Properties: props,
		})
	}
	if err := nodeRows.Err(); err != nil {
		return nil, err
	}

	return &SubGraph{Nodes: nodes, Edges: edges}, nil
}

// Cypher executes a raw Cypher query against the AGE graph and returns results
// as a slice of maps. Each column is returned as the map value.
// The query must use the graph's name internally (the caller should NOT prefix it).
// Results are JSON-decoded from agtype::text.
func (g *Graph) Cypher(ctx context.Context, query string, returnCols []string) ([]map[string]any, error) {
	conn, err := g.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, "$user", public;`); err != nil {
		return nil, fmt.Errorf("set search_path: %w", err)
	}

	// Build the AS clause: each return column scanned as text (cast from agtype).
	cols := make([]string, len(returnCols))
	for i, c := range returnCols {
		cols[i] = fmt.Sprintf("%s agtype", c)
	}
	asClause := strings.Join(cols, ", ")
	if asClause == "" {
		asClause = "result agtype"
	}

	sql := fmt.Sprintf(
		`SELECT * FROM cypher('%s', $$ %s $$) AS (%s)`,
		g.name, query, asClause,
	)

	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("cypher query: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(returnCols))
		for i, col := range returnCols {
			if i < len(vals) {
				// agtype comes back as []byte; decode as JSON.
				switch v := vals[i].(type) {
				case []byte:
					var parsed any
					if json.Unmarshal(v, &parsed) == nil {
						row[col] = parsed
					} else {
						row[col] = string(v)
					}
				case string:
					var parsed any
					if json.Unmarshal([]byte(v), &parsed) == nil {
						row[col] = parsed
					} else {
						row[col] = v
					}
				default:
					row[col] = v
				}
			}
		}
		results = append(results, row)
	}
	return results, rows.Err()
}
