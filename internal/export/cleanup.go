// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// CleanupStore is the interface consumed by the cleanup worker. It abstracts
// the subset of datastore.DB methods needed to discover and mark expired
// export jobs. Using an interface allows the cleanup logic to be tested with
// in-memory stubs.
type CleanupStore interface {
	// ListExpiredExportJobs returns completed export jobs whose expires_at
	// is before the given time.
	ListExpiredExportJobs(ctx context.Context, now time.Time) ([]datastore.ExportJob, error)

	// UpdateExportJobExpired marks a completed export job as expired.
	UpdateExportJobExpired(ctx context.Context, id string) error
}

// Compile-time assertion: *datastore.DB satisfies CleanupStore.
var _ CleanupStore = (*datastore.DB)(nil)

// CleanupResult holds statistics from a single cleanup pass.
type CleanupResult struct {
	// JobsExpired is the number of export jobs transitioned to "expired" status.
	JobsExpired int

	// FilesDeleted is the number of export files removed from disk.
	FilesDeleted int

	// Errors collects non-fatal errors encountered during cleanup. Each
	// entry describes a single job that could not be fully cleaned up.
	Errors []string
}

// CleanupExpiredExports finds completed export jobs whose retention period
// has elapsed, deletes their output files from disk, and marks the jobs as
// expired in the database.
//
// The function is designed to be called periodically (e.g. every hour) by a
// ticker in main.go or by the collection scheduler. It is safe to call
// concurrently — each invocation works on its own snapshot of expired jobs.
//
// Non-fatal errors (e.g. file already deleted, single job update failure)
// are collected in CleanupResult.Errors rather than aborting the entire pass.
// A fatal error (e.g. cannot query the database at all) is returned directly.
func CleanupExpiredExports(ctx context.Context, db CleanupStore, outputDir string) (*CleanupResult, error) {
	result := &CleanupResult{}

	expired, err := db.ListExpiredExportJobs(ctx, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("export cleanup: listing expired jobs: %w", err)
	}

	for _, job := range expired {
		jobErr := cleanupSingleJob(ctx, db, job)
		if jobErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("job %s: %v", job.ID, jobErr))
			// Continue processing other jobs — this is non-fatal.
			continue
		}
		result.JobsExpired++

		// Count the file deletion. The file may have already been removed
		// (e.g. by manual cleanup or a previous partial run), which is fine.
		if job.FilePath != "" {
			result.FilesDeleted++
		}
	}

	return result, nil
}

// cleanupSingleJob handles the cleanup of one expired export job: delete the
// file on disk (if it exists), then mark the job as expired in the database.
func cleanupSingleJob(ctx context.Context, db CleanupStore, job datastore.ExportJob) error {
	// Attempt to delete the export file from disk.
	if job.FilePath != "" {
		if err := os.Remove(job.FilePath); err != nil && !os.IsNotExist(err) {
			// The file exists but we can't delete it — this is an error
			// worth reporting, but we still try to mark the job as expired
			// so we don't retry the file deletion forever.
			markErr := db.UpdateExportJobExpired(ctx, job.ID)
			if markErr != nil {
				return fmt.Errorf("deleting file %q: %v; also failed to mark expired: %w", job.FilePath, err, markErr)
			}
			return fmt.Errorf("deleting file %q: %w (job marked expired anyway)", job.FilePath, err)
		}
		// File deleted successfully (or didn't exist).
	}

	// Mark the job as expired in the database.
	if err := db.UpdateExportJobExpired(ctx, job.ID); err != nil {
		return fmt.Errorf("marking job expired: %w", err)
	}

	return nil
}

// StartCleanupTicker launches a background goroutine that runs
// CleanupExpiredExports at the specified interval. It returns a stop function
// that the caller should invoke during shutdown to terminate the ticker.
//
// The logFn callback receives structured log messages. If nil, cleanup runs
// silently. The level parameter is one of "DEBUG", "INFO", "WARN", "ERROR".
func StartCleanupTicker(
	db CleanupStore,
	outputDir string,
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

	go func() {
		for {
			select {
			case <-done:
				ticker.Stop()
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				result, err := CleanupExpiredExports(ctx, db, outputDir)
				cancel()

				if err != nil {
					log("ERROR", "export cleanup failed: %v", err)
					continue
				}

				if result.JobsExpired > 0 || len(result.Errors) > 0 {
					log("INFO", "export cleanup: expired=%d files_deleted=%d errors=%d",
						result.JobsExpired, result.FilesDeleted, len(result.Errors))
				}

				for _, e := range result.Errors {
					log("WARN", "export cleanup: %s", e)
				}
			}
		}
	}()

	return func() {
		close(done)
	}
}
