// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
)

// ListAllServerCookbookCookstyleResultsByTargetVersion returns all server
// cookbook cookstyle results for the given target Chef version, across all
// organisations. Used by the violations browser to present a flat,
// cross-org list.
func (db *DB) ListAllServerCookbookCookstyleResultsByTargetVersion(ctx context.Context, targetChefVersion string) ([]ServerCookbookCookstyleResult, error) {
	return db.listAllServerCookbookCookstyleResultsByTargetVersion(ctx, db.q(), targetChefVersion)
}

func (db *DB) listAllServerCookbookCookstyleResultsByTargetVersion(ctx context.Context, q queryable, targetChefVersion string) ([]ServerCookbookCookstyleResult, error) {
	const query = `
		SELECT organisation_name, cookbook_name, cookbook_version,
		       target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       error_message, scanned_at, created_at
		  FROM server_cookbook_cookstyle_results
		 WHERE target_chef_version = $1
		 ORDER BY cookbook_name, cookbook_version, organisation_name
	`
	return scanServerCookbookCookstyleResults(q.QueryContext(ctx, query, targetChefVersion))
}

// ListGitRepoCookstyleResultsByTargetVersion returns all git repo cookstyle
// results for a single target Chef version. This is a convenience wrapper
// around ListGitRepoCookstyleResultsByTargetVersions for single-version
// lookups.
func (db *DB) ListGitRepoCookstyleResultsByTargetVersion(ctx context.Context, targetChefVersion string) ([]GitRepoCookstyleResult, error) {
	if targetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target chef version is required")
	}
	return db.ListGitRepoCookstyleResultsByTargetVersions(ctx, []string{targetChefVersion})
}
