package docs

import (
	"fmt"
	"strings"

	"github.com/zerelio/knowledge/internal/graph"
	"github.com/zerelio/knowledge/pkg/types"
)

// ContextPacker assembles a text context block from a resource and its graph
// neighbourhood, respecting a token budget.
//
// Token estimation uses 4 chars ≈ 1 token (rough OpenAI approximation).
// Content is ordered: core resource → direct neighbours → 2-hop neighbours.
type ContextPacker struct {
	maxTokens int // approximate max tokens for the packed context
}

// Pack produces a text representation of the resource and its neighbourhood,
// trimmed to the token budget.
func (p *ContextPacker) Pack(r *types.Resource, subgraph *graph.SubGraph) string {
	budget := p.maxTokens * 4 // chars

	var sb strings.Builder

	// Core resource block.
	core := p.resourceBlock(r)
	sb.WriteString(core)
	budget -= len(core)

	if subgraph == nil || budget <= 0 {
		return sb.String()
	}

	// Relationships section.
	if len(subgraph.Edges) > 0 {
		rel := p.edgesBlock(subgraph.Edges)
		if len(rel) <= budget {
			sb.WriteString(rel)
			budget -= len(rel)
		}
	}

	// Neighbouring nodes (up to budget).
	if len(subgraph.Nodes) > 1 {
		header := "\n## Neighbours\n\n"
		if len(header) <= budget {
			sb.WriteString(header)
			budget -= len(header)
		}
		for _, n := range subgraph.Nodes {
			if n.ID == r.ID {
				continue
			}
			line := p.nodeOneLiner(n)
			if len(line)+1 > budget {
				sb.WriteString("\n_(context truncated to fit token budget)_\n")
				break
			}
			sb.WriteString(line)
			sb.WriteByte('\n')
			budget -= len(line) + 1
		}
	}

	return sb.String()
}

func (p *ContextPacker) resourceBlock(r *types.Resource) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Resource: %s\n\n", r.Name))
	sb.WriteString(fmt.Sprintf("- **ID:** `%s`\n", r.ID))
	sb.WriteString(fmt.Sprintf("- **Type:** %s\n", r.Type))
	sb.WriteString(fmt.Sprintf("- **Provider:** %s\n", r.Provider))
	if r.Region != "" {
		sb.WriteString(fmt.Sprintf("- **Region:** %s\n", r.Region))
	}
	if r.AccountID != "" {
		sb.WriteString(fmt.Sprintf("- **Account:** %s\n", r.AccountID))
	}
	if len(r.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("- **Tags:** %s\n", strings.Join(r.Tags, ", ")))
	}
	if len(r.Properties) > 0 {
		sb.WriteString("\n### Properties\n\n")
		for k, v := range r.Properties {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
	}
	return sb.String()
}

func (p *ContextPacker) edgesBlock(edges []graph.EdgeResult) string {
	if len(edges) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n## Relationships\n\n")
	for _, e := range edges {
		sb.WriteString(fmt.Sprintf("- `%s` -[%s]-> `%s`\n", e.SourceID, e.Type, e.TargetID))
	}
	return sb.String()
}

func (p *ContextPacker) nodeOneLiner(n graph.NodeResult) string {
	typ, _ := n.Properties["type"].(string)
	name, _ := n.Properties["name"].(string)
	if name == "" {
		name = n.ID
	}
	return fmt.Sprintf("- `%s` (%s) — %s", name, typ, n.ID)
}
