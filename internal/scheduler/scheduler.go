// Package scheduler drives automatic cron-based discovery scope runs.
// It polls storage.Backend for all scopes with a non-empty Schedule field
// and registers each one as a robfig/cron/v3 job.
package scheduler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/zerelio/zerelio/internal/storage"
)

// ScopeRunner is the minimal interface the scheduler needs from the orchestrator.
// Using an interface (rather than the concrete *discovery.Orchestrator) avoids
// import cycles between scheduler ↔ discovery.
type ScopeRunner interface {
	RunScope(ctx context.Context, scopeID string) error
}

// Scheduler drives automatic cron-based discovery scope runs.
type Scheduler struct {
	mu     sync.Mutex
	c      *cron.Cron
	store  storage.Backend
	orch   ScopeRunner
	logger *slog.Logger
}

// New creates a Scheduler. Call Start or Reload to begin scheduling.
func New(store storage.Backend, orch ScopeRunner, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		store:  store,
		orch:   orch,
		logger: logger,
	}
}

// Start loads all scopes, registers cron jobs for every scope whose Schedule
// field is non-empty, starts the cron engine, and blocks until ctx is cancelled.
// Intended to be called in a goroutine: go sched.Start(ctx).
func (s *Scheduler) Start(ctx context.Context) {
	s.rebuild(ctx)
	s.logger.Info("discovery scheduler started")

	<-ctx.Done()

	s.mu.Lock()
	if s.c != nil {
		s.c.Stop()
	}
	s.mu.Unlock()

	s.logger.Info("discovery scheduler stopped")
}

// Reload stops the running cron engine and restarts it with a fresh scope list.
// It is safe to call concurrently with Start.
func (s *Scheduler) Reload(ctx context.Context) {
	s.rebuild(ctx)
	s.logger.Info("discovery scheduler reloaded")
}

// IsRunning returns true once Start has been called and the cron has at least one entry.
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.c != nil && len(s.c.Entries()) > 0
}

// rebuild creates a fresh cron instance, registers all scheduled scopes, and
// starts the engine. Any previously running cron is stopped first.
func (s *Scheduler) rebuild(ctx context.Context) {
	s.mu.Lock()
	if s.c != nil {
		s.c.Stop()
	}
	c := cron.New()
	s.c = c
	s.mu.Unlock()

	scopes, err := s.store.ListScopes(ctx, "")
	if err != nil {
		s.logger.Error("scheduler: failed to list scopes", "error", err)
		// Start the (empty) cron so the state remains consistent.
		s.mu.Lock()
		c.Start()
		s.mu.Unlock()
		return
	}

	registered := 0
	for _, scope := range scopes {
		if scope.Schedule == "" {
			continue
		}

		// Capture loop variables for the closure.
		scopeID := scope.ID
		schedule := scope.Schedule

		_, addErr := c.AddFunc(schedule, func() {
			s.logger.Info("scheduler: running scope", "scope_id", scopeID, "schedule", schedule)
			if runErr := s.orch.RunScope(ctx, scopeID); runErr != nil {
				s.logger.Error("scheduler: scope run failed", "scope_id", scopeID, "error", runErr)
			}
		})
		if addErr != nil {
			s.logger.Error("scheduler: invalid cron expression",
				"scope_id", scopeID, "schedule", schedule, "error", addErr)
			continue
		}
		s.logger.Info("scheduler: registered scope", "scope_id", scopeID, "schedule", schedule)
		registered++
	}

	s.logger.Info("scheduler: registration complete",
		"registered", registered, "total_scopes", len(scopes))

	s.mu.Lock()
	c.Start()
	s.mu.Unlock()
}
