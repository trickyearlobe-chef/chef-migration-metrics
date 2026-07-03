// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"sync"
	"time"
)

// reclassificationQueue coalesces cop-reclassification reassessments so that
// saving a classification returns instantly instead of blocking on the scoped
// recompute closure (verdicts + git status + complexity + readiness), which is
// O(repos) and took tens of seconds at ~2000 repos.
//
// Model (per cop, timestamp-deduplicated):
//   - Each save stamps pending[cop] = now() and pokes the worker.
//   - A single worker processes one cop at a time: it removes the cop from the
//     pending set, records the run start, runs the reassessment, then re-runs the
//     cop only if it was saved again during/after that run (its enqueue re-adds it
//     to the set). Multiple saves of the same cop before it is processed collapse
//     into one run.
//
// Guarantees: no save is dropped (every save enqueues), reassessments are
// serialized (no concurrent DB contention), and the system is eventually
// consistent — after a burst the set drains and the derived data converges.
type reclassificationQueue struct {
	// run performs the reassessment for one cop × target. Injected for testing.
	run func(ctx context.Context, copName, targetVersion string)
	// now returns the current time. Injected for testing.
	now func() time.Time
	// runTimeout bounds a single reassessment. 0 means no timeout.
	runTimeout time.Duration

	mu      sync.Mutex
	pending map[string]reclassRequest
	started bool
	signal  chan struct{}
}

type reclassRequest struct {
	target      string
	requestedAt time.Time
}

// newReclassificationQueue builds a queue. run must be non-nil.
func newReclassificationQueue(run func(ctx context.Context, copName, targetVersion string)) *reclassificationQueue {
	return &reclassificationQueue{
		run:        run,
		now:        time.Now,
		runTimeout: 30 * time.Minute,
		pending:    make(map[string]reclassRequest),
		signal:     make(chan struct{}, 1),
	}
}

// enqueue records that copName (for the given target) needs reassessment and
// wakes the worker, starting it on first use. Returns immediately.
func (q *reclassificationQueue) enqueue(copName, targetVersion string) {
	q.mu.Lock()
	q.pending[copName] = reclassRequest{target: targetVersion, requestedAt: q.now()}
	startWorker := !q.started
	q.started = true
	q.mu.Unlock()

	if startWorker {
		go q.worker()
	}
	// Non-blocking poke: a full buffer already means "work pending".
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

// worker drains the pending set whenever poked. It runs for the process lifetime.
func (q *reclassificationQueue) worker() {
	for range q.signal {
		q.drain()
	}
}

// drain processes pending cops until the set is empty. Each cop is removed before
// its run; a save during the run re-adds it (via enqueue), so it is picked up on a
// later iteration — this is the timestamp-dedup re-run.
func (q *reclassificationQueue) drain() {
	for {
		q.mu.Lock()
		copName, req, ok := takeOne(q.pending)
		if !ok {
			q.mu.Unlock()
			return
		}
		delete(q.pending, copName)
		q.mu.Unlock()

		q.runOne(copName, req.target)
	}
}

// runOne executes a single reassessment with the configured timeout.
func (q *reclassificationQueue) runOne(copName, targetVersion string) {
	ctx := context.Background()
	if q.runTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, q.runTimeout)
		defer cancel()
	}
	q.run(ctx, copName, targetVersion)
}

// takeOne returns an arbitrary entry from the map (the map is small — at most the
// number of distinct cops reclassified in a burst).
func takeOne(m map[string]reclassRequest) (string, reclassRequest, bool) {
	for k, v := range m {
		return k, v, true
	}
	return "", reclassRequest{}, false
}
