// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"fmt"
	"time"
)

// RetentionStore is the slice of the datastore the log retention ticker needs.
// *datastore.DB satisfies it.
type RetentionStore interface {
	PurgeLogEntryPartitions(ctx context.Context, olderThan time.Time) (int, error)
}

// StartRetentionTicker periodically drops log_entries day partitions older than
// retentionDaysFn() days, read live each tick so a change takes effect without
// a restart. It purges once immediately, then every interval, and returns a
// stop func. Mirrors ingest.StartRetentionTicker.
//
// Expiry previously ran only as the last step of a collection run, which made
// it conditional on collection reaching that point: a failed run, a run skipped
// because the previous one overran its tick, or an early return on an empty
// organisation list all meant logs were never expired. That is how a log table
// reaches 26GB. Retention is a property of the data, not of the collector, so
// it runs on its own clock.
//
// A retention of zero or less means "keep everything" and purges nothing. The
// alternative — falling back to a default, as the ingest ticker does — would
// delete history the operator asked to keep.
func StartRetentionTicker(
	store RetentionStore,
	retentionDaysFn func() int,
	interval time.Duration,
	logFn func(level, msg string),
) (stop func()) {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	log := func(level, format string, args ...any) {
		if logFn != nil {
			logFn(level, fmt.Sprintf(format, args...))
		}
	}

	purge := func() {
		days := retentionDaysFn()
		if days < 1 {
			return
		}

		// Cut at UTC midnight so the boundary does not drift with the time of
		// day the process happened to start.
		cutoff := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -days)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		n, err := store.PurgeLogEntryPartitions(ctx, cutoff)
		cancel()
		if err != nil {
			log("ERROR", "log retention purge failed: %v", err)
			return
		}
		if n > 0 {
			log("INFO", "log retention: dropped %d partition(s) older than %d day(s)", n, days)
		}
	}

	go func() {
		purge()
		for {
			select {
			case <-done:
				ticker.Stop()
				return
			case <-ticker.C:
				purge()
			}
		}
	}()

	return func() { close(done) }
}
