package discovery

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockPlugin is a test double that emits a fixed set of resources.
type mockPlugin struct {
	name      string
	resources []*types.Resource
	rels      []*types.Relationship
	err       error
}

func (m *mockPlugin) Name() string { return m.name }

func (m *mockPlugin) Discover(_ context.Context, _ *types.DiscoveryScope) (<-chan DiscoveryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan DiscoveryResult, 1)
	ch <- DiscoveryResult{Resources: m.resources, Relationships: m.rels}
	close(ch)
	return ch, nil
}

func TestOrchestratorRunScope(t *testing.T) {
	logger := noopLogger()
	store := storage.NewMemoryBackend()
	ctx := context.Background()

	scope := &types.DiscoveryScope{
		ID:         "scope_01",
		Name:       "test-aws",
		PluginName: "mock",
		TenantID:   "tenant_01",
	}
	if err := store.CreateScope(ctx, scope); err != nil {
		t.Fatal(err)
	}

	plugin := &mockPlugin{
		name: "mock",
		resources: []*types.Resource{
			{
				ID:           "res_001",
				Type:         "aws:ec2:instance",
				Name:         "web-server",
				Provider:     "aws",
				Region:       "us-east-1",
				TenantID:     "tenant_01",
				DiscoveredAt: time.Now(),
				ValidFrom:    time.Now(),
			},
		},
	}

	orch := NewOrchestrator(store, logger)
	orch.RegisterPlugin(plugin)

	run, err := orch.RunScope(ctx, scope.ID)
	if err != nil {
		t.Fatalf("RunScope: %v", err)
	}
	if run.Status != types.StatusRunning {
		t.Errorf("initial run status = %q, want running", run.Status)
	}

	// Wait for the async run to complete.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs, _ := store.ListRuns(ctx, scope.ID, 10)
		for _, r := range runs {
			if r.ID == run.ID && r.Status == types.StatusCompleted {
				if r.ResourcesFound != 1 {
					t.Errorf("resources_found = %d, want 1", r.ResourcesFound)
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("run did not complete within timeout")
}

func TestOrchestratorUnknownPlugin(t *testing.T) {
	logger := noopLogger()
	store := storage.NewMemoryBackend()
	ctx := context.Background()

	scope := &types.DiscoveryScope{
		ID:         "scope_02",
		Name:       "no-plugin",
		PluginName: "nonexistent",
	}
	if err := store.CreateScope(ctx, scope); err != nil {
		t.Fatal(err)
	}

	orch := NewOrchestrator(store, logger)
	_, err := orch.RunScope(ctx, scope.ID)
	if err == nil {
		t.Error("expected error for unknown plugin, got nil")
	}
}
