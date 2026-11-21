package knowledge_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/zerelio/knowledge/internal/knowledge"
	"github.com/zerelio/knowledge/internal/llm"
	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestSummarizeScope_StatisticalFallback(t *testing.T) {
	store := storage.NewMemoryBackend()
	ctx := context.Background()
	_ = store.Connect(ctx)

	// Store some resources.
	_ = store.StoreResources(ctx, []*types.Resource{
		{ID: "r1", TenantID: "t1", Type: "aws:ec2:instance", Name: "web-prod", Properties: map[string]string{}},
		{ID: "r2", TenantID: "t1", Type: "aws:ec2:instance", Name: "db-prod", Properties: map[string]string{}},
		{ID: "r3", TenantID: "t1", Type: "aws:s3:bucket", Name: "data-bucket", Properties: map[string]string{}},
	})

	s := knowledge.New(store, &llm.NoopProvider{}, discardLogger())
	// Should not panic and should store summaries.
	s.SummarizeScope(ctx, "scope-1", "t1", "Production AWS")

	// Verify summaries were stored.
	summaries, err := store.ListKnowledgeSummaries(ctx, "t1", "scope-1")
	if err != nil {
		t.Fatalf("ListKnowledgeSummaries: %v", err)
	}
	if len(summaries) == 0 {
		t.Error("expected at least one summary to be stored")
	}

	// There should be an "all" overview summary.
	hasAll := false
	for _, s := range summaries {
		if s.TypePrefix == "all" {
			hasAll = true
		}
		if s.Summary == "" {
			t.Errorf("summary %s has empty content", s.ID)
		}
	}
	if !hasAll {
		t.Error("expected an 'all' overview summary")
	}
}

func TestSummarizeScope_EmptyScope(t *testing.T) {
	store := storage.NewMemoryBackend()
	ctx := context.Background()
	_ = store.Connect(ctx)

	s := knowledge.New(store, &llm.NoopProvider{}, discardLogger())
	// Should not panic on empty scope.
	s.SummarizeScope(ctx, "empty-scope", "t2", "Empty")

	summaries, _ := store.ListKnowledgeSummaries(ctx, "t2", "empty-scope")
	if len(summaries) != 0 {
		t.Errorf("expected no summaries for empty scope, got %d", len(summaries))
	}
}
