// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"net/url"
	"os"
	"sync"
	"testing"
)

// TestFunctional_MigrateUp_ConcurrentCallersSerialize reproduces the failure it
// guards against: multiple processes (here, independent connection pools)
// applying migrations to the same fresh database at once. Without the
// pg_advisory_lock in MigrateUpFS they race on identical DDL and fail with
// "column ... already exists". With the lock, exactly one caller applies the full
// set and the rest observe it and no-op.
func TestFunctional_MigrateUp_ConcurrentCallersSerialize(t *testing.T) {
	baseURL := os.Getenv("CMM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("CMM_TEST_DATABASE_URL not set — skipping functional test")
	}
	ctx := context.Background()

	// A throwaway database so the migrations are genuinely unapplied — that is the
	// only state in which concurrent callers would race.
	const tmpDB = "cmm_migrate_race_test"
	admin, err := Open(baseURL)
	if err != nil {
		t.Fatalf("opening admin connection: %v", err)
	}
	defer admin.Close()
	if _, err := admin.pool.ExecContext(ctx, "DROP DATABASE IF EXISTS "+tmpDB+" WITH (FORCE)"); err != nil {
		t.Fatalf("dropping pre-existing temp db: %v", err)
	}
	if _, err := admin.pool.ExecContext(ctx, "CREATE DATABASE "+tmpDB); err != nil {
		t.Fatalf("creating temp db: %v", err)
	}
	defer admin.pool.ExecContext(ctx, "DROP DATABASE IF EXISTS "+tmpDB+" WITH (FORCE)")

	tmpURL, err := withDatabaseName(baseURL, tmpDB)
	if err != nil {
		t.Fatalf("building temp db url: %v", err)
	}

	// N independent pools migrate concurrently, released together via a barrier.
	const N = 6
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, N)
	applied := make([]int, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, oerr := Open(tmpURL)
			if oerr != nil {
				errs[i] = oerr
				return
			}
			defer d.Close()
			<-start
			applied[i], errs[i] = d.MigrateUp(ctx, "../../migrations")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Errorf("concurrent caller %d: MigrateUp failed: %v", i, e)
		}
	}

	// Exactly one caller applies the full set; every other sees them applied and
	// reports 0. No migration is applied twice.
	winners, total := 0, 0
	for _, a := range applied {
		total += a
		if a > 0 {
			winners++
		}
	}
	if winners != 1 {
		t.Errorf("expected exactly one caller to apply migrations, got %d (applied=%v)", winners, applied)
	}

	// The database ends fully migrated: a fresh MigrateUp is a no-op.
	verify, err := Open(tmpURL)
	if err != nil {
		t.Fatalf("reopening temp db: %v", err)
	}
	defer verify.Close()
	extra, err := verify.MigrateUp(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("verifying idempotency: %v", err)
	}
	if extra != 0 {
		t.Errorf("re-run applied %d migration(s); want 0 (already fully migrated)", extra)
	}
	if total == 0 {
		t.Error("no migrations were applied at all — test did not exercise the race")
	}
}

// withDatabaseName returns the connection URL with its database name replaced.
func withDatabaseName(rawURL, dbName string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.Path = "/" + dbName
	return u.String(), nil
}
