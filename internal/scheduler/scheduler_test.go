package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

// mockRunner is a counting ScopeRunner for tests.
type mockRunner struct {
	count atomic.Int64
	onRun func() // optional callback fired on each RunScope call
}

func (m *mockRunner) RunScope(_ context.Context, _ string) error {
	m.count.Add(1)
	if m.onRun != nil {
		m.onRun()
	}
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestNoSchedule_NotRegistered verifies that a scope without a schedule
// results in zero cron entries.
func TestNoSchedule_NotRegistered(t *testing.T) {
	store := storage.NewMemoryBackend()
	ctx := context.Background()
	_ = store.Connect(ctx)
	_ = store.CreateScope(ctx, &types.DiscoveryScope{
		ID:         "s-noschedule",
		Name:       "no-schedule",
		PluginName: "noop",
		// Schedule intentionally empty
	})

	mock := &mockRunner{}
	sched := New(store, mock, discardLogger())
	sched.rebuild(ctx)

	sched.mu.Lock()
	entries := sched.c.Entries()
	sched.c.Stop()
	sched.mu.Unlock()

	if len(entries) != 0 {
		t.Fatalf("expected 0 cron entries for scope with no schedule, got %d", len(entries))
	}
	if got := mock.count.Load(); got != 0 {
		t.Fatalf("expected 0 RunScope calls, got %d", got)
	}
}

// TestValidSchedule_Registered verifies that a scope with a valid cron
// expression is registered and the job fires at least once.
func TestValidSchedule_Registered(t *testing.T) {
	store := storage.NewMemoryBackend()
	ctx := context.Background()
	_ = store.Connect(ctx)
	_ = store.CreateScope(ctx, &types.DiscoveryScope{
		ID:         "s-with-schedule",
		Name:       "with-schedule",
		PluginName: "noop",
		Schedule:   "@every 1s", // 1s is the minimum reliable robfig/cron resolution
	})

	fired := make(chan struct{}, 1)
	mock := &mockRunner{}
	mock.onRun = func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	}

	runCtx, runCancel := context.WithTimeout(ctx, 5*time.Second)
	defer runCancel()

	sched := New(store, mock, discardLogger())
	go sched.Start(runCtx)

	select {
	case <-fired:
		// At least one job fired — success
	case <-runCtx.Done():
		t.Fatal("timed out waiting for scheduled scope to run")
	}
}

// TestReload_NoPanic verifies that calling Reload on a running scheduler
// does not panic.
func TestReload_NoPanic(t *testing.T) {
	store := storage.NewMemoryBackend()
	ctx := context.Background()
	_ = store.Connect(ctx)

	mock := &mockRunner{}
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	sched := New(store, mock, discardLogger())
	go sched.Start(runCtx)
	time.Sleep(20 * time.Millisecond)

	// Reload must not panic.
	sched.Reload(ctx)
	time.Sleep(20 * time.Millisecond)
	runCancel()
}
