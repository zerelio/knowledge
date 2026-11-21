package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zerelio/zerelio/internal/storage"
	"github.com/zerelio/zerelio/pkg/types"
)

// Orchestrator coordinates discovery runs across all registered plugins.
// It is responsible for:
//   - Loading scopes from storage
//   - Dispatching to the appropriate plugin
//   - Persisting results to storage
//   - Tracking run state (started → completed/failed)
type Orchestrator struct {
	store   storage.Backend
	plugins map[string]Plugin
	logger  *slog.Logger
	mu      sync.RWMutex
}

func NewOrchestrator(store storage.Backend, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		store:   store,
		plugins: make(map[string]Plugin),
		logger:  logger,
	}
}

// RegisterPlugin adds a plugin to the orchestrator.
// If a plugin with the same name already exists, it is replaced.
func (o *Orchestrator) RegisterPlugin(p Plugin) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.plugins[p.Name()] = p
	o.logger.Info("registered discovery plugin", "plugin", p.Name())
}

// RunScope executes a discovery run for the given scope ID.
// It is safe to call from multiple goroutines; each call gets its own run record.
func (o *Orchestrator) RunScope(ctx context.Context, scopeID string) (*types.DiscoveryRun, error) {
	scope, err := o.store.GetScope(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("get scope: %w", err)
	}

	o.mu.RLock()
	plugin, ok := o.plugins[scope.PluginName]
	o.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("plugin %q not registered", scope.PluginName)
	}

	run := &types.DiscoveryRun{
		ID:        fmt.Sprintf("run_%d", time.Now().UnixNano()),
		ScopeID:   scope.ID,
		Status:    types.StatusRunning,
		StartedAt: time.Now(),
	}

	if err := o.store.StoreRun(ctx, run); err != nil {
		return nil, fmt.Errorf("store run: %w", err)
	}

	o.logger.Info("discovery run started",
		"run_id", run.ID,
		"scope_id", scope.ID,
		"plugin", scope.PluginName,
	)

	// Snapshot before goroutine launch. executeRun owns the original pointer exclusively.
	snapshot := *run
	go o.executeRun(ctx, run, scope, plugin)
	return &snapshot, nil
}

func (o *Orchestrator) executeRun(ctx context.Context, run *types.DiscoveryRun, scope *types.DiscoveryScope, plugin Plugin) {
	var runErrors []string
	resourceCount := 0
	relCount := 0

	resultCh, err := plugin.Discover(ctx, scope)
	if err != nil {
		o.failRun(ctx, run, fmt.Sprintf("plugin discover: %v", err))
		return
	}

	for result := range resultCh {
		if result.Err != nil {
			runErrors = append(runErrors, result.Err.Error())
			o.logger.Warn("discovery error from plugin",
				"plugin", plugin.Name(),
				"error", result.Err,
			)
			continue
		}

		if len(result.Resources) > 0 {
			if err := o.store.StoreResources(ctx, result.Resources); err != nil {
				runErrors = append(runErrors, fmt.Sprintf("store resources: %v", err))
			} else {
				resourceCount += len(result.Resources)
			}
		}

		if len(result.Relationships) > 0 {
			if err := o.store.StoreRelationships(ctx, result.Relationships); err != nil {
				runErrors = append(runErrors, fmt.Sprintf("store relationships: %v", err))
			} else {
				relCount += len(result.Relationships)
			}
		}
	}

	now := time.Now()
	run.CompletedAt = &now
	run.ResourcesFound = resourceCount
	run.RelationshipsFound = relCount
	run.Errors = runErrors

	if len(runErrors) > 0 && resourceCount == 0 {
		run.Status = types.StatusFailed
	} else {
		run.Status = types.StatusCompleted
	}

	if err := o.store.StoreRun(ctx, run); err != nil {
		o.logger.Error("failed to persist run completion", "run_id", run.ID, "error", err)
	}

	o.logger.Info("discovery run completed",
		"run_id", run.ID,
		"status", run.Status,
		"resources", resourceCount,
		"relationships", relCount,
		"errors", len(runErrors),
	)
}

func (o *Orchestrator) failRun(ctx context.Context, run *types.DiscoveryRun, errMsg string) {
	now := time.Now()
	run.CompletedAt = &now
	run.Status = types.StatusFailed
	run.Errors = []string{errMsg}

	if err := o.store.StoreRun(ctx, run); err != nil {
		o.logger.Error("failed to persist failed run", "run_id", run.ID, "error", err)
	}

	o.logger.Error("discovery run failed", "run_id", run.ID, "error", errMsg)
}

// ListPlugins returns the names of all registered plugins.
func (o *Orchestrator) ListPlugins() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	names := make([]string, 0, len(o.plugins))
	for name := range o.plugins {
		names = append(names, name)
	}
	return names
}
