// Package knowledge implements community-level summarization of infrastructure
// resources for Zerelio's dual-level retrieval architecture.
//
// Motivation (LightRAG dual-level retrieval):
//
//	Entity level  — search returns individual resources (already implemented
//	                via BM25 + pgvector + graph proximity with RRF fusion).
//	Community level — high-level queries ("What does our AWS setup look like?")
//	                need pre-computed abstractions over groups of resources, not
//	                per-entity matches.
//
// The Summarizer groups resources by (scope, type_prefix) and calls the LLM
// to generate a 2–4 sentence prose summary for each cluster. Summaries are
// upserted after each discovery run. When an LLM is not configured, the
// summarizer falls back to a statistical summary (counts, regions, top names).
//
// # Trigger
//
// SummarizeScope is called by the discovery Orchestrator in a background
// goroutine after each successful run, the same way compliance evaluation is.
package knowledge

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/zerelio/knowledge/internal/llm"
	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

// Summarizer generates and stores community-level infrastructure summaries.
type Summarizer struct {
	store  storage.Backend
	llm    llm.Provider
	logger *slog.Logger
}

// New creates a Summarizer. llmProvider may be a NoopProvider when LLM is
// not configured; the summarizer falls back to statistical summaries.
func New(store storage.Backend, llmProvider llm.Provider, logger *slog.Logger) *Summarizer {
	if llmProvider == nil {
		llmProvider = &llm.NoopProvider{}
	}
	return &Summarizer{store: store, llm: llmProvider, logger: logger}
}

// SummarizeScope generates or refreshes all community summaries for a scope.
// It runs after discovery completes; errors are logged but never propagate.
func (s *Summarizer) SummarizeScope(ctx context.Context, scopeID, tenantID, scopeName string) {
	resources, _, err := s.store.GetResources(ctx, storage.ResourceFilter{
		TenantID: tenantID,
		Limit:    5000,
	})
	if err != nil {
		s.logger.Warn("knowledge: failed to load resources", "scope_id", scopeID, "error", err)
		return
	}

	// Filter to this scope's resources.
	scoped := append([]*types.Resource(nil), resources...)
	if len(scoped) == 0 {
		return
	}

	// Group by type prefix (first two segments: "aws:ec2", "k8s:pod", etc.)
	groups := groupByTypePrefix(scoped)

	// Generate and store one summary per group, plus an "all" overview.
	generated := 0
	for prefix, group := range groups {
		if len(group) == 0 {
			continue
		}
		summary := s.generateSummary(ctx, scopeName, prefix, group)
		if err := s.store.StoreKnowledgeSummary(ctx, &types.KnowledgeSummary{
			ID:            fmt.Sprintf("ks_%s_%s", scopeID, strings.ReplaceAll(prefix, ":", "_")),
			ScopeID:       scopeID,
			TenantID:      tenantID,
			TypePrefix:    prefix,
			ScopeName:     scopeName,
			Summary:       summary,
			ResourceCount: len(group),
			GeneratedAt:   time.Now(),
		}); err != nil {
			s.logger.Warn("knowledge: failed to store summary", "scope_id", scopeID, "prefix", prefix, "error", err)
		} else {
			generated++
		}
	}

	// Generate an "all" overview summary for the whole scope.
	overviewSummary := s.generateOverview(ctx, scopeName, scoped)
	if err := s.store.StoreKnowledgeSummary(ctx, &types.KnowledgeSummary{
		ID:            fmt.Sprintf("ks_%s_all", scopeID),
		ScopeID:       scopeID,
		TenantID:      tenantID,
		TypePrefix:    "all",
		ScopeName:     scopeName,
		Summary:       overviewSummary,
		ResourceCount: len(scoped),
		GeneratedAt:   time.Now(),
	}); err != nil {
		s.logger.Warn("knowledge: failed to store overview summary", "scope_id", scopeID, "error", err)
	} else {
		generated++
	}

	s.logger.Info("knowledge summaries generated",
		"scope_id", scopeID,
		"resources", len(scoped),
		"summaries", generated,
	)
}

// generateSummary produces a prose description of a resource group.
// Falls back to a statistical description when LLM is not configured.
func (s *Summarizer) generateSummary(ctx context.Context, scopeName, typePrefix string, resources []*types.Resource) string {
	if llm.IsNoop(s.llm) {
		return statisticalSummary(scopeName, typePrefix, resources)
	}

	resourceList := buildResourceList(resources, 20)
	prompt := fmt.Sprintf(
		"You are summarizing infrastructure resources for a knowledge graph.\n"+
			"Describe these %d %s resources from the \"%s\" scope in 2-3 sentences.\n"+
			"Focus on: what they are, their configuration patterns, and notable details.\n"+
			"Be specific and technical. Do not use bullet points or headers.\n\n"+
			"Resources:\n%s\n\nSummary:",
		len(resources), typePrefix, scopeName, resourceList,
	)

	resp, err := s.llm.Complete(ctx,
		"You are an infrastructure documentation assistant that generates concise technical summaries.",
		[]llm.Message{{Role: "user", Content: prompt}},
	)
	if err != nil {
		s.logger.Warn("knowledge: LLM summary failed, using statistical fallback",
			"prefix", typePrefix, "error", err)
		return statisticalSummary(scopeName, typePrefix, resources)
	}
	return strings.TrimSpace(resp)
}

// generateOverview produces a high-level scope summary across all resource types.
func (s *Summarizer) generateOverview(ctx context.Context, scopeName string, resources []*types.Resource) string {
	if llm.IsNoop(s.llm) {
		return statisticalOverview(scopeName, resources)
	}

	// Build a compact breakdown: type → count
	typeCounts := make(map[string]int)
	for _, r := range resources {
		typeCounts[r.Type]++
	}
	breakdown := buildBreakdown(typeCounts, 10)

	prompt := fmt.Sprintf(
		"You are summarizing an infrastructure scope for a knowledge graph.\n"+
			"The \"%s\" scope contains %d resources. Describe its overall structure "+
			"and what this infrastructure does in 3-4 sentences.\n"+
			"Be specific, technical, and mention the most significant resource types.\n\n"+
			"Resource breakdown:\n%s\n\nScope summary:",
		scopeName, len(resources), breakdown,
	)

	resp, err := s.llm.Complete(ctx,
		"You are an infrastructure documentation assistant that generates concise technical summaries.",
		[]llm.Message{{Role: "user", Content: prompt}},
	)
	if err != nil {
		return statisticalOverview(scopeName, resources)
	}
	return strings.TrimSpace(resp)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// groupByTypePrefix clusters resources by their two-segment type prefix.
// Resources with single-segment types (no colon) use the full type as the key.
func groupByTypePrefix(resources []*types.Resource) map[string][]*types.Resource {
	groups := make(map[string][]*types.Resource)
	for _, r := range resources {
		prefix := typePrefix(r.Type)
		groups[prefix] = append(groups[prefix], r)
	}
	return groups
}

func typePrefix(t string) string {
	parts := strings.SplitN(t, ":", 3)
	if len(parts) >= 2 {
		return parts[0] + ":" + parts[1]
	}
	return t
}

// buildResourceList formats up to maxItems resources for an LLM prompt.
func buildResourceList(resources []*types.Resource, maxItems int) string {
	if len(resources) > maxItems {
		resources = resources[:maxItems]
	}
	var sb strings.Builder
	for i, r := range resources {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, r.Type, r.Name))
		// Include a few key properties if available.
		for _, k := range []string{"region", "size", "instance_type", "namespace", "status"} {
			if v, ok := r.Properties[k]; ok && v != "" {
				sb.WriteString(fmt.Sprintf(" (%s: %s)", k, v))
				break
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// buildBreakdown formats a type→count map as a compact list for LLM prompts.
func buildBreakdown(typeCounts map[string]int, maxTypes int) string {
	type entry struct {
		t string
		n int
	}
	entries := make([]entry, 0, len(typeCounts))
	for t, n := range typeCounts {
		entries = append(entries, entry{t, n})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].n > entries[j].n })
	if len(entries) > maxTypes {
		entries = entries[:maxTypes]
	}
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("  %s: %d\n", e.t, e.n))
	}
	return sb.String()
}

// statisticalSummary produces a no-LLM fallback for a resource group.
func statisticalSummary(scopeName, typePrefix string, resources []*types.Resource) string {
	names := make([]string, 0, 5)
	for _, r := range resources {
		if len(names) >= 5 {
			break
		}
		names = append(names, r.Name)
	}
	nameList := strings.Join(names, ", ")
	if len(resources) > 5 {
		nameList += fmt.Sprintf(", and %d others", len(resources)-5)
	}
	return fmt.Sprintf(
		"%s scope contains %d %s resource(s): %s.",
		scopeName, len(resources), typePrefix, nameList,
	)
}

// statisticalOverview produces a no-LLM fallback overview for a whole scope.
func statisticalOverview(scopeName string, resources []*types.Resource) string {
	typeCounts := make(map[string]int)
	for _, r := range resources {
		typeCounts[typePrefix(r.Type)]++
	}

	type entry struct {
		t string
		n int
	}
	entries := make([]entry, 0, len(typeCounts))
	for t, n := range typeCounts {
		entries = append(entries, entry{t, n})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].n > entries[j].n })

	parts := make([]string, 0, 5)
	for _, e := range entries {
		if len(parts) >= 5 {
			break
		}
		parts = append(parts, fmt.Sprintf("%d %s", e.n, e.t))
	}
	return fmt.Sprintf(
		"%s scope contains %d total resources: %s.",
		scopeName, len(resources), strings.Join(parts, ", "),
	)
}
