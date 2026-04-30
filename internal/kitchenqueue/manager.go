// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package kitchenqueue provides a DB-backed queue and worker pool for all
// test kitchen execution. It replaces fire-and-forget goroutines with bounded
// concurrency, deduplication, and priority scheduling.
package kitchenqueue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Store abstracts the datastore methods needed by the queue manager.
type Store interface {
	ClaimNextKitchenRun(ctx context.Context) (*datastore.KitchenQueueItem, error)
	CompleteKitchenRun(ctx context.Context, id string, output string) error
	FailKitchenRun(ctx context.Context, id string, errMsg string, output string) error
	CancelKitchenRun(ctx context.Context, id string) error
	MarkInterruptedKitchenRuns(ctx context.Context) (int64, error)
	GetKitchenQueueStats(ctx context.Context) (*datastore.KitchenQueueStats, error)
}

// Executor runs a single kitchen queue item. Implementations handle the
// specifics of git vs node kitchen runs. It returns output and any error.
type Executor interface {
	Execute(ctx context.Context, item *datastore.KitchenQueueItem) (output string, err error)
}

// OutputListener receives live output lines for a running queue item.
type OutputListener func(itemID string, line string)

// EventListener is called when a queue item changes state.
type EventListener func(item *datastore.KitchenQueueItem)

// LogFunc receives structured log messages.
type LogFunc func(level, msg string, args ...any)

// Manager coordinates the worker pool and dequeue loop.
type Manager struct {
	store    Store
	executor Executor
	logFn    LogFunc
	eventFn  EventListener
	outputFn OutputListener

	workerCount int
	pollInterval time.Duration

	mu       sync.Mutex
	running  map[string]context.CancelFunc // item ID → cancel func
	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// Option configures a Manager.
type Option func(*Manager)

// WithWorkerCount sets the number of concurrent workers.
func WithWorkerCount(n int) Option {
	return func(m *Manager) {
		if n > 0 {
			m.workerCount = n
		}
	}
}

// WithPollInterval sets how often workers poll for new work.
func WithPollInterval(d time.Duration) Option {
	return func(m *Manager) {
		if d > 0 {
			m.pollInterval = d
		}
	}
}

// WithLogFunc sets the logging callback.
func WithLogFunc(fn LogFunc) Option {
	return func(m *Manager) { m.logFn = fn }
}

// WithEventListener sets the callback for queue state changes.
func WithEventListener(fn EventListener) Option {
	return func(m *Manager) { m.eventFn = fn }
}

// WithOutputListener sets the callback for live output lines.
func WithOutputListener(fn OutputListener) Option {
	return func(m *Manager) { m.outputFn = fn }
}

// New creates a Manager with the given options.
func New(store Store, executor Executor, opts ...Option) *Manager {
	m := &Manager{
		store:        store,
		executor:     executor,
		workerCount:  4,
		pollInterval: 2 * time.Second,
		running:      make(map[string]context.CancelFunc),
		stopCh:       make(chan struct{}),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start launches the worker pool and begins processing queue items.
// It also performs startup recovery (marks in-flight items as interrupted).
func (m *Manager) Start(ctx context.Context) error {
	n, err := m.store.MarkInterruptedKitchenRuns(ctx)
	if err != nil {
		return fmt.Errorf("kitchenqueue: startup recovery: %w", err)
	}
	if n > 0 {
		m.log("WARN", "marked %d in-flight items as interrupted on startup", n)
	}

	for i := 0; i < m.workerCount; i++ {
		m.wg.Add(1)
		go m.worker(i)
	}

	m.log("INFO", "started %d kitchen queue workers", m.workerCount)
	return nil
}

// Stop gracefully shuts down the worker pool. Workers finish their current
// item but don't pick up new ones. If timeout is reached, running items are
// cancelled via their context.
func (m *Manager) Stop(timeout time.Duration) {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.log("INFO", "all kitchen queue workers stopped")
	case <-time.After(timeout):
		m.log("WARN", "timeout waiting for workers, cancelling running items")
		m.mu.Lock()
		for id, cancel := range m.running {
			m.log("WARN", "cancelling running item %s", id)
			cancel()
		}
		m.mu.Unlock()
		<-done
	}
}

// CancelItem cancels a specific running item by its queue ID.
// If the item is queued (not running), use the store's CancelKitchenRun directly.
func (m *Manager) CancelItem(id string) bool {
	m.mu.Lock()
	cancel, ok := m.running[id]
	m.mu.Unlock()
	if ok {
		cancel()
		return true
	}
	return false
}

// IsRunning returns true if the given item ID is currently being executed.
func (m *Manager) IsRunning(id string) bool {
	m.mu.Lock()
	_, ok := m.running[id]
	m.mu.Unlock()
	return ok
}

// RunningCount returns the number of items currently being executed.
func (m *Manager) RunningCount() int {
	m.mu.Lock()
	n := len(m.running)
	m.mu.Unlock()
	return n
}

// worker is the main loop for a single worker goroutine.
func (m *Manager) worker(id int) {
	defer m.wg.Done()

	for {
		select {
		case <-m.stopCh:
			return
		default:
		}

		item, err := m.store.ClaimNextKitchenRun(context.Background())
		if err != nil {
			m.log("ERROR", "worker %d: claim error: %v", id, err)
			m.sleep()
			continue
		}

		if item == nil {
			m.sleep()
			continue
		}

		m.log("INFO", "worker %d: executing %s (%s/%s/%s)",
			id, item.ID, item.GitRepoName, item.SuiteName, item.PlatformName)

		m.executeItem(item)
	}
}

// executeItem runs a single queue item with cancellation support.
func (m *Manager) executeItem(item *datastore.KitchenQueueItem) {
	ctx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	m.running[item.ID] = cancel
	m.mu.Unlock()

	if m.eventFn != nil {
		m.eventFn(item)
	}

	output, execErr := m.executor.Execute(ctx, item)

	// Check if the context was cancelled BEFORE we call cancel() ourselves
	wasCancelled := ctx.Err() != nil

	cancel() // release context resources

	m.mu.Lock()
	delete(m.running, item.ID)
	m.mu.Unlock()

	if execErr != nil {
		if wasCancelled {
			_ = m.store.CancelKitchenRun(context.Background(), item.ID)
			item.Status = datastore.QueueStatusCancelled
		} else {
			_ = m.store.FailKitchenRun(context.Background(), item.ID, execErr.Error(), output)
			item.Status = datastore.QueueStatusFailed
			item.ErrorMessage = execErr.Error()
		}
	} else {
		_ = m.store.CompleteKitchenRun(context.Background(), item.ID, output)
		item.Status = datastore.QueueStatusCompleted
	}

	item.Output = output
	if m.eventFn != nil {
		m.eventFn(item)
	}

	m.log("INFO", "item %s finished with status %s", item.ID, item.Status)
}

// sleep pauses the worker between poll attempts, but wakes on stop.
func (m *Manager) sleep() {
	select {
	case <-m.stopCh:
	case <-time.After(m.pollInterval):
	}
}

func (m *Manager) log(level, format string, args ...any) {
	if m.logFn != nil {
		m.logFn(level, fmt.Sprintf(format, args...))
	}
}
