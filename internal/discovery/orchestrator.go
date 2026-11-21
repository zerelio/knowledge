package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	sdkplugin "github.com/zerelio/plugin-sdk/plugin"
	"github.com/zerelio/zerelio/internal/graph"
	"github.com/zerelio/zerelio/internal/storage"
	"github.com/zerelio/zerelio/pkg/types"
)

// Orchestrator coordinates discovery runs. It delegates subprocess plugin loading
// to the SDK's PluginManager and falls back to in-process plugins registered
// directly (useful for tests and embedded mode).
type Orchestrator struct {
	store            storage.Backend
	manager          *sdkplugin.PluginManager
	inProcessPlugins map[string]Plugin
	graph            *graph.Graph // optional; nil when running without PostgreSQL
	logger           *slog.Logger
	mu               sync.RWMutex
}

func NewOrchestrator(store storage.Backend, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		store:            store,
		manager:          sdkplugin.NewPluginManager(nil),
		inProcessPlugins: make(map[string]Plugin),
		logger:           logger,
	}
}

// SetGraph attaches an optional graph module for post-run sync.
func (o *Orchestrator) SetGraph(g *graph.Graph) {
	o.graph = g
}

// RegisterPlugin registers an in-process plugin (for testing or embedded use).
func (o *Orchestrator) RegisterPlugin(p Plugin) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.inProcessPlugins[p.Name()] = p
	o.logger.Info("registered in-process plugin", "plugin", p.Name())
}

// LoadSubprocessPlugin loads a plugin from a binary path via hashicorp/go-plugin.
func (o *Orchestrator) LoadSubprocessPlugin(name, path string) error {
	return o.manager.LoadPlugin(name, path)
}

// RunScope executes a discovery run for the given scope ID.
// Returns an error synchronously if the scope or plugin cannot be found.
func (o *Orchestrator) RunScope(ctx context.Context, scopeID string) (*types.DiscoveryRun, error) {
	scope, err := o.store.GetScope(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("get scope: %w", err)
	}

	// Validate plugin availability before creating the run record.
	_, sdkErr := o.manager.GetPlugin(scope.PluginName)
	if sdkErr != nil {
		o.mu.RLock()
		_, inProcOk := o.inProcessPlugins[scope.PluginName]
		o.mu.RUnlock()
		if !inProcOk {
			return nil, fmt.Errorf("plugin %q not found", scope.PluginName)
		}
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

	o.logger.Info("discovery run started", "run_id", run.ID, "scope_id", scope.ID, "plugin", scope.PluginName)

	snapshot := *run
	go o.executeRun(ctx, run, scope)
	return &snapshot, nil
}

func (o *Orchestrator) executeRun(ctx context.Context, run *types.DiscoveryRun, scope *types.DiscoveryScope) {
	var runErrors []string
	resourceCount, relCount := 0, 0

	// Try subprocess plugin first, fall back to in-process.
	sdkPlug, sdkErr := o.manager.GetPlugin(scope.PluginName)
	if sdkErr != nil {
		// In-process fallback.
		o.mu.RLock()
		inProc, ok := o.inProcessPlugins[scope.PluginName]
		o.mu.RUnlock()
		if !ok {
			o.failRun(ctx, run, fmt.Sprintf("plugin %q not found (subprocess: %v)", scope.PluginName, sdkErr))
			return
		}
		// Use the simple in-process plugin path.
		resultCh, err := inProc.Discover(ctx, scope)
		if err != nil {
			o.failRun(ctx, run, fmt.Sprintf("plugin discover: %v", err))
			return
		}
		for result := range resultCh {
			if result.Err != nil {
				runErrors = append(runErrors, result.Err.Error())
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
	} else {
		// Real subprocess plugin via SDK.
		req := &sdkplugin.DiscoverRequest{
			ScopeID: scope.ID,
			Config:  scope.Config,
		}
		if scope.CredentialSource != nil {
			req.CredentialSource = sdkplugin.CredentialSource{
				Type:   scope.CredentialSource.Type,
				Path:   scope.CredentialSource.Path,
				Params: scope.CredentialSource.Params,
			}
		}

		eventCh, err := sdkPlug.Discover(ctx, req)
		if err != nil {
			o.failRun(ctx, run, fmt.Sprintf("plugin discover: %v", err))
			return
		}

		for event := range eventCh {
			if event.Error != nil {
				runErrors = append(runErrors, event.Error.Message)
				continue
			}
			if event.Resource != nil {
				r := sdkResourceToTypes(event.Resource)
				if err := o.store.StoreResources(ctx, []*types.Resource{r}); err != nil {
					runErrors = append(runErrors, fmt.Sprintf("store resource: %v", err))
				} else {
					resourceCount++
				}
			}
			if event.Relationship != nil {
				rel := sdkRelToTypes(event.Relationship)
				if err := o.store.StoreRelationships(ctx, []*types.Relationship{rel}); err != nil {
					runErrors = append(runErrors, fmt.Sprintf("store relationship: %v", err))
				} else {
					relCount++
				}
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
		o.logger.Error("failed to persist run", "run_id", run.ID, "error", err)
	}

	// Update scope: record last run time and resource count.
	scope.LastRun = &now
	scope.ResourceCount = resourceCount
	if err := o.store.UpdateScope(ctx, scope); err != nil {
		o.logger.Error("failed to update scope after run", "scope_id", scope.ID, "error", err)
	}

	// Sync to AGE graph if available.
	if o.graph != nil {
		resources, _, _ := o.store.GetResources(ctx, storage.ResourceFilter{Limit: 10000})
		rels, _ := o.store.GetRelationships(ctx, storage.RelationshipFilter{Limit: 100000})
		go o.graph.SyncBatch(context.Background(), resources, rels)
	}

	o.logger.Info("discovery run completed",
		"run_id", run.ID, "status", run.Status,
		"resources", resourceCount, "relationships", relCount, "errors", len(runErrors),
	)
}

func (o *Orchestrator) failRun(ctx context.Context, run *types.DiscoveryRun, errMsg string) {
	now := time.Now()
	run.CompletedAt = &now
	run.Status = types.StatusFailed
	run.Errors = []string{errMsg}
	o.store.StoreRun(ctx, run)
	o.logger.Error("discovery run failed", "run_id", run.ID, "error", errMsg)
}

// ListPlugins returns the names of all available plugins (subprocess + in-process).
func (o *Orchestrator) ListPlugins() []string {
	names := o.manager.ListPlugins()
	o.mu.RLock()
	defer o.mu.RUnlock()
	for name := range o.inProcessPlugins {
		names = append(names, name)
	}
	return names
}

// ─── Type adapters ───────────────────────────────────────────────────────────

func sdkResourceToTypes(e *sdkplugin.ResourceEvent) *types.Resource {
	r := &types.Resource{
		ID:           e.ID,
		Type:         e.Type,
		Name:         e.Name,
		Provider:     e.Provider,
		Region:       e.Region,
		AccountID:    e.AccountID,
		Tags:         e.Tags,
		DiscoveredAt: time.Unix(e.DiscoveredAt, 0),
	}
	r.Properties = make(map[string]string, len(e.Properties))
	for k, v := range e.Properties {
		r.Properties[k] = v
	}
	return r
}

func sdkRelToTypes(e *sdkplugin.RelationshipEvent) *types.Relationship {
	return &types.Relationship{
		SourceID:   e.SourceID,
		TargetID:   e.TargetID,
		Type:       e.Type,
		Properties: make(map[string]string),
	}
}
