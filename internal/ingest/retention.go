// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ingest

import (
	"context"
	"fmt"
	"time"
)

// RetentionStore is the slice of the datastore the retention ticker needs.
// *datastore.DB satisfies it. Declared here (not imported from datastore) to
// avoid an import cycle — datastore already imports this package for ConvergeRun.
type RetentionStore interface {
	PurgeConvergeRunPartitions(ctx context.Context, olderThan time.Time) (int, error)
}

// StartRetentionTicker periodically drops converge_runs day partitions older than
// retentionDaysFn() days (read live each tick, so a config change takes effect
// without restart). It runs once immediately so a restart reclaims promptly, then
// every interval. Returns a stop func. Mirrors export.StartCleanupTicker.
//
// New partitions are NOT created here — the store creates each day partition
// on demand at insert time (converge_runs_ensure_partition); this only reaps.
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
			days = 2
		}
		// Cutoff at UTC midnight `days` days ago: a whole day partition is dropped
		// once its entire range predates the cutoff (see PurgeConvergeRunPartitions).
		cutoff := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -days)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		n, err := store.PurgeConvergeRunPartitions(ctx, cutoff)
		cancel()
		if err != nil {
			log("ERROR", "converge_runs retention purge failed: %v", err)
			return
		}
		if n > 0 {
			log("INFO", "converge_runs retention: dropped %d partition(s) older than %d day(s)", n, days)
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
