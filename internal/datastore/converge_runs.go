// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ingest"
)

// BulkUpsertConvergeRuns persists a batch of normalised converge runs in ONE
// transaction (a partial/failed batch persists nothing). It ensures the day
// partition exists for each run's end_time before inserting, and dedups on the
// (run_id, end_time) primary key — a run delivered twice (e.g. Server proxy AND
// Automate) inserts once. Returns the number of rows actually inserted (i.e.
// excluding deduped duplicates).
func (db *DB) BulkUpsertConvergeRuns(ctx context.Context, runs []ingest.ConvergeRun) (int, error) {
	if len(runs) == 0 {
		return 0, nil
	}

	inserted := 0
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		ensured := make(map[string]struct{})
		for i := range runs {
			day := runs[i].EndTime.UTC().Format("2006-01-02")
			if _, ok := ensured[day]; ok {
				continue
			}
			if _, err := tx.ExecContext(ctx, `SELECT converge_runs_ensure_partition($1::date)`, day); err != nil {
				return fmt.Errorf("datastore: ensuring converge_runs partition %s: %w", day, err)
			}
			ensured[day] = struct{}{}
		}

		for i := range runs {
			n, err := insertConvergeRun(ctx, tx, &runs[i])
			if err != nil {
				return err
			}
			inserted += n
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

const insertConvergeRunSQL = `
INSERT INTO converge_runs (
    run_id, organisation, node_name, source_fqdn, chef_server_fqdn, status,
    chef_version, start_time, end_time, run_list, expanded_run_list, cookbooks,
    total_resource_count, updated_resource_count, error, failed_resource, shape
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
ON CONFLICT (run_id, end_time) DO NOTHING`

func insertConvergeRun(ctx context.Context, tx *sql.Tx, r *ingest.ConvergeRun) (int, error) {
	runList := r.RunList
	if runList == nil {
		runList = []string{}
	}
	runListJSON, err := json.Marshal(runList)
	if err != nil {
		return 0, fmt.Errorf("datastore: marshal run_list for %q: %w", r.RunID, err)
	}

	cookbooks := r.Cookbooks
	if cookbooks == nil {
		cookbooks = map[string]string{}
	}
	cookbooksJSON, err := json.Marshal(cookbooks)
	if err != nil {
		return 0, fmt.Errorf("datastore: marshal cookbooks for %q: %w", r.RunID, err)
	}

	// Nullable JSONB / timestamp columns: pass nil (SQL NULL) when absent.
	var expandedRunList any
	if len(r.ExpandedRunList) > 0 {
		expandedRunList = []byte(r.ExpandedRunList)
	}
	var errJSON any
	if r.Error != nil {
		b, err := json.Marshal(r.Error)
		if err != nil {
			return 0, fmt.Errorf("datastore: marshal error for %q: %w", r.RunID, err)
		}
		errJSON = b
	}
	var failedResourceJSON any
	if r.FailedResource != nil {
		b, err := json.Marshal(r.FailedResource)
		if err != nil {
			return 0, fmt.Errorf("datastore: marshal failed_resource for %q: %w", r.RunID, err)
		}
		failedResourceJSON = b
	}
	var startTime any
	if !r.StartTime.IsZero() {
		startTime = r.StartTime
	}

	res, err := tx.ExecContext(ctx, insertConvergeRunSQL,
		r.RunID, r.Organisation, r.NodeName, nullString(r.SourceFQDN), nullString(r.ChefServerFQDN),
		r.Status, nullString(r.ChefVersion), startTime, r.EndTime, runListJSON, expandedRunList,
		cookbooksJSON, r.TotalResourceCount, r.UpdatedResourceCount, errJSON, failedResourceJSON, r.Shape,
	)
	if err != nil {
		return 0, fmt.Errorf("datastore: inserting converge_run %q: %w", r.RunID, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PurgeConvergeRunPartitions drops every whole day partition of converge_runs
// whose entire time range predates olderThan. Retention is by dropping
// partitions, never row-level deletes on the hot path. Returns the number of
// partitions dropped. The dropped table name is re-derived from the parsed
// partition date, so the catalogue name is never interpolated verbatim.
func (db *DB) PurgeConvergeRunPartitions(ctx context.Context, olderThan time.Time) (int, error) {
	const listSQL = `
SELECT c.relname
FROM pg_inherits i
JOIN pg_class c ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = 'converge_runs'`

	rows, err := db.pool.QueryContext(ctx, listSQL)
	if err != nil {
		return 0, fmt.Errorf("datastore: listing converge_runs partitions: %w", err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return 0, fmt.Errorf("datastore: scanning partition name: %w", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("datastore: iterating partitions: %w", err)
	}
	rows.Close()

	cutoff := olderThan.UTC()
	dropped := 0
	for _, name := range names {
		suffix := strings.TrimPrefix(name, "converge_runs_")
		if suffix == name {
			continue // not one of our day partitions
		}
		day, err := time.Parse("20060102", suffix)
		if err != nil {
			continue // unrecognised naming — leave it alone
		}
		// Partition covers [day, day+1). Drop only when the upper bound is at or
		// before the cutoff, i.e. the whole day is older than the retention window.
		if day.AddDate(0, 0, 1).After(cutoff) {
			continue
		}
		safe := "converge_runs_" + day.Format("20060102")
		if _, err := db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS `+safe); err != nil {
			return dropped, fmt.Errorf("datastore: dropping partition %s: %w", safe, err)
		}
		dropped++
	}
	return dropped, nil
}
