// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
)

// Unscoped result listings for the one-time precise CookStyle status backfill.
// Unlike the per-cop propagation queries (ListServerCookbookCookstyleResultsWithCop
// et al.), these return every result across all target versions, projected to the
// lightweight CookstyleResultRef (natural key + target + offences + current
// verdict) — deliberately excluding the heavy process_stdout / process_stderr
// columns so a single pass over ~tens of thousands of rows stays cheap. The
// analysis backfill groups these by target, builds one resolver per target, and
// re-derives the rollup status from the stored offences. See
// internal/analysis/cookstyle_backfill.go.

// ListAllServerCookbookCookstyleResultRefs returns every server cookbook
// cookstyle result as a lightweight ref, across all target versions.
func (db *DB) ListAllServerCookbookCookstyleResultRefs(ctx context.Context) ([]CookstyleResultRef, error) {
	const query = `
		SELECT organisation_name, cookbook_name, cookbook_version,
		       COALESCE(target_chef_version, ''),
		       COALESCE(error_message, ''), passed, offences, cookstyle_status
		FROM server_cookbook_cookstyle_results`

	rows, err := db.q().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing all server cookstyle result refs: %w", err)
	}
	defer rows.Close()

	var refs []CookstyleResultRef
	for rows.Next() {
		var r CookstyleResultRef
		if err := rows.Scan(&r.OrganisationName, &r.CookbookName, &r.CookbookVersion,
			&r.TargetChefVersion, &r.ErrorMessage, &r.Passed, &r.Offences, &r.CookstyleStatus); err != nil {
			return nil, fmt.Errorf("datastore: scanning server cookstyle result ref: %w", err)
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating server cookstyle result refs: %w", err)
	}
	return refs, nil
}

// ListAllGitRepoCookstyleResultRefs returns every git repo cookstyle result as a
// lightweight ref, across all target versions.
func (db *DB) ListAllGitRepoCookstyleResultRefs(ctx context.Context) ([]CookstyleResultRef, error) {
	const query = `
		SELECT git_repo_name, git_repo_url,
		       COALESCE(target_chef_version, ''),
		       COALESCE(error_message, ''), passed, offences, cookstyle_status
		FROM git_repo_cookstyle_results`

	rows, err := db.q().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing all git cookstyle result refs: %w", err)
	}
	defer rows.Close()

	var refs []CookstyleResultRef
	for rows.Next() {
		var r CookstyleResultRef
		if err := rows.Scan(&r.GitRepoName, &r.GitRepoURL,
			&r.TargetChefVersion, &r.ErrorMessage, &r.Passed, &r.Offences, &r.CookstyleStatus); err != nil {
			return nil, fmt.Errorf("datastore: scanning git cookstyle result ref: %w", err)
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git cookstyle result refs: %w", err)
	}
	return refs, nil
}
