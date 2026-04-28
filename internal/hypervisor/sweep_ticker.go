// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"fmt"
	"time"
)

// StartSweepTicker launches a background goroutine that runs
// SweepOrphanVMs at the specified interval. Each tick executes a real
// (non-dry-run) sweep. It returns a stop function that the caller should
// invoke during shutdown to terminate the ticker.
//
// The logFn callback receives structured log messages. If nil, the sweep
// runs silently.
func StartSweepTicker(
	hyp Hypervisor,
	prefix string,
	ageThreshold time.Duration,
	interval time.Duration,
	logFn func(level, msg string, args ...any),
) (stop func()) {
	if interval <= 0 {
		interval = 30 * time.Minute
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
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				result, err := SweepOrphanVMs(ctx, hyp, prefix, ageThreshold, false)
				cancel()

				if err != nil {
					log("ERROR", "orphan sweep failed: %v", err)
					continue
				}

				if result.Destroyed > 0 || result.Errors > 0 {
					log("INFO", "orphan sweep: scanned=%d destroyed=%d skipped_young=%d skipped_unparsed=%d errors=%d",
						result.Scanned, result.Destroyed, result.SkippedTooYoung, result.SkippedUnparsed, result.Errors)
				}
			}
		}
	}()

	return func() {
		close(done)
	}
}
