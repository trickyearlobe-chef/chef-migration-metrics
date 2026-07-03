// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
)

// RecomputeGitRepoCompatibilityStatus recomputes the compatibility_status
// column for a single git repo from its latest cookstyle result for the given
// target Chef version.
//
// Call this after upserting or deleting a cookstyle result.
func (db *DB) RecomputeGitRepoCompatibilityStatus(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) error {
	const query = `
		UPDATE git_repos
		SET compatibility_status = COALESCE((
			SELECT CASE
				WHEN cs.error_message != '' THEN 'error'
				WHEN cs.passed = true THEN 'compatible'
				WHEN cs.passed = false THEN 'incompatible'
			END
			FROM git_repo_cookstyle_results cs
			WHERE cs.git_repo_name = $1
			  AND cs.git_repo_url = $2
			  AND cs.target_chef_version = $3
			ORDER BY cs.scanned_at DESC
			LIMIT 1
		), 'untested'),
		    cookstyle_status = COALESCE((
			SELECT CASE
				WHEN cs.error_message != '' THEN 'untested'
				ELSE NULLIF(cs.cookstyle_status, '')
			END
			FROM git_repo_cookstyle_results cs
			WHERE cs.git_repo_name = $1
			  AND cs.git_repo_url = $2
			  AND cs.target_chef_version = $3
			ORDER BY cs.scanned_at DESC
			LIMIT 1
		), 'untested'),
		    updated_at = now()
		WHERE name = $1 AND git_repo_url = $2`

	_, err := db.q().ExecContext(ctx, query, gitRepoName, gitRepoURL, targetChefVersion)
	if err != nil {
		return fmt.Errorf("datastore: recomputing git repo compatibility status: %w", err)
	}
	return nil
}

// RecomputeGitRepoTKStatus recomputes the tk_status, tk_passed, and tk_total
// columns for a single git repo from its active (non-excluded) kitchen results.
//
// Call this after upserting/deleting kitchen results or changing exclusions.
func (db *DB) RecomputeGitRepoTKStatus(ctx context.Context, gitRepoName, gitRepoURL string) error {
	const query = `
		UPDATE git_repos
		SET tk_passed = COALESCE(counts.passed_count, 0),
		    tk_total  = COALESCE(counts.total_count, 0),
		    tk_status = CASE
		        WHEN COALESCE(counts.passed_count, 0) > 0 AND COALESCE(counts.failed_count, 0) > 0 THEN 'partial'
		        WHEN COALESCE(counts.failed_count, 0) > 0 THEN 'failed'
		        WHEN COALESCE(counts.passed_count, 0) > 0 THEN 'passed'
		        ELSE 'untested'
		    END,
		    updated_at = now()
		FROM (
			SELECT
				COUNT(*) FILTER (WHERE passed = true) AS passed_count,
				COUNT(*) FILTER (WHERE passed = false OR timed_out = true) AS failed_count,
				COUNT(*) FILTER (WHERE passed IS NOT NULL OR timed_out = true) AS total_count
			FROM git_kitchen_results_active
			WHERE git_repo_name = $1
			  AND git_repo_url = $2
		) counts
		WHERE name = $1 AND git_repo_url = $2`

	_, err := db.q().ExecContext(ctx, query, gitRepoName, gitRepoURL)
	if err != nil {
		return fmt.Errorf("datastore: recomputing git repo TK status: %w", err)
	}
	return nil
}

// RecomputeGitRepoTKStatusByName recomputes TK status for all git repos with
// the given name (may have multiple URLs). Use when an exclusion changes.
func (db *DB) RecomputeGitRepoTKStatusByName(ctx context.Context, gitRepoName string) error {
	const query = `
		UPDATE git_repos gr
		SET tk_passed = COALESCE(counts.passed_count, 0),
		    tk_total  = COALESCE(counts.total_count, 0),
		    tk_status = CASE
		        WHEN COALESCE(counts.passed_count, 0) > 0 AND COALESCE(counts.failed_count, 0) > 0 THEN 'partial'
		        WHEN COALESCE(counts.failed_count, 0) > 0 THEN 'failed'
		        WHEN COALESCE(counts.passed_count, 0) > 0 THEN 'passed'
		        ELSE 'untested'
		    END,
		    updated_at = now()
		FROM (
			SELECT git_repo_name, git_repo_url,
				COUNT(*) FILTER (WHERE passed = true) AS passed_count,
				COUNT(*) FILTER (WHERE passed = false OR timed_out = true) AS failed_count,
				COUNT(*) FILTER (WHERE passed IS NOT NULL OR timed_out = true) AS total_count
			FROM git_kitchen_results_active
			WHERE git_repo_name = $1
			GROUP BY git_repo_name, git_repo_url
		) counts
		WHERE gr.name = counts.git_repo_name
		  AND gr.git_repo_url = counts.git_repo_url`

	_, err := db.q().ExecContext(ctx, query, gitRepoName)
	if err != nil {
		return fmt.Errorf("datastore: recomputing git repo TK status by name: %w", err)
	}
	return nil
}

// ResetAllGitRepoStatuses resets all materialised status columns to 'untested'.
// Call this when the active target Chef version changes (before results are
// invalidated and re-computed).
func (db *DB) ResetAllGitRepoStatuses(ctx context.Context) error {
	const query = `
		UPDATE git_repos
		SET compatibility_status = 'untested',
		    cookstyle_status = 'untested',
		    tk_status = 'untested',
		    tk_passed = 0,
		    tk_total = 0,
		    updated_at = now()`

	_, err := db.q().ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: resetting all git repo statuses: %w", err)
	}
	return nil
}

// ComputeGitRepoCompatibilityFromResult derives the compatibility status
// string from a cookstyle result. This is a pure function for use in tests
// and non-DB contexts.
func ComputeGitRepoCompatibilityFromResult(passed bool, errorMessage string) string {
	if errorMessage != "" {
		return "error"
	}
	if passed {
		return "compatible"
	}
	return "incompatible"
}

// ComputeGitRepoTKStatusFromCounts derives the TK status from pass/fail counts.
// Delegates to the canonical tkstatus package. Returns "untested" when both are 0.
func ComputeGitRepoTKStatusFromCounts(passed, failed int) string {
	s := tkstatus.ComputeTKStatus(passed, failed)
	if s == "" {
		return "untested"
	}
	return s
}
