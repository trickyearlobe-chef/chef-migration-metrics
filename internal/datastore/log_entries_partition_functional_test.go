// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Far-future dates keep these partitions clear of real data in the shared
// cmm_test DB, and they are dropped in cleanup.
var (
	logD1 = time.Date(2041, 3, 1, 9, 0, 0, 0, time.UTC)
	logD2 = time.Date(2041, 3, 2, 9, 0, 0, 0, time.UTC)
	logD3 = time.Date(2041, 3, 3, 9, 0, 0, 0, time.UTC)
)

func dropLogPartitions(t *testing.T, db *DB, days ...time.Time) {
	t.Helper()
	for _, d := range days {
		name := "log_entries_" + d.UTC().Format("20060102")
		if _, err := db.pool.ExecContext(context.Background(), `DROP TABLE IF EXISTS `+name); err != nil {
			t.Logf("cleanup: dropping %s: %v", name, err)
		}
	}
}

func insertLogAt(t *testing.T, db *DB, ts time.Time, msg string) {
	t.Helper()
	_, err := db.InsertLogEntry(context.Background(), InsertLogEntryParams{
		Timestamp: ts,
		Severity:  "INFO",
		Scope:     "startup",
		Message:   msg,
	})
	if err != nil {
		t.Fatalf("inserting log entry at %s: %v", ts, err)
	}
}

func logPartitionExists(t *testing.T, db *DB, day time.Time) bool {
	t.Helper()
	name := "log_entries_" + day.UTC().Format("20060102")
	var n int
	err := db.pool.QueryRowContext(context.Background(), `
SELECT count(*)
FROM pg_inherits i
JOIN pg_class c ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = 'log_entries' AND c.relname = $1`, name).Scan(&n)
	if err != nil {
		t.Fatalf("checking partition %s: %v", name, err)
	}
	return n > 0
}

// An insert must create its own day partition. Without this, the first log line
// of each new day fails to route — and the failure would be in the logging path
// itself, so it would be near-invisible.
func TestLogEntryPartitions_InsertCreatesPartition(t *testing.T) {
	db := testDB(t)
	dropLogPartitions(t, db, logD1)
	t.Cleanup(func() { dropLogPartitions(t, db, logD1) })

	if logPartitionExists(t, db, logD1) {
		t.Fatal("partition existed before the insert")
	}

	insertLogAt(t, db, logD1, "partition creation test")

	if !logPartitionExists(t, db, logD1) {
		t.Error("insert did not create its day partition")
	}
}

// Retention drops whole partitions. This is the change: expiry reclaims space
// immediately instead of leaving dead tuples for autovacuum.
func TestLogEntryPartitions_PurgeDropsOnlyFullyExpiredDays(t *testing.T) {
	db := testDB(t)
	dropLogPartitions(t, db, logD1, logD2, logD3)
	t.Cleanup(func() { dropLogPartitions(t, db, logD1, logD2, logD3) })

	insertLogAt(t, db, logD1, "day one")
	insertLogAt(t, db, logD2, "day two")
	insertLogAt(t, db, logD3, "day three")

	// Cut at the start of day three: days one and two are wholly expired, day
	// three is not.
	cutoff := time.Date(2041, 3, 3, 0, 0, 0, 0, time.UTC)

	// The count is not asserted: cmm_test is shared, and any other partition
	// predating the far-future cutoff (including today's, created by the
	// migration) is legitimately dropped too. What matters is which of *these*
	// partitions survive.
	dropped, err := db.PurgeLogEntryPartitions(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("purging: %v", err)
	}
	if dropped < 2 {
		t.Errorf("dropped %d partitions, want at least the 2 expired ones", dropped)
	}
	if logPartitionExists(t, db, logD1) || logPartitionExists(t, db, logD2) {
		t.Error("an expired partition survived the purge")
	}
	if !logPartitionExists(t, db, logD3) {
		t.Error("the current day's partition was dropped")
	}
}

// A partition is only expired once its *entire* range predates the cutoff. A
// cutoff mid-day must not drop that day, or entries newer than the retention
// window are destroyed along with it.
func TestLogEntryPartitions_PartialDayIsNotDropped(t *testing.T) {
	db := testDB(t)
	dropLogPartitions(t, db, logD1)
	t.Cleanup(func() { dropLogPartitions(t, db, logD1) })

	insertLogAt(t, db, logD1, "mid-day retention boundary")

	// Noon on the same day: the partition covers [day, day+1), so part of it is
	// still within retention.
	cutoff := time.Date(2041, 3, 1, 12, 0, 0, 0, time.UTC)

	dropped, err := db.PurgeLogEntryPartitions(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("purging: %v", err)
	}
	if dropped != 0 {
		t.Errorf("dropped %d partitions, want 0 for a partially-expired day", dropped)
	}
	if !logPartitionExists(t, db, logD1) {
		t.Error("a partially-expired partition was dropped")
	}
}

// Rows must remain readable through the parent table after partitioning —
// the log viewer queries log_entries, not the partitions.
func TestLogEntryPartitions_RowsReadableThroughParent(t *testing.T) {
	db := testDB(t)
	dropLogPartitions(t, db, logD1, logD2)
	t.Cleanup(func() { dropLogPartitions(t, db, logD1, logD2) })

	marker := fmt.Sprintf("parent-read-%d", time.Now().UnixNano())
	insertLogAt(t, db, logD1, marker)
	insertLogAt(t, db, logD2, marker)

	var n int
	err := db.pool.QueryRowContext(context.Background(),
		`SELECT count(*) FROM log_entries WHERE message = $1`, marker).Scan(&n)
	if err != nil {
		t.Fatalf("reading through parent: %v", err)
	}
	if n != 2 {
		t.Errorf("read %d rows through the parent table, want 2", n)
	}
}

// The id sequence must keep producing unique values across partitions — the
// primary key is (id, timestamp), so a repeated id in different partitions
// would not be caught by the constraint.
func TestLogEntryPartitions_IDsAreUniqueAcrossPartitions(t *testing.T) {
	db := testDB(t)
	dropLogPartitions(t, db, logD1, logD2)
	t.Cleanup(func() { dropLogPartitions(t, db, logD1, logD2) })

	marker := fmt.Sprintf("id-unique-%d", time.Now().UnixNano())
	insertLogAt(t, db, logD1, marker)
	insertLogAt(t, db, logD2, marker)

	var distinct, total int
	err := db.pool.QueryRowContext(context.Background(),
		`SELECT count(DISTINCT id), count(*) FROM log_entries WHERE message = $1`, marker).Scan(&distinct, &total)
	if err != nil {
		t.Fatalf("counting ids: %v", err)
	}
	if distinct != total {
		t.Errorf("got %d distinct ids across %d rows — ids collided between partitions", distinct, total)
	}
}
