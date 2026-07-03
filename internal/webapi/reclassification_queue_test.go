// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestReclassificationQueue_CoalescesSameCop verifies that saves of the SAME cop
// arriving while it is being reassessed collapse into a single re-run (not one
// per save) — and that nothing is dropped: a save during the run still triggers
// exactly one follow-up run.
func TestReclassificationQueue_CoalescesSameCop(t *testing.T) {
	var calls int32
	started := make(chan struct{})
	release := make(chan struct{})

	q := newReclassificationQueue(func(ctx context.Context, cop, target string) {
		atomic.AddInt32(&calls, 1)
		started <- struct{}{} // announce this run has begun
		<-release             // block until the test releases it
	})
	q.runTimeout = 0

	q.enqueue("Chef/X", "t") // run 1 begins
	recvWithin(t, started, "run 1 to start")

	// Three saves of the same cop while run 1 is in flight — must coalesce.
	q.enqueue("Chef/X", "t")
	q.enqueue("Chef/X", "t")
	q.enqueue("Chef/X", "t")

	release <- struct{}{}                       // finish run 1 → one coalesced re-run
	recvWithin(t, started, "coalesced re-run")  // run 2 begins
	release <- struct{}{}                       // finish run 2 → cop no longer pending

	// No third run should start.
	if gotRunWithin(started, 100*time.Millisecond) {
		t.Fatal("a third run started — saves did not coalesce")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("run count = %d, want 2 (initial + one coalesced re-run)", got)
	}
}

// TestReclassificationQueue_AllDistinctCopsRun verifies that a burst of distinct
// cops each gets reassessed exactly once (no drops), and the target is preserved.
func TestReclassificationQueue_AllDistinctCopsRun(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{} // cop -> target
	var wg sync.WaitGroup
	wg.Add(3)

	q := newReclassificationQueue(func(ctx context.Context, cop, target string) {
		mu.Lock()
		if _, dup := seen[cop]; !dup {
			seen[cop] = target
			wg.Done()
		}
		mu.Unlock()
	})
	q.runTimeout = 0

	q.enqueue("Chef/A", "19.3.15")
	q.enqueue("Chef/B", "19.3.15")
	q.enqueue("Chef/C", "19.3.15")

	waitWithin(t, &wg, "all three cops to be reassessed")

	mu.Lock()
	defer mu.Unlock()
	got := make([]string, 0, len(seen))
	for c := range seen {
		got = append(got, c)
	}
	sort.Strings(got)
	want := []string{"Chef/A", "Chef/B", "Chef/C"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("reassessed cops = %v, want %v", got, want)
	}
	if seen["Chef/A"] != "19.3.15" {
		t.Errorf("target for Chef/A = %q, want 19.3.15", seen["Chef/A"])
	}
}

// TestReclassificationQueue_LatestTargetWins verifies that re-saving a cop before
// it is processed uses the most recent target (map overwrite).
func TestReclassificationQueue_LatestTargetWins(t *testing.T) {
	done := make(chan string, 1)
	q := newReclassificationQueue(func(ctx context.Context, cop, target string) {
		done <- target
	})
	q.runTimeout = 0

	// Enqueue twice before the worker can process; the second target must win.
	q.mu.Lock() // hold the worker off so both enqueues land before processing
	q.pending["Chef/X"] = reclassRequest{target: "old", requestedAt: q.now()}
	q.pending["Chef/X"] = reclassRequest{target: "new", requestedAt: q.now()}
	q.started = true
	q.mu.Unlock()
	go q.worker()
	q.signal <- struct{}{}

	select {
	case target := <-done:
		if target != "new" {
			t.Errorf("target = %q, want new (latest save wins)", target)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reassessment")
	}
}

// --- helpers ---------------------------------------------------------------

func recvWithin(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func gotRunWithin(ch <-chan struct{}, d time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(d):
		return false
	}
}

func waitWithin(t *testing.T, wg *sync.WaitGroup, what string) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}
