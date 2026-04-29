// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package kitchenqueue_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/kitchenqueue"
)

// ---------------------------------------------------------------------------
// Mock Store
// ---------------------------------------------------------------------------

type mockStore struct {
	mu    sync.Mutex
	items []*datastore.KitchenQueueItem
}

func newMockStore(items ...*datastore.KitchenQueueItem) *mockStore {
	return &mockStore{items: items}
}

func (s *mockStore) ClaimNextKitchenRun(ctx context.Context) (*datastore.KitchenQueueItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.Status == datastore.QueueStatusQueued {
			item.Status = datastore.QueueStatusRunning
			now := time.Now()
			item.StartedAt = &now
			return item, nil
		}
	}
	return nil, nil
}

func (s *mockStore) CompleteKitchenRun(ctx context.Context, id string, output string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			item.Status = datastore.QueueStatusCompleted
			item.Output = output
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (s *mockStore) FailKitchenRun(ctx context.Context, id string, errMsg string, output string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			item.Status = datastore.QueueStatusFailed
			item.ErrorMessage = errMsg
			item.Output = output
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (s *mockStore) CancelKitchenRun(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			item.Status = datastore.QueueStatusCancelled
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (s *mockStore) MarkInterruptedKitchenRuns(ctx context.Context) (int64, error) {
	return 0, nil
}

func (s *mockStore) GetKitchenQueueStats(ctx context.Context) (*datastore.KitchenQueueStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var stats datastore.KitchenQueueStats
	for _, item := range s.items {
		switch item.Status {
		case datastore.QueueStatusQueued:
			stats.Queued++
		case datastore.QueueStatusRunning:
			stats.Running++
		}
	}
	return &stats, nil
}

func (s *mockStore) getItem(id string) *datastore.KitchenQueueItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			return item
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Mock Executor
// ---------------------------------------------------------------------------

type mockExecutor struct {
	delay   time.Duration
	failIDs map[string]bool
	calls   atomic.Int32
}

func (e *mockExecutor) Execute(ctx context.Context, item *datastore.KitchenQueueItem) (string, error) {
	e.calls.Add(1)
	select {
	case <-time.After(e.delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if e.failIDs != nil && e.failIDs[item.ID] {
		return "failure output", fmt.Errorf("execution failed")
	}
	return "success output", nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestManager_ProcessesQueuedItems(t *testing.T) {
	items := []*datastore.KitchenQueueItem{
		{ID: "item-1", RunType: "git", Status: datastore.QueueStatusQueued, TargetChefVersion: "18"},
		{ID: "item-2", RunType: "git", Status: datastore.QueueStatusQueued, TargetChefVersion: "18"},
	}
	store := newMockStore(items...)
	exec := &mockExecutor{delay: 10 * time.Millisecond}

	mgr := kitchenqueue.New(store, exec,
		kitchenqueue.WithWorkerCount(2),
		kitchenqueue.WithPollInterval(10*time.Millisecond),
	)

	err := mgr.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for items to be processed
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for items to complete")
		default:
		}
		i1 := store.getItem("item-1")
		i2 := store.getItem("item-2")
		if i1.Status == datastore.QueueStatusCompleted && i2.Status == datastore.QueueStatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mgr.Stop(5 * time.Second)

	if exec.calls.Load() != 2 {
		t.Errorf("expected 2 executor calls, got %d", exec.calls.Load())
	}
}

func TestManager_BoundsConcurrency(t *testing.T) {
	// Create 4 items but only 2 workers
	items := make([]*datastore.KitchenQueueItem, 4)
	for i := range items {
		items[i] = &datastore.KitchenQueueItem{
			ID:                fmt.Sprintf("item-%d", i),
			RunType:           "git",
			Status:            datastore.QueueStatusQueued,
			TargetChefVersion: "18",
		}
	}
	store := newMockStore(items...)

	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	exec := &mockExecutor{delay: 50 * time.Millisecond}
	trackingExec := &concurrencyTrackingExecutor{
		inner:          exec,
		current:        &currentConcurrent,
		maxConcurrent:  &maxConcurrent,
	}

	mgr := kitchenqueue.New(store, trackingExec,
		kitchenqueue.WithWorkerCount(2),
		kitchenqueue.WithPollInterval(5*time.Millisecond),
	)

	err := mgr.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for all items
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for items to complete")
		default:
		}
		allDone := true
		for _, item := range items {
			s := store.getItem(item.ID)
			if s.Status != datastore.QueueStatusCompleted {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mgr.Stop(5 * time.Second)

	if maxConcurrent.Load() > 2 {
		t.Errorf("max concurrency exceeded 2: got %d", maxConcurrent.Load())
	}
}

func TestManager_FailedItems(t *testing.T) {
	items := []*datastore.KitchenQueueItem{
		{ID: "fail-1", RunType: "git", Status: datastore.QueueStatusQueued, TargetChefVersion: "18"},
	}
	store := newMockStore(items...)
	exec := &mockExecutor{delay: 5 * time.Millisecond, failIDs: map[string]bool{"fail-1": true}}

	mgr := kitchenqueue.New(store, exec,
		kitchenqueue.WithWorkerCount(1),
		kitchenqueue.WithPollInterval(5*time.Millisecond),
	)

	err := mgr.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for item to fail")
		default:
		}
		if store.getItem("fail-1").Status == datastore.QueueStatusFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mgr.Stop(5 * time.Second)

	item := store.getItem("fail-1")
	if item.ErrorMessage != "execution failed" {
		t.Errorf("expected error 'execution failed', got %q", item.ErrorMessage)
	}
}

func TestManager_CancelRunningItem(t *testing.T) {
	items := []*datastore.KitchenQueueItem{
		{ID: "cancel-1", RunType: "git", Status: datastore.QueueStatusQueued, TargetChefVersion: "18"},
	}
	store := newMockStore(items...)
	exec := &mockExecutor{delay: 5 * time.Second} // long-running

	mgr := kitchenqueue.New(store, exec,
		kitchenqueue.WithWorkerCount(1),
		kitchenqueue.WithPollInterval(5*time.Millisecond),
	)

	err := mgr.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the item to start running
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for item to start")
		default:
		}
		if mgr.IsRunning("cancel-1") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Cancel it
	cancelled := mgr.CancelItem("cancel-1")
	if !cancelled {
		t.Fatal("CancelItem returned false")
	}

	// Wait for it to finish
	deadline = time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for cancel to take effect")
		default:
		}
		if store.getItem("cancel-1").Status == datastore.QueueStatusCancelled {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mgr.Stop(5 * time.Second)
}

func TestManager_StopDrainsWorkers(t *testing.T) {
	items := []*datastore.KitchenQueueItem{
		{ID: "drain-1", RunType: "git", Status: datastore.QueueStatusQueued, TargetChefVersion: "18"},
	}
	store := newMockStore(items...)
	exec := &mockExecutor{delay: 100 * time.Millisecond}

	mgr := kitchenqueue.New(store, exec,
		kitchenqueue.WithWorkerCount(1),
		kitchenqueue.WithPollInterval(5*time.Millisecond),
	)

	err := mgr.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for item to start
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for item to start")
		default:
		}
		if mgr.IsRunning("drain-1") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Stop should wait for the item to finish (100ms delay)
	mgr.Stop(5 * time.Second)

	// Item should have completed, not been cancelled
	if store.getItem("drain-1").Status != datastore.QueueStatusCompleted {
		t.Errorf("expected completed, got %s", store.getItem("drain-1").Status)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type concurrencyTrackingExecutor struct {
	inner         kitchenqueue.Executor
	current       *atomic.Int32
	maxConcurrent *atomic.Int32
}

func (e *concurrencyTrackingExecutor) Execute(ctx context.Context, item *datastore.KitchenQueueItem) (string, error) {
	cur := e.current.Add(1)
	for {
		old := e.maxConcurrent.Load()
		if cur <= old || e.maxConcurrent.CompareAndSwap(old, cur) {
			break
		}
	}
	defer e.current.Add(-1)
	return e.inner.Execute(ctx, item)
}
