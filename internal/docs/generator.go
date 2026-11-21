// Package docs provides LLM-powered documentation generation with:
//   - Document caching (invalidated by resource changes)
//   - Full generation logging for auditability
//   - Few-shot injection from human-approved examples
//   - Human feedback loop integration
package docs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zerelio/knowledge/internal/graph"
	"github.com/zerelio/knowledge/internal/llm"
	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

// Document is a generated markdown document about a resource.
type Document struct {
	ResourceID   string               `json:"resource_id"`
	Title        string               `json:"title"`
	Content      string               `json:"content"` // markdown
	GenerationID string               `json:"generation_id"`
	FromCache    bool                 `json:"from_cache"`
	Status       types.DocumentStatus `json:"status"`
}

// Runbook is a generated operational procedure.
type Runbook struct {
	Title        string   `json:"title"`
	Scenario     string   `json:"scenario"`
	Resources    []string `json:"resource_ids"`
	Content      string   `json:"content"` // markdown
	GenerationID string   `json:"generation_id"`
}

// Generator produces documents and runbooks using an LLM.
// It logs every generation, caches results, and injects few-shot examples
// drawn from human-approved prior generations.
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
// Returns from cache if a valid cached document exists (non-rejected status).
func (gen *Generator) GenerateResourceDoc(ctx context.Context, resourceID string) (*Document, error) {
	r, err := gen.store.GetResourceByID(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("get resource: %w", err)
	}

	// Cache check — serve approved/draft docs without calling the LLM.
	if cached, err := gen.store.GetDocument(ctx, resourceID); err == nil {
		if cached.Status != types.DocumentRejected {
			return &Document{
				ResourceID:   resourceID,
				Title:        cached.Title,
				Content:      cached.Content,
				GenerationID: cached.GenerationID,
				FromCache:    true,
				Status:       cached.Status,
			}, nil
		}
	}

	// Fetch neighbourhood for context.
	var subgraph *graph.SubGraph
	if gen.graph != nil {
		subgraph, _ = gen.graph.Neighborhood(ctx, resourceID, 2)
	}
	packed := gen.packer.Pack(r, subgraph)

	// Few-shot injection from approved examples of the same resource type.
	fewShot := gen.buildFewShotExamples(ctx, types.GenerationTypeDoc, r.Type)

	system := `You are a technical documentation writer for infrastructure knowledge bases.
Write clear, accurate, concise markdown documentation grounded only in the information provided.
Do not invent or assume details that are not present in the context.`

	prompt := fmt.Sprintf(`Generate a technical dossier for this infrastructure resource.

%s%s

Write a markdown document with sections:
1. Overview (what it is, its role in the infrastructure)
2. Configuration (key settings visible in the data)
3. Relationships (what it connects to and how)
4. Operational Notes (important things an engineer should know)

Stay factual. Only use the data provided.`, packed, fewShot)

	start := time.Now()
	content, err := gen.llm.Complete(ctx, system, []llm.Message{{Role: "user", Content: prompt}})
	latencyMS := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("generate doc: %w", err)
	}

	title := fmt.Sprintf("%s — %s", r.Name, r.Type)
	genID := fmt.Sprintf("gen_%d", start.UnixNano())

	gen.logGeneration(ctx, &types.GenerationLog{
		ID:          genID,
		Type:        types.GenerationTypeDoc,
		ResourceID:  resourceID,
		ContextText: packed,
		Model:       gen.llm.Name(),
		Provider:    gen.llm.Name(),
		LatencyMS:   latencyMS,
		Output:      content,
		CreatedAt:   start,
	})

	now := time.Now()
	gen.store.UpsertDocument(ctx, &types.CachedDocument{ //nolint:errcheck
		ID:           genID,
		ResourceID:   resourceID,
		Title:        title,
		Content:      content,
		GenerationID: genID,
		Status:       types.DocumentDraft,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	return &Document{
		ResourceID:   resourceID,
		Title:        title,
		Content:      content,
		GenerationID: genID,
		FromCache:    false,
		Status:       types.DocumentDraft,
	}, nil
}

// GenerateRunbook produces an operational runbook for a scenario.
func (gen *Generator) GenerateRunbook(ctx context.Context, scenario string, resourceIDs []string) (*Runbook, error) {
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

	fewShot := gen.buildFewShotExamples(ctx, types.GenerationTypeRunbook, "")

	system := `You are a senior site reliability engineer writing operational runbooks.
Runbooks must be clear, numbered, actionable, and safe. Include verification steps.
Use the infrastructure context provided. Format as markdown.`

	prompt := fmt.Sprintf(`Write an operational runbook for the following scenario:

**Scenario:** %s
%s%s

Structure:
1. Prerequisites
2. Steps (numbered, with specific commands or actions)
3. Verification
4. Rollback
5. Escalation`, scenario, resourceContext, fewShot)

	start := time.Now()
	content, err := gen.llm.Complete(ctx, system, []llm.Message{{Role: "user", Content: prompt}})
	latencyMS := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("generate runbook: %w", err)
	}

	genID := fmt.Sprintf("gen_%d", start.UnixNano())

	gen.logGeneration(ctx, &types.GenerationLog{
		ID:           genID,
		Type:         types.GenerationTypeRunbook,
		Query:        scenario,
		RetrievedIDs: resourceIDs,
		ContextText:  resourceContext,
		Model:        gen.llm.Name(),
		Provider:     gen.llm.Name(),
		LatencyMS:    latencyMS,
		Output:       content,
		CreatedAt:    start,
	})

	return &Runbook{
		Title:        "Runbook: " + scenario,
		Scenario:     scenario,
		Resources:    resourceIDs,
		Content:      content,
		GenerationID: genID,
	}, nil
}

// buildFewShotExamples retrieves approved examples and formats them as a
// few-shot section appended to the prompt.
func (gen *Generator) buildFewShotExamples(ctx context.Context, genType types.GenerationType, hint string) string {
	examples, err := gen.store.GetApprovedExamples(ctx, genType, hint, 2)
	if err != nil || len(examples) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n---\n## Examples of approved outputs\n\n")
	for i, ex := range examples {
		sb.WriteString(fmt.Sprintf("### Example %d\n\n%s\n\n", i+1, ex.Output))
	}
	sb.WriteString("---\n\nGenerate for the resource above, following the same style and quality.\n")
	return sb.String()
}

func (gen *Generator) logGeneration(ctx context.Context, log *types.GenerationLog) {
	gen.store.LogGeneration(ctx, log) //nolint:errcheck
}
