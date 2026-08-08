// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package ownershipschedule runs saved ownership imports on a cron schedule.
//
// The schedules live in the database, one per saved import, and are re-read on
// every tick — so adding, changing or switching one off in the UI takes effect
// without a restart. That is why this polls rather than holding a timer per
// import: there is no reliable moment to rebuild a set of timers from, and a
// stale timer is a schedule that silently stopped matching what the screen says.
package ownershipschedule

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Run outcomes, as stored against the saved import.
const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// tickInterval is how often the scheduler asks what is due. Cron's finest
// grain is the minute, so checking more often would only add load.
const tickInterval = time.Minute

// runTimeout bounds a single unattended import.
//
// A backstop for a source that has stopped answering rather than a target: an
// import against a system of record can legitimately take minutes. Without it a
// hung connection would block every later schedule indefinitely, and the symptom
// would be "nothing has run since Tuesday" with nothing to say why.
const runTimeout = 30 * time.Minute

// Store is the slice of the datastore this needs.
type Store interface {
	ListScheduledImports(ctx context.Context) ([]datastore.ImportMapping, error)
	RecordImportRun(ctx context.Context, id int64, status, detail string) error
}

// RunFunc executes one saved import and returns a one-line summary of what it
// did, for the run history.
//
// A function rather than an interface because the only implementation lives in
// the web layer, beside the classify-and-commit code an interactive import
// uses. Importing that package here would be a cycle, and duplicating the
// import logic would be two behaviours to keep in step.
type RunFunc func(ctx context.Context, m datastore.ImportMapping) (string, error)

// Scheduler polls for due imports and runs them.
type Scheduler struct {
	store  Store
	run    RunFunc
	logger func(level, msg string)

	stopCh chan struct{}
	doneCh chan struct{}

	// mu serialises runs. Imports are heavy and a tick must never start a
	// second copy of one that is still going.
	mu sync.Mutex
}

// New creates a scheduler. A nil logger discards.
func New(store Store, run RunFunc, logger func(level, msg string)) *Scheduler {
	if logger == nil {
		logger = func(string, string) {}
	}
	return &Scheduler{
		store:  store,
		run:    run,
		logger: logger,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins polling. It runs until Stop is called or the context is done.
func (s *Scheduler) Start(ctx context.Context) {
	go s.loop(ctx)
}

// Stop signals the scheduler to stop and waits for the loop to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
}

func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.doneCh)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	// Nothing is due before the first tick: an import whose cron happens to
	// match the moment the service started should not fire on every restart.
	since := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.RunDue(ctx, since, now)
			since = now
		}
	}
}

// RunDue runs every scheduled import whose expression came round in the window
// (since, now].
//
// Exported so the decision of what is due can be tested without waiting on a
// clock. Runs sequentially: two imports firing at 02:00 are two queries against
// systems of record, and running them one after another is both kinder to those
// systems and easier to read in a log.
func (s *Scheduler) RunDue(ctx context.Context, since, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	imports, err := s.store.ListScheduledImports(ctx)
	if err != nil {
		s.logger("error", "ownership schedule: could not list scheduled imports: "+err.Error())
		return
	}

	for _, m := range imports {
		schedule, err := collector.ParseSchedule(m.Schedule)
		if err != nil {
			// Recorded rather than skipped: an import that can never fire has
			// to say so, or the screen shows a schedule and nothing happens.
			s.logger("error", fmt.Sprintf("ownership schedule: import %q has an unreadable expression %q: %v",
				m.Name, m.Schedule, err))
			s.record(ctx, m, StatusFailed, "The schedule "+m.Schedule+" is not a valid cron expression.")
			continue
		}

		// One run per window, however many occurrences it spans. After an
		// outage an hourly import owes one import, not one per missed hour.
		next := schedule.Next(since)
		if next.IsZero() || next.After(now) {
			continue
		}

		s.runOne(ctx, m)
	}
}

func (s *Scheduler) runOne(ctx context.Context, m datastore.ImportMapping) {
	s.logger("info", fmt.Sprintf("ownership schedule: running import %q", m.Name))

	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	detail, err := s.run(runCtx, m)
	if err != nil {
		s.logger("error", fmt.Sprintf("ownership schedule: import %q failed: %v", m.Name, err))
		s.record(ctx, m, StatusFailed, err.Error())
		return
	}

	s.logger("info", fmt.Sprintf("ownership schedule: import %q finished: %s", m.Name, detail))
	s.record(ctx, m, StatusSucceeded, detail)
}

// record stores the outcome. A failure to write the history is logged and
// dropped: the import has already happened, and reporting the bookkeeping
// failure as an import failure would be a lie in the more misleading direction.
func (s *Scheduler) record(ctx context.Context, m datastore.ImportMapping, status, detail string) {
	if err := s.store.RecordImportRun(ctx, m.ID, status, detail); err != nil {
		s.logger("warn", fmt.Sprintf("ownership schedule: could not record the run of %q: %v", m.Name, err))
	}
}
