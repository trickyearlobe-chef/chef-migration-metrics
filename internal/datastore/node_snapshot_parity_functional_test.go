// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/lib/pq"
)

// Count-split parity suite.
//
// The node list now runs two statements — a lean COUNT(*) query and a
// windowless rows query — that share buildNodeSnapshotFilterParts. This suite
// proves, against live data, that the split preserves behaviour:
//   - the count query's total equals an INDEPENDENT SELECT COUNT(*) over the
//     same WHERE (so the two never drift, and neither drifts from hand SQL),
//   - the rows query returns exactly that many rows (count agrees with rows),
//   - each page is the fully-ordered set sliced by LIMIT/OFFSET,
//   - the streaming export path (ListNodeSnapshotsForExport /
//     scanFilteredNodeSnapshots) returns the same set for the same filter.

// parityOrgs are the organisations seeded by seedParityNodes. Filter cases scope
// to a subset of these so the suite is hermetic regardless of other cmm_test data.
var parityOrgs = []string{"parity-org-a", "parity-org-b", "parity-org-c"}

// parity value pools. Chosen so every value is unambiguous under any collation
// (lowercase ascii + digits) and every sort key is well-defined.
var (
	parityEnvs   = []string{"production", "staging", "development"}
	parityPlats  = [][2]string{{"ubuntu", "22.04"}, {"centos", "7.9.2009"}, {"windows", "2019"}}
	parityVers   = []string{"17.10.24", "18.2.5", "19.3.15"}
	parityStates = []string{"omnibus_only", "hab_dormant", "hab_active"}
	parityTCS    = []string{"success", "failed", ""} // "" stored as NULL → "pending"
)

func parityBoolPtr(b bool) *bool { return &b }

// seedParityNodes creates a deterministic, varied set of node snapshots across
// parityOrgs and registers cleanup. Small (fast): every filter has both matches
// and non-matches, and every sort key has orderable values. Data is read back
// from the DB — the helper returns nothing.
func seedParityNodes(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM node_snapshots  WHERE organisation_name = ANY('{parity-org-a,parity-org-b,parity-org-c}')",
		"DELETE FROM collection_runs WHERE organisation_name = ANY('{parity-org-a,parity-org-b,parity-org-c}')",
		"DELETE FROM organisations   WHERE name              = ANY('{parity-org-a,parity-org-b,parity-org-c}')",
	)

	// One org + collection run per parity org, keyed for the node FK.
	runOrg := make(map[string]string, len(parityOrgs))
	for _, org := range parityOrgs {
		o, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
			Name: org, ChefServerURL: "https://example.com/organizations/" + org, OrgName: org, ClientName: "c",
		})
		if err != nil {
			t.Fatalf("org %s: %v", org, err)
		}
		run, err := db.CreateCollectionRun(ctx, CreateCollectionRunParams{OrganisationName: o.Name})
		if err != nil {
			t.Fatalf("run %s: %v", org, err)
		}
		runOrg[org] = run.OrganisationName
	}

	const n = 54 // 18 per org
	const ohaiBase = 1_600_000_000.0
	var nodes []InsertNodeSnapshotParams
	for i := 0; i < n; i++ {
		org := parityOrgs[i%3]
		plat := parityPlats[(i/3)%3]

		var tags []string
		switch i % 4 {
		case 0:
			tags = []string{"web"}
		case 1:
			tags = []string{"db"}
		case 2:
			tags = []string{"web", "db"}
		} // i%4==3 → no tags

		nodes = append(nodes, InsertNodeSnapshotParams{
			CollectionRunOrg:     runOrg[org],
			OrganisationName:     org,
			NodeName:             fmt.Sprintf("parity-node-%03d", i),
			ChefEnvironment:      parityEnvs[i%3],
			ChefVersion:          parityVers[(i/9)%3],
			Platform:             plat[0],
			PlatformVersion:      plat[1],
			PlatformFamily:       "family",
			Tags:                 tags,
			OhaiTime:             ohaiBase + float64(i), // distinct → ohai_time sort is total
			IsStale:              i%2 == 0,
			MigrationState:       parityStates[i%3],
			TargetConvergeStatus: parityTCS[i%3],
		})
	}
	if _, err := db.BulkUpsertNodeSnapshots(ctx, nodes); err != nil {
		t.Fatalf("seeding parity nodes: %v", err)
	}
}

// filterCase pairs a production filter with an INDEPENDENT raw SQL predicate
// (hand-written, not via buildNodeSnapshotFilterParts) that must count the same
// rows. rawWhere references $2.. ; $1 is always the org scope (filter.OrganisationNames).
type filterCase struct {
	name     string
	filter   NodeSnapshotFilter
	rawWhere string
	rawArgs  []interface{}
}

// rawCount runs the independent oracle: SELECT COUNT(*) FROM node_snapshots with
// the case's hand-written WHERE, scoped to the same orgs the filter uses.
func (fc filterCase) rawCount(t *testing.T, ctx context.Context, db *DB) int {
	t.Helper()
	orgs := fc.filter.OrganisationNames
	if len(orgs) == 0 {
		orgs = parityOrgs
	}
	q := "SELECT COUNT(*) FROM node_snapshots ns WHERE ns.organisation_name = ANY($1)"
	if fc.rawWhere != "" {
		q += " AND " + fc.rawWhere
	}
	args := append([]interface{}{pq.Array(orgs)}, fc.rawArgs...)
	var n int
	if err := db.pool.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		t.Fatalf("raw count [%s]: %v", fc.name, err)
	}
	return n
}

func nodeKeys(ns []NodeSnapshot) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.OrganisationName + "/" + n.NodeName
	}
	return out
}

func parityMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestFunctional_NodeListCountSplit_Parity(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	seedParityNodes(t, db)

	base := func() NodeSnapshotFilter {
		return NodeSnapshotFilter{OrganisationNames: parityOrgs}
	}

	cases := []filterCase{
		{name: "no-filter", filter: base()},
		{name: "org-subset-a", filter: NodeSnapshotFilter{OrganisationNames: []string{"parity-org-a"}}},
		{
			name:   "node-name-substring",
			filter: NodeSnapshotFilter{OrganisationNames: parityOrgs, NodeName: "node-01"},
			// builder: LOWER(node_name) LIKE '%'||LOWER($n)||'%'
			rawWhere: "LOWER(ns.node_name) LIKE '%' || LOWER($2) || '%'", rawArgs: []interface{}{"node-01"},
		},
		{
			name:     "environment-substring",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, Environment: "prod"},
			rawWhere: "LOWER(ns.chef_environment) LIKE '%' || LOWER($2) || '%'", rawArgs: []interface{}{"prod"},
		},
		{
			name:     "environments-exact",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, Environments: []string{"production", "staging"}},
			rawWhere: "ns.chef_environment = ANY($2)", rawArgs: []interface{}{pq.Array([]string{"production", "staging"})},
		},
		{
			name:     "platform-substring",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, Platform: "ubuntu"},
			rawWhere: "LOWER(ns.platform || ' ' || COALESCE(ns.platform_version,'')) LIKE '%' || LOWER($2) || '%'", rawArgs: []interface{}{"ubuntu"},
		},
		{
			name:     "platforms-exact",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, Platforms: []string{"ubuntu 22.04"}},
			rawWhere: "(ns.platform || ' ' || COALESCE(ns.platform_version,'')) = ANY($2)", rawArgs: []interface{}{pq.Array([]string{"ubuntu 22.04"})},
		},
		{
			name:   "chef-version-prefix",
			filter: NodeSnapshotFilter{OrganisationNames: parityOrgs, ChefVersion: "18"},
			// builder: chef_version substring filter is a PREFIX LIKE, not a wildcard both sides
			rawWhere: "LOWER(ns.chef_version) LIKE LOWER($2) || '%'", rawArgs: []interface{}{"18"},
		},
		{
			name:     "chef-versions-exact",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, ChefVersions: []string{"19.3.15"}},
			rawWhere: "ns.chef_version = ANY($2)", rawArgs: []interface{}{pq.Array([]string{"19.3.15"})},
		},
		{
			name:     "chef-version-exact",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, ChefVersionExact: "17.10.24"},
			rawWhere: "ns.chef_version = $2", rawArgs: []interface{}{"17.10.24"},
		},
		{
			name:     "stale-true",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, Stale: parityBoolPtr(true)},
			rawWhere: "ns.is_stale = $2", rawArgs: []interface{}{true},
		},
		{
			name:     "stale-false",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, Stale: parityBoolPtr(false)},
			rawWhere: "ns.is_stale = $2", rawArgs: []interface{}{false},
		},
		{
			name:     "tags-overlap",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, Tags: []string{"web"}},
			rawWhere: "ns.tags && $2", rawArgs: []interface{}{pq.Array([]string{"web"})},
		},
		{
			name:     "migration-states",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, MigrationStates: []string{"hab_active"}},
			rawWhere: "ns.migration_state = ANY($2)", rawArgs: []interface{}{pq.Array([]string{"hab_active"})},
		},
		{
			name:     "target-converge-success",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, TargetConvergeStatuses: []string{"success"}},
			rawWhere: "ns.target_converge_status = ANY($2)", rawArgs: []interface{}{pq.Array([]string{"success"})},
		},
		{
			name:     "target-converge-pending",
			filter:   NodeSnapshotFilter{OrganisationNames: parityOrgs, TargetConvergeStatuses: []string{"pending"}},
			rawWhere: "(ns.target_converge_status IS NULL OR ns.target_converge_status = '')",
		},
		{
			name: "combined-org-env-stale",
			filter: NodeSnapshotFilter{
				OrganisationNames: []string{"parity-org-a", "parity-org-b"},
				Environments:      []string{"production"}, Stale: parityBoolPtr(true),
			},
			rawWhere: "ns.chef_environment = ANY($2) AND ns.is_stale = $3",
			rawArgs:  []interface{}{pq.Array([]string{"production"}), true},
		},
		{
			name:   "empty-result",
			filter: NodeSnapshotFilter{OrganisationNames: parityOrgs, NodeName: "zzz-no-such-node"},
			// no rawWhere override needed — raw oracle over the same predicate:
			rawWhere: "LOWER(ns.node_name) LIKE '%' || LOWER($2) || '%'", rawArgs: []interface{}{"zzz-no-such-node"},
		},
	}

	for _, fc := range cases {
		fc := fc
		t.Run("filter/"+fc.name, func(t *testing.T) {
			wantN := fc.rawCount(t, ctx, db)

			// Canonical full set (rows query, unpaginated, default sort).
			f := fc.filter
			f.Limit, f.Offset = 100000, 0
			full, total, err := db.ListNodeSnapshotsFiltered(ctx, f)
			if err != nil {
				t.Fatalf("list full: %v", err)
			}

			// Count query == independent SQL count.
			if total != wantN {
				t.Errorf("total: split count=%d, independent SQL=%d", total, wantN)
			}
			// Rows query returns exactly that many rows (count agrees with rows).
			if len(full) != wantN {
				t.Errorf("rows: got %d, independent SQL=%d", len(full), wantN)
			}
			// The standalone count entrypoint agrees too.
			cntOnly, err := db.CountNodeSnapshotsFiltered(ctx, fc.filter)
			if err != nil {
				t.Fatalf("count only: %v", err)
			}
			if cntOnly != wantN {
				t.Errorf("CountNodeSnapshotsFiltered=%d, independent SQL=%d", cntOnly, wantN)
			}

			assertPagination(t, ctx, db, fc.filter, "", "", nodeKeys(full), wantN)
		})
	}

	// Sort matrix: every sort key × direction over the full org-scoped set.
	sortKeys := []string{"node_name", "chef_environment", "chef_version", "platform", "ohai_time", "migration_state"}
	for _, key := range sortKeys {
		for _, dir := range []string{"asc", "desc"} {
			key, dir := key, dir
			t.Run("sort/"+key+"-"+dir, func(t *testing.T) {
				f := base()
				f.Sort, f.SortOrder = key, dir
				f.Limit, f.Offset = 100000, 0
				full, total, err := db.ListNodeSnapshotsFiltered(ctx, f)
				if err != nil {
					t.Fatalf("list full sorted: %v", err)
				}
				assertSortedBy(t, full, key, dir)
				fp := base()
				fp.Sort, fp.SortOrder = key, dir
				assertPagination(t, ctx, db, fp, key, dir, nodeKeys(full), total)
			})
		}
	}

	// Export path parity: the streamed set equals the filtered set + count.
	t.Run("export-path", func(t *testing.T) {
		f := base()

		var got []NodeSnapshot
		cur := NodeSnapshotCursor{}
		const pageSize = 20
		for {
			page, err := db.ListNodeSnapshotsForExport(ctx, f, cur, pageSize)
			if err != nil {
				t.Fatalf("export page: %v", err)
			}
			got = append(got, page...)
			if len(page) < pageSize {
				break
			}
			last := page[len(page)-1]
			cur = NodeSnapshotCursor{OrganisationName: last.OrganisationName, NodeName: last.NodeName, Valid: true}
		}

		ff := base()
		ff.Limit = 100000
		full, _, err := db.ListNodeSnapshotsFiltered(ctx, ff)
		if err != nil {
			t.Fatalf("list full: %v", err)
		}
		cnt, err := db.CountNodeSnapshotsFiltered(ctx, f)
		if err != nil {
			t.Fatalf("count: %v", err)
		}

		if len(got) != cnt {
			t.Errorf("export rows=%d, count=%d", len(got), cnt)
		}
		gk, fk := nodeKeys(got), nodeKeys(full)
		sort.Strings(gk)
		sort.Strings(fk)
		if len(gk) != len(fk) {
			t.Fatalf("export set size=%d, filtered set size=%d", len(gk), len(fk))
		}
		for i := range gk {
			if gk[i] != fk[i] {
				t.Fatalf("export set differs from filtered set at %d: %q vs %q", i, gk[i], fk[i])
			}
		}
	})
}

// assertPagination checks that pages are the fully-ordered set sliced by
// LIMIT/OFFSET, and that total is stable across pages. wantKeys is the canonical
// ordered key sequence for the filter+sort; total is its length.
func assertPagination(t *testing.T, ctx context.Context, db *DB, f NodeSnapshotFilter, sortKey, sortDir string, wantKeys []string, total int) {
	t.Helper()
	f.Sort, f.SortOrder = sortKey, sortDir

	pages := []struct {
		name           string
		limit, offset  int
	}{
		{"first", 10, 0},
		{"last-partial", 10, maxInt(0, total-4)},
		{"offset-past-end", 10, total + 10},
	}
	for _, pg := range pages {
		f2 := f
		f2.Limit, f2.Offset = pg.limit, pg.offset
		rows, tot, err := db.ListNodeSnapshotsFiltered(ctx, f2)
		if err != nil {
			t.Fatalf("page %s: %v", pg.name, err)
		}
		if tot != total {
			t.Errorf("page %s: total=%d, want %d", pg.name, tot, total)
		}
		lo := parityMin(pg.offset, total)
		hi := parityMin(pg.offset+pg.limit, total)
		want := wantKeys[lo:hi]
		got := nodeKeys(rows)
		if len(got) != len(want) {
			t.Fatalf("page %s: got %d rows, want %d (slice [%d:%d])", pg.name, len(got), len(want), lo, hi)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("page %s row %d: got %q, want %q", pg.name, i, got[i], want[i])
			}
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// assertSortedBy verifies the DB applied ORDER BY <key> <dir> — the primary key
// is monotonic in the requested direction (ties allowed; the node_name tiebreak
// is not checked here).
func assertSortedBy(t *testing.T, ns []NodeSnapshot, key, dir string) {
	t.Helper()
	val := func(n NodeSnapshot) string {
		switch key {
		case "chef_environment":
			return n.ChefEnvironment
		case "chef_version":
			return n.ChefVersion
		case "platform":
			return n.Platform
		case "ohai_time":
			return fmt.Sprintf("%020.4f", n.OhaiTime)
		case "migration_state":
			return n.MigrationState
		default:
			return n.NodeName
		}
	}
	for i := 1; i < len(ns); i++ {
		prev, cur := val(ns[i-1]), val(ns[i])
		if dir == "desc" {
			if prev < cur {
				t.Errorf("sort %s desc violated at %d: %q < %q", key, i, prev, cur)
			}
		} else {
			if prev > cur {
				t.Errorf("sort %s asc violated at %d: %q > %q", key, i, prev, cur)
			}
		}
	}
}
