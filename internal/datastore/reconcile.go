// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// PurgeStaleTargetVersionResult reports what was deleted during reconciliation.
type PurgeStaleTargetVersionResult struct {
	NodeReadiness                     int64
	ServerCookbookCookstyleResults    int64
	ServerCookbookComplexity          int64
	ServerCookbookAutocorrectPreviews int64
	GitRepoCookstyleResults           int64
	GitRepoComplexity                 int64
	GitRepoAutocorrectPreviews        int64
	MetricSnapshots                   int64
}

// Total returns the total number of rows deleted across all tables.
func (r PurgeStaleTargetVersionResult) Total() int64 {
	return r.NodeReadiness +
		r.ServerCookbookCookstyleResults +
		r.ServerCookbookComplexity +
		r.ServerCookbookAutocorrectPreviews +
		r.GitRepoCookstyleResults +
		r.GitRepoComplexity +
		r.GitRepoAutocorrectPreviews +
		r.MetricSnapshots
}

// PurgeStaleTargetVersionData removes all analysis records whose
// target_chef_version is not in the provided list of active versions.
// This is called at startup to clean up data for target versions that
// have been removed from the configuration.
//
// If activeVersions is empty, no deletions are performed (safety check —
// an empty list likely means a config error, not "delete everything").
func (db *DB) PurgeStaleTargetVersionData(ctx context.Context, activeVersions []string) (*PurgeStaleTargetVersionResult, error) {
	if len(activeVersions) == 0 {
		return &PurgeStaleTargetVersionResult{}, nil
	}

	result := &PurgeStaleTargetVersionResult{}

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Build the NOT IN clause with parameterised placeholders.
		placeholders := make([]string, len(activeVersions))
		args := make([]any, len(activeVersions))
		for i, v := range activeVersions {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = v
		}
		notIn := strings.Join(placeholders, ", ")

		// Each table that stores target_chef_version-scoped data.
		tables := []struct {
			name  string
			query string
			dest  *int64
		}{
			{
				name:  "node_readiness",
				query: "DELETE FROM node_readiness WHERE target_chef_version NOT IN (" + notIn + ")",
				dest:  &result.NodeReadiness,
			},
			{
				name:  "server_cookbook_cookstyle_results",
				query: "DELETE FROM server_cookbook_cookstyle_results WHERE target_chef_version NOT IN (" + notIn + ")",
				dest:  &result.ServerCookbookCookstyleResults,
			},
			{
				name:  "server_cookbook_complexity",
				query: "DELETE FROM server_cookbook_complexity WHERE target_chef_version NOT IN (" + notIn + ")",
				dest:  &result.ServerCookbookComplexity,
			},
			{
				name:  "server_cookbook_autocorrect_previews",
				query: "DELETE FROM server_cookbook_autocorrect_previews WHERE target_chef_version NOT IN (" + notIn + ")",
				dest:  &result.ServerCookbookAutocorrectPreviews,
			},
			{
				name:  "git_repo_cookstyle_results",
				query: "DELETE FROM git_repo_cookstyle_results WHERE target_chef_version NOT IN (" + notIn + ")",
				dest:  &result.GitRepoCookstyleResults,
			},
			{
				name:  "git_repo_complexity",
				query: "DELETE FROM git_repo_complexity WHERE target_chef_version NOT IN (" + notIn + ")",
				dest:  &result.GitRepoComplexity,
			},
			{
				name:  "git_repo_autocorrect_previews",
				query: "DELETE FROM git_repo_autocorrect_previews WHERE target_chef_version NOT IN (" + notIn + ")",
				dest:  &result.GitRepoAutocorrectPreviews,
			},
			{
				name:  "metric_snapshots",
				query: "DELETE FROM metric_snapshots WHERE target_chef_version IS NOT NULL AND target_chef_version NOT IN (" + notIn + ")",
				dest:  &result.MetricSnapshots,
			},
		}

		for _, t := range tables {
			res, err := tx.ExecContext(ctx, t.query, args...)
			if err != nil {
				return fmt.Errorf("datastore: purging stale target versions from %s: %w", t.name, err)
			}
			n, _ := res.RowsAffected()
			*t.dest = n
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}
