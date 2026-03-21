// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"time"
)

// StartSessionCleanup launches a background goroutine that periodically
// removes expired sessions from the database. It runs an initial cleanup
// immediately, then repeats every hour. The goroutine exits when ctx is
// cancelled.
//
// This replaces the one-shot CleanupExpired call at startup, ensuring
// expired session rows are purged continuously rather than accumulating
// between process restarts.
func StartSessionCleanup(ctx context.Context, mgr *SessionManager) {
	cleanup := func() {
		n, err := mgr.CleanupExpired(ctx)
		if err != nil {
			mgr.logf("WARN", "periodic session cleanup failed: %v", err)
			return
		}
		if n > 0 {
			mgr.logf("INFO", "periodic session cleanup: removed %d expired session(s)", n)
		}
	}

	// Run once immediately (replaces the startup one-shot).
	cleanup()

	// Then run hourly.
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanup()
			}
		}
	}()
}
