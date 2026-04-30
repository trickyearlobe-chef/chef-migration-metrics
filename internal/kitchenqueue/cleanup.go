// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package kitchenqueue

import (
	"context"
	"fmt"
	"time"
)

// CleanupStore is the subset of datastore.DB needed for queue cleanup.
type CleanupStore interface {
	PurgeCompletedKitchenRuns(ctx context.Context, olderThan time.Time) (int64, error)
}

// StartCleanupTicker launches a background goroutine that periodically
// purges terminal kitchen queue items older than maxAge. Returns a stop
// function that should be called during shutdown.
func StartCleanupTicker(
	db CleanupStore,
	interval time.Duration,
	maxAge time.Duration,
	logFn func(level, msg string),
) (stop func()) {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	if maxAge <= 0 {
		maxAge = 24 * time.Hour
	}

	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	log := func(level, format string, args ...any) {
		if logFn != nil {
			logFn(level, fmt.Sprintf(format, args...))
		}
	}

	go func() {
		for {
			select {
			case <-done:
				ticker.Stop()
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				cutoff := time.Now().Add(-maxAge)
				n, err := db.PurgeCompletedKitchenRuns(ctx, cutoff)
				cancel()
				if err != nil {
					log("ERROR", "kitchen queue cleanup failed: %v", err)
				} else if n > 0 {
					log("INFO", "kitchen queue cleanup: purged %d completed items older than %s", n, maxAge)
				}
			}
		}
	}()

	return func() { close(done) }
}
