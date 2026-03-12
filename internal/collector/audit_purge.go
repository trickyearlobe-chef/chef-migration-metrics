// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// StartAuditLogPurge starts a background goroutine that daily purges
// ownership audit log entries older than the configured retention period.
// It runs once immediately on start, then repeats every 24 hours.
// The goroutine exits when ctx is cancelled.
func StartAuditLogPurge(ctx context.Context, db *datastore.DB, retentionDays int, logger *logging.Logger) {
	if retentionDays <= 0 {
		// 0 means retain indefinitely.
		return
	}

	log := logger.WithScope(logging.ScopeOwnership)

	purge := func() {
		cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		deleted, err := db.PurgeAuditLog(ctx, cutoff)
		if err != nil {
			log.Error(fmt.Sprintf("audit log purge failed: %v", err))
			return
		}
		if deleted > 0 {
			log.Info(fmt.Sprintf("purged %d audit log entries older than %d days", deleted, retentionDays))
		}
	}

	// Run once immediately.
	purge()

	// Then run daily.
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				purge()
			}
		}
	}()
}
