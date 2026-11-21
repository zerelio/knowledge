package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zerelio/knowledge/pkg/types"
)

// SyncResource upserts a resource as an AGE vertex with label "Resource".
func (g *Graph) SyncResource(ctx context.Context, r *types.Resource) error {
	conn, err := g.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, "$user", public;`); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}

	props, _ := json.Marshal(map[string]any{
		"id":        r.ID,
		"name":      r.Name,
		"type":      r.Type,
		"provider":  r.Provider,
		"region":    r.Region,
		"tenant_id": r.TenantID,
	})

	cypher := fmt.Sprintf(`
		SELECT * FROM cypher('%s', $$
			MERGE (n:Resource {id: '%s'})
			SET n += %s
		$$) AS (result agtype)`,
		g.name,
		escapeCypher(r.ID),
		string(props),
	)

	if _, err := conn.Exec(ctx, cypher); err != nil {
		return fmt.Errorf("sync resource vertex %s: %w", r.ID, err)
	}
	return nil
}

// SyncRelationship upserts a directed edge between two Resource vertices.
func (g *Graph) SyncRelationship(ctx context.Context, rel *types.Relationship) error {
	conn, err := g.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, "$user", public;`); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}

	relType := sanitizeLabel(rel.Type)

	cypher := fmt.Sprintf(`
		SELECT * FROM cypher('%s', $$
			MATCH (a:Resource {id: '%s'}), (b:Resource {id: '%s'})
			MERGE (a)-[r:%s]->(b)
		$$) AS (result agtype)`,
		g.name,
		escapeCypher(rel.SourceID),
		escapeCypher(rel.TargetID),
		relType,
	)

	if _, err := conn.Exec(ctx, cypher); err != nil {
		// Non-fatal: source/target might not exist yet. Log and skip.
		g.logger.Warn("sync relationship skipped",
			"src", rel.SourceID, "tgt", rel.TargetID, "type", rel.Type, "error", err)
	}
	return nil
}

// SyncBatch syncs a batch of resources and relationships in one pass.
func (g *Graph) SyncBatch(ctx context.Context, resources []*types.Resource, rels []*types.Relationship) {
	for _, r := range resources {
		if err := g.SyncResource(ctx, r); err != nil {
			g.logger.Error("sync resource to graph", "id", r.ID, "error", err)
		}
	}
	for _, rel := range rels {
		if err := g.SyncRelationship(ctx, rel); err != nil {
			g.logger.Error("sync relationship to graph", "src", rel.SourceID, "tgt", rel.TargetID, "error", err)
		}
	}
}

// escapeCypher escapes single quotes for use inside AGE Cypher string literals.
func escapeCypher(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}

// sanitizeLabel converts a relationship type like "aws:uses" into a valid
// AGE label "aws__uses" (labels cannot contain colons or hyphens).
func sanitizeLabel(s string) string {
	r := strings.NewReplacer(":", "__", "-", "_", ".", "_", " ", "_")
	return r.Replace(s)
}
