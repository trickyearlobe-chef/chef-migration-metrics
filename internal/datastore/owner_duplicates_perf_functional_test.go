// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// The scan has to stay bounded on a catalogue the size the customer actually
// has, and owner names cluster: people share surnames, and committer-derived
// names are email localparts, so a large share of any real catalogue is
// mutually similar. Comparing everything with everything took minutes on data
// this shape, which is why the scan takes only the nearest few per row.
//
// This test seeds a deliberately dense catalogue — every name has dozens of
// near-twins — and fails if a scan of it stops being quick.
func TestFunctional_OwnerDuplicates_ScanStaysBoundedOnADenseCatalogue(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds twenty thousand owners")
	}

	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM owner_aliases WHERE source = 'scale-test'",
		"DELETE FROM owners WHERE name LIKE 'scaleowner-%'",
	)

	const seedQuery = `
		INSERT INTO owners (name, owner_type)
		SELECT 'scaleowner-'
		       || chr(97 + (g % 26))
		       || (ARRAY['smith','jones','patel','okafor','nguyen','murray','ivanov','silva','kowalski','haddad'])[1 + (g % 10)]
		       || (g / 260)::text,
		       'individual'
		FROM generate_series(1, 20000) AS g
		ON CONFLICT DO NOTHING
	`
	if _, err := db.pool.ExecContext(ctx, seedQuery); err != nil {
		t.Fatalf("seeding owners: %v", err)
	}
	if _, err := db.pool.ExecContext(ctx, `
		INSERT INTO owner_aliases (owner_name, alias_type, alias_value, source)
		SELECT name, 'email', name || '@example-corp.test', 'scale-test'
		FROM owners WHERE name LIKE 'scaleowner-%'
		ON CONFLICT DO NOTHING
	`); err != nil {
		t.Fatalf("seeding aliases: %v", err)
	}
	if _, err := db.pool.ExecContext(ctx, `ANALYZE owners`); err != nil {
		t.Fatalf("analyzing owners: %v", err)
	}
	if _, err := db.pool.ExecContext(ctx, `ANALYZE owner_aliases`); err != nil {
		t.Fatalf("analyzing owner_aliases: %v", err)
	}

	var owners int
	if err := db.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM owners`).Scan(&owners); err != nil {
		t.Fatalf("counting owners: %v", err)
	}
	if owners < 5000 {
		t.Fatalf("seeded only %d owners; the test is not measuring what it claims", owners)
	}

	start := time.Now()
	found, err := db.RecomputeOwnerDuplicateCandidates(ctx)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RecomputeOwnerDuplicateCandidates: %v", err)
	}
	t.Logf("scanned %d owners in %s, found %d pairs", owners, elapsed.Round(time.Millisecond), found)

	// Generous: this runs on whatever machine the suite runs on. It is here to
	// catch a return to unbounded comparison, which was minutes, not seconds.
	if elapsed > 60*time.Second {
		t.Errorf("scanning %d owners took %s — the scan is no longer bounded", owners, elapsed)
	}

	// Reading a page must not depend on the size of the catalogue at all.
	start = time.Now()
	if _, _, err := db.ListOwnerDuplicateCandidates(ctx, OwnerDuplicateFilter{Limit: 25}); err != nil {
		t.Fatalf("ListOwnerDuplicateCandidates: %v", err)
	}
	if page := time.Since(start); page > 5*time.Second {
		t.Errorf("reading one page took %s — the list is not being read from the stored scan", page)
	}
}
