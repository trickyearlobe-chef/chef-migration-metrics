// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
)

// TopQueryStat holds per-query statistics from pg_stat_statements.
type TopQueryStat struct {
	Query          string  `json:"query"`
	Calls          int64   `json:"calls"`
	TotalTimeMs    float64 `json:"total_time_ms"`
	MeanTimeMs     float64 `json:"mean_time_ms"`
	MinTimeMs      float64 `json:"min_time_ms"`
	MaxTimeMs      float64 `json:"max_time_ms"`
	Rows           int64   `json:"rows"`
	SharedBlksHit  int64   `json:"shared_blks_hit"`
	SharedBlksRead int64   `json:"shared_blks_read"`
}

// TableStat holds per-table statistics from pg_stat_user_tables.
type TableStat struct {
	TableName   string  `json:"table_name"`
	SeqScan     int64   `json:"seq_scan"`
	SeqTupRead  int64   `json:"seq_tup_read"`
	IdxScan     int64   `json:"idx_scan"`
	IdxTupFetch int64   `json:"idx_tup_fetch"`
	NLiveTup    int64   `json:"n_live_tup"`
	NDeadTup    int64   `json:"n_dead_tup"`
	LastVacuum  *string `json:"last_vacuum"`
	LastAnalyze *string `json:"last_analyze"`
}

// IndexStat holds per-index statistics from pg_stat_user_indexes.
type IndexStat struct {
	TableName   string `json:"table_name"`
	IndexName   string `json:"index_name"`
	IdxScan     int64  `json:"idx_scan"`
	IdxTupRead  int64  `json:"idx_tup_read"`
	IdxTupFetch int64  `json:"idx_tup_fetch"`
	SizeBytes   int64  `json:"size_bytes"`
}

// ActiveQuery represents a currently running query from pg_stat_activity.
type ActiveQuery struct {
	PID           int     `json:"pid"`
	State         string  `json:"state"`
	Query         string  `json:"query"`
	DurationMs    float64 `json:"duration_ms"`
	WaitEventType *string `json:"wait_event_type"`
	WaitEvent     *string `json:"wait_event"`
}

// PgStatStatementsAvailable returns true if the pg_stat_statements
// extension is installed and queryable.
func (db *DB) PgStatStatementsAvailable(ctx context.Context) bool {
	var one int
	err := db.pool.QueryRowContext(ctx,
		`SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements'`,
	).Scan(&one)
	return err == nil
}

// TopQueryStats returns the top N queries by total execution time from
// pg_stat_statements. Returns nil, nil if the extension is unavailable.
func (db *DB) TopQueryStats(ctx context.Context, limit int) ([]TopQueryStat, error) {
	if !db.PgStatStatementsAvailable(ctx) {
		return nil, nil
	}

	rows, err := db.pool.QueryContext(ctx, `
		SELECT query, calls, total_exec_time, mean_exec_time, min_exec_time,
		       max_exec_time, rows, shared_blks_hit, shared_blks_read
		FROM pg_stat_statements
		WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
		ORDER BY total_exec_time DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying top query stats: %w", err)
	}
	defer rows.Close()

	var stats []TopQueryStat
	for rows.Next() {
		var s TopQueryStat
		if err := rows.Scan(
			&s.Query, &s.Calls, &s.TotalTimeMs, &s.MeanTimeMs,
			&s.MinTimeMs, &s.MaxTimeMs, &s.Rows,
			&s.SharedBlksHit, &s.SharedBlksRead,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning top query stat row: %w", err)
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating top query stats: %w", err)
	}
	return stats, nil
}

// TableStats returns per-table statistics from pg_stat_user_tables,
// ordered by sequential tuples read descending.
func (db *DB) TableStats(ctx context.Context) ([]TableStat, error) {
	rows, err := db.pool.QueryContext(ctx, `
		SELECT relname, seq_scan, seq_tup_read, COALESCE(idx_scan, 0),
		       COALESCE(idx_tup_fetch, 0), n_live_tup, n_dead_tup,
		       to_char(last_vacuum, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(last_analyze, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM pg_stat_user_tables
		ORDER BY seq_tup_read DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying table stats: %w", err)
	}
	defer rows.Close()

	var stats []TableStat
	for rows.Next() {
		var s TableStat
		if err := rows.Scan(
			&s.TableName, &s.SeqScan, &s.SeqTupRead,
			&s.IdxScan, &s.IdxTupFetch, &s.NLiveTup, &s.NDeadTup,
			&s.LastVacuum, &s.LastAnalyze,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning table stat row: %w", err)
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating table stats: %w", err)
	}
	return stats, nil
}

// IndexStats returns per-index statistics from pg_stat_user_indexes,
// ordered by index scans descending.
func (db *DB) IndexStats(ctx context.Context) ([]IndexStat, error) {
	rows, err := db.pool.QueryContext(ctx, `
		SELECT t.relname, i.relname, sui.idx_scan, sui.idx_tup_read,
		       sui.idx_tup_fetch, pg_relation_size(sui.indexrelid)
		FROM pg_stat_user_indexes sui
		JOIN pg_class t ON sui.relid = t.oid
		JOIN pg_class i ON sui.indexrelid = i.oid
		ORDER BY sui.idx_scan DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying index stats: %w", err)
	}
	defer rows.Close()

	var stats []IndexStat
	for rows.Next() {
		var s IndexStat
		if err := rows.Scan(
			&s.TableName, &s.IndexName, &s.IdxScan,
			&s.IdxTupRead, &s.IdxTupFetch, &s.SizeBytes,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning index stat row: %w", err)
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating index stats: %w", err)
	}
	return stats, nil
}

// ActiveQueries returns currently running queries from pg_stat_activity,
// excluding idle connections and the caller's own backend.
func (db *DB) ActiveQueries(ctx context.Context) ([]ActiveQuery, error) {
	rows, err := db.pool.QueryContext(ctx, `
		SELECT pid, state, query,
		       EXTRACT(EPOCH FROM (now() - query_start)) * 1000,
		       wait_event_type, wait_event
		FROM pg_stat_activity
		WHERE datname = current_database()
		  AND state != 'idle'
		  AND pid != pg_backend_pid()
		ORDER BY query_start ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying active queries: %w", err)
	}
	defer rows.Close()

	var queries []ActiveQuery
	for rows.Next() {
		var q ActiveQuery
		var durationMs sql.NullFloat64
		if err := rows.Scan(
			&q.PID, &q.State, &q.Query,
			&durationMs, &q.WaitEventType, &q.WaitEvent,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning active query row: %w", err)
		}
		q.DurationMs = floatFromNull(durationMs)
		queries = append(queries, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating active queries: %w", err)
	}
	return queries, nil
}

// ResetPgStats calls pg_stat_statements_reset() (if available) and
// pg_stat_reset() to clear accumulated statistics.
func (db *DB) ResetPgStats(ctx context.Context) error {
	if db.PgStatStatementsAvailable(ctx) {
		if _, err := db.pool.ExecContext(ctx, `SELECT pg_stat_statements_reset()`); err != nil {
			return fmt.Errorf("datastore: resetting pg_stat_statements: %w", err)
		}
	}

	if _, err := db.pool.ExecContext(ctx, `SELECT pg_stat_reset()`); err != nil {
		return fmt.Errorf("datastore: resetting pg_stat: %w", err)
	}
	return nil
}

// VacuumFull runs VACUUM FULL on the database to reclaim disk space from dead
// tuples. This rewrites all tables and reclaims bloat but requires an exclusive
// lock on each table — callers should be aware this blocks concurrent access.
func (db *DB) VacuumFull(ctx context.Context) error {
	if _, err := db.pool.ExecContext(ctx, `VACUUM FULL`); err != nil {
		return fmt.Errorf("datastore: vacuum full: %w", err)
	}
	return nil
}
