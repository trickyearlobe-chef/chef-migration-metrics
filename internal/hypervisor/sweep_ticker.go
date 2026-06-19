// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"sync"
	"time"
)

// sweepDisabledRecheck is how often the ticker wakes to re-read config while
// the scheduled sweep is disabled (Interval <= 0), so a runtime enable takes
// effect without a restart. It is a var (not const) so tests can shrink it.
var sweepDisabledRecheck = 1 * time.Minute

// SweepParams are the live, per-tick inputs to the scheduled orphan sweep.
// They are read fresh on every cycle via the paramsFn passed to
// StartSweepTicker, so config changes (interval, age, prefix, enable/disable)
// take effect without a restart (CLAUDE.md dynamic-config mandate). An
// Interval <= 0 disables the scheduled sweep; the manual endpoint is
// unaffected.
type SweepParams struct {
	Prefix   string
	Age      time.Duration
	Interval time.Duration
}

// StartSweepTicker launches a background goroutine that periodically runs a
// real (non-dry-run) orphan sweep. Both the schedule and the hypervisor client
// are resolved live on every cycle:
//
//   - paramsFn returns the current prefix/age/interval. Interval <= 0 means the
//     scheduled sweep is disabled; the ticker idles, re-checking every
//     sweepDisabledRecheck so a runtime re-enable is picked up without a restart.
//   - hypFn builds a fresh hypervisor client from live config + resolved
//     credentials each tick (there is no long-lived client). (nil, nil) means no
//     hypervisor is configured — a quiet no-op.
//
// CAVEAT (deferred folder scoping): the scheduled sweep is scoped by name
// prefix + age only — ListManagedVMs has no folder filter yet. On a shared
// vSphere this can match foreign kitchen-* VMs. A folder filter is tracked as
// follow-up work (plans/todo-tech-debt.md). A one-time WARN is logged at start.
//
// The logFn callback receives structured log messages. If nil, the sweep runs
// silently. The returned stop function is idempotent and terminates the ticker.
func StartSweepTicker(
	paramsFn func() SweepParams,
	hypFn func(ctx context.Context) (Hypervisor, error),
	logFn func(level, msg string, args ...any),
) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})

	log := func(level, msg string, args ...any) {
		if logFn != nil {
			logFn(level, msg, args...)
		}
	}

	log("WARN", "scheduled orphan sweep is scoped by name prefix + age only (no folder filter); "+
		"on a shared hypervisor this can match VMs owned by other tools — folder scoping is deferred")

	go func() {
		defer close(stopped)
		for {
			wait := paramsFn().Interval
			if wait <= 0 {
				wait = sweepDisabledRecheck
			}

			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}

			// Re-read after waking: interval/enable may have changed mid-wait.
			p := paramsFn()
			if p.Interval <= 0 {
				continue // disabled at runtime
			}

			hyp, err := hypFn(ctx)
			if err != nil {
				if ctx.Err() == nil { // suppress noise caused by shutdown
					log("ERROR", "orphan sweep: build hypervisor client: %v", err)
				}
				continue
			}
			if hyp == nil {
				continue // no hypervisor configured — quiet no-op
			}

			sweepCtx, sweepCancel := context.WithTimeout(ctx, 5*time.Minute)
			result, err := SweepOrphanVMs(sweepCtx, hyp, p.Prefix, p.Age, false)
			sweepCancel()

			if err != nil {
				if ctx.Err() == nil {
					log("ERROR", "orphan sweep failed: %v", err)
				}
				continue
			}

			if result.Destroyed > 0 || result.Errors > 0 {
				log("INFO", "orphan sweep: scanned=%d destroyed=%d skipped_young=%d skipped_unparsed=%d errors=%d",
					result.Scanned, result.Destroyed, result.SkippedTooYoung, result.SkippedUnparsed, result.Errors)
			}
		}
	}()

	// stop is synchronous: cancel the context (aborting any in-flight sweep)
	// and wait for the goroutine to exit, so shutdown is ordered and callers
	// observe a fully stopped ticker. Safe to call more than once.
	var once sync.Once
	return func() {
		once.Do(cancel)
		<-stopped
	}
}
