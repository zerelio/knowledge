package change_test

import (
	"context"
	"testing"
	"time"

	"github.com/zerelio/knowledge/internal/change"
	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

func newResource(id string, props map[string]string) *types.Resource {
	return &types.Resource{
		ID:           id,
		Name:         id,
		Type:         "test",
		Provider:     "test",
		TenantID:     "tenant1",
		Properties:   props,
		DiscoveredAt: time.Now(),
	}
}

func TestDetectAndRecord_Added(t *testing.T) {
	store := storage.NewMemoryBackend()
	ctx := context.Background()

	det := change.New(store, nil)

	res := newResource("res-001", map[string]string{"env": "prod"})
	changes, err := det.DetectAndRecord(ctx, "scope-1", "run-1", "tenant1", []*types.Resource{res})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	c := changes[0]
	if c.ChangeType != types.ChangeAdded {
		t.Errorf("expected ChangeAdded, got %s", c.ChangeType)
	}
	if c.ResourceID != "res-001" {
		t.Errorf("unexpected resource_id: %s", c.ResourceID)
	}
	if c.AfterProps["env"] != "prod" {
		t.Errorf("expected after_props.env=prod, got %q", c.AfterProps["env"])
	}
	if c.BeforeProps != nil {
		t.Errorf("expected no before_props for added resource")
	}

	// Verify it was persisted.
	stored, err := store.ListChanges(ctx, "scope-1", "tenant1", 10)
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	if len(stored) != 1 {
		t.Errorf("expected 1 stored change, got %d", len(stored))
	}
}

func TestDetectAndRecord_Modified(t *testing.T) {
	store := storage.NewMemoryBackend()
	ctx := context.Background()

	// Store the "old" version of the resource.
	old := newResource("res-002", map[string]string{"env": "staging", "version": "1"})
	if err := store.StoreResources(ctx, []*types.Resource{old}); err != nil {
		t.Fatalf("store old resource: %v", err)
	}

	det := change.New(store, nil)

	// New version has different Properties.
	newRes := newResource("res-002", map[string]string{"env": "prod", "version": "2"})
	changes, err := det.DetectAndRecord(ctx, "scope-1", "run-2", "tenant1", []*types.Resource{newRes})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	c := changes[0]
	if c.ChangeType != types.ChangeModified {
		t.Errorf("expected ChangeModified, got %s", c.ChangeType)
	}
	if c.BeforeProps["env"] != "staging" {
		t.Errorf("expected before_props.env=staging, got %q", c.BeforeProps["env"])
	}
	if c.AfterProps["env"] != "prod" {
		t.Errorf("expected after_props.env=prod, got %q", c.AfterProps["env"])
	}

	// No change when properties are identical.
	changes2, err := det.DetectAndRecord(ctx, "scope-1", "run-3", "tenant1", []*types.Resource{old})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes2) != 0 {
		t.Errorf("expected 0 changes for identical resource, got %d", len(changes2))
	}
}
