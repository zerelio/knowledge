// Package docs provides LLM-powered documentation generation for discovered
// infrastructure resources.
//
// Two primary outputs:
//   - Resource dossier: a structured markdown document describing a single
//     resource, its context (neighbourhood), and known relationships.
//   - Runbook: an operational procedure generated for a named scenario
//     (e.g. "how to debug a failing pod", "RDS failover procedure").
//
// Context assembly follows the research principle: pack the graph neighbourhood
// of the resource into the LLM context window, ordered by proximity and
// semantic relevance, respecting a configurable token budget.
package docs

import (
	"context"
	"fmt"
	"strings"

	"github.com/zerelio/zerelio/internal/graph"
	"github.com/zerelio/zerelio/internal/llm"
	"github.com/zerelio/zerelio/internal/storage"
)

// Document is a generated markdown document about a resource.
type Document struct {
	ResourceID string `json:"resource_id"`
	Title      string `json:"title"`
	Content    string `json:"content"` // markdown
	TokensUsed int    `json:"tokens_used"`
}

// Runbook is a generated operational procedure.
type Runbook struct {
	Title     string   `json:"title"`
	Scenario  string   `json:"scenario"`
	Resources []string `json:"resource_ids"`
	Content   string   `json:"content"` // markdown
}

// Generator produces documents and runbooks using an LLM.
type Generator struct {
	llm    llm.Provider
	store  storage.Backend
	graph  *graph.Graph
	packer *ContextPacker
}

// New creates a Generator.
func New(provider llm.Provider, store storage.Backend, g *graph.Graph) *Generator {
	return &Generator{
		llm:    provider,
		store:  store,
		graph:  g,
		packer: &ContextPacker{maxTokens: 6000},
	}
}

// GenerateResourceDoc produces a dossier for the given resource.
// It assembles graph context (neighbourhood + relationships) then calls the LLM.
func (gen *Generator) GenerateResourceDoc(ctx context.Context, resourceID string) (*Document, error) {
	r, err := gen.store.GetResourceByID(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("get resource: %w", err)
	}

	// Fetch graph neighbourhood for context.
	var subgraph *graph.SubGraph
	if gen.graph != nil {
		subgraph, _ = gen.graph.Neighborhood(ctx, resourceID, 2)
	}

	packed := gen.packer.Pack(r, subgraph)

	system := `You are a technical documentation writer for infrastructure knowledge bases.
Write clear, accurate, and concise markdown documentation about the infrastructure resource provided.
Use only the information given — do not invent details. Format as professional technical documentation.`

	prompt := fmt.Sprintf(`Generate a technical dossier for this infrastructure resource.

%s

Write a markdown document with sections:
1. Overview (what it is, its role)
2. Configuration (key settings visible in the data)
3. Relationships (what it connects to and how)
4. Operational Notes (things an engineer should know)

Keep it factual and grounded in the data provided.`, packed)

	content, err := gen.llm.Complete(ctx, system, []llm.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("generate doc: %w", err)
	}

	return &Document{
		ResourceID: resourceID,
		Title:      fmt.Sprintf("%s — %s", r.Name, r.Type),
		Content:    content,
	}, nil
}

// GenerateRunbook produces an operational runbook for a scenario.
// resourceIDs provides the resources involved (can be empty for general scenarios).
func (gen *Generator) GenerateRunbook(ctx context.Context, scenario string, resourceIDs []string) (*Runbook, error) {
	// Gather context for each named resource.
	var contextBlocks []string
	for _, id := range resourceIDs {
		r, err := gen.store.GetResourceByID(ctx, id)
		if err != nil {
			continue
		}
		var subgraph *graph.SubGraph
		if gen.graph != nil {
			subgraph, _ = gen.graph.Neighborhood(ctx, id, 1)
		}
		contextBlocks = append(contextBlocks, gen.packer.Pack(r, subgraph))
	}

	resourceContext := ""
	if len(contextBlocks) > 0 {
		resourceContext = "\n\n## Involved Resources\n\n" + strings.Join(contextBlocks, "\n\n---\n\n")
	}

	system := `You are a senior site reliability engineer writing operational runbooks.
Runbooks must be clear, numbered, actionable, and safe. Include verification steps.
Use the infrastructure context provided. Format as markdown.`

	prompt := fmt.Sprintf(`Write an operational runbook for the following scenario:

**Scenario:** %s
%s

Structure the runbook as:
1. Prerequisites (what must be in place before starting)
2. Steps (numbered, with specific commands or actions)
3. Verification (how to confirm success)
4. Rollback (how to undo if something goes wrong)
5. Escalation (when to escalate and to whom)`, scenario, resourceContext)

	content, err := gen.llm.Complete(ctx, system, []llm.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("generate runbook: %w", err)
	}

	return &Runbook{
		Title:     "Runbook: " + scenario,
		Scenario:  scenario,
		Resources: resourceIDs,
		Content:   content,
	}, nil
}
