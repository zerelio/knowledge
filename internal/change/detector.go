// Package change provides change-detection logic: diffing discovered resources
// against the previously stored state and recording the deltas.
package change

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/zerelio/zerelio/internal/storage"
	"github.com/zerelio/zerelio/pkg/types"
)

// Detector compares newly discovered resources against the stored state and
// records any added or modified resources as ResourceChange entries.
type Detector struct {
	store  storage.Backend
	logger *slog.Logger
}

// New creates a Detector backed by the given storage.Backend.
func New(store storage.Backend, logger *slog.Logger) *Detector {
	return &Detector{store: store, logger: logger}
}

// DetectAndRecord must be called BEFORE StoreResources so the DB still holds
// the previous state.  It compares each resource in newResources against the
// stored version:
//   - not found  → ChangeAdded
//   - found, Properties differ → ChangeModified
//
// Every detected change is persisted via store.RecordChange, and the full
// slice is returned to the caller.
func (d *Detector) DetectAndRecord(
	ctx context.Context,
	scopeID, runID, tenantID string,
	newResources []*types.Resource,
) ([]*types.ResourceChange, error) {
	var changes []*types.ResourceChange

	for _, res := range newResources {
		old, err := d.store.GetResourceByID(ctx, res.ID)
		if err != nil {
			// Resource not found in DB — treat as new.
			c := &types.ResourceChange{
				ID:         fmt.Sprintf("chg_%d_%s", time.Now().UnixNano(), res.ID),
				ScopeID:    scopeID,
				RunID:      runID,
				ResourceID: res.ID,
				TenantID:   tenantID,
				ChangeType: types.ChangeAdded,
				AfterProps: copyMap(res.Properties),
				DetectedAt: time.Now(),
			}
			changes = append(changes, c)
			continue
		}

		if !mapsEqual(old.Properties, res.Properties) {
			c := &types.ResourceChange{
				ID:          fmt.Sprintf("chg_%d_%s", time.Now().UnixNano(), res.ID),
				ScopeID:     scopeID,
				RunID:       runID,
				ResourceID:  res.ID,
				TenantID:    tenantID,
				ChangeType:  types.ChangeModified,
				BeforeProps: copyMap(old.Properties),
				AfterProps:  copyMap(res.Properties),
				DetectedAt:  time.Now(),
			}
			changes = append(changes, c)
		}
	}

	for _, c := range changes {
		if err := d.store.RecordChange(ctx, c); err != nil {
			if d.logger != nil {
				d.logger.Error("failed to record change", "resource_id", c.ResourceID, "error", err)
			}
		}
	}

	return changes, nil
}

// copyMap returns a shallow copy of m (safe for nil input).
func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}

// mapsEqual reports whether two string→string maps have identical contents.
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
