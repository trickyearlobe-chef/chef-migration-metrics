// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// buildNodeSnapshotExportQuery — unit tests (no DB)
// ---------------------------------------------------------------------------

func TestBuildNodeSnapshotExportQuery_FixedKeysetOrder(t *testing.T) {
	// The streamed export must order by the unique (organisation_name,
	// node_name) tuple regardless of the UI sort, so keyset paging is stable.
	q, _ := buildNodeSnapshotExportQuery(NodeSnapshotFilter{
		Sort:      "ohai_time",
		SortOrder: "desc",
	}, NodeSnapshotCursor{}, 500)

	if !strings.Contains(q, "ORDER BY cn.organisation_name ASC, cn.node_name ASC") {
		t.Errorf("export query must order by the unique (org, node) tuple; got:\n%s", q)
	}
	// UI sort column must not leak into the streamed order.
	if strings.Contains(q, "ORDER BY cn.ohai_time") {
		t.Errorf("export query must not honour the UI sort column; got:\n%s", q)
	}
}

func TestBuildNodeSnapshotExportQuery_NoWindowCount(t *testing.T) {
	// Streaming must not pay for COUNT(*) OVER() on every page.
	q, _ := buildNodeSnapshotExportQuery(NodeSnapshotFilter{}, NodeSnapshotCursor{}, 500)
	if strings.Contains(q, "COUNT(*) OVER") {
		t.Errorf("export query must not include a window count; got:\n%s", q)
	}
	if !strings.Contains(q, "0 AS total_count") {
		t.Errorf("export query must emit a constant total_count for the shared scanner; got:\n%s", q)
	}
}

func TestBuildNodeSnapshotExportQuery_LightProjectionOnly(t *testing.T) {
	// Even if a caller sets IncludeHeavyJSON, the export must stay light.
	q, _ := buildNodeSnapshotExportQuery(NodeSnapshotFilter{IncludeHeavyJSON: true}, NodeSnapshotCursor{}, 500)
	outerSelect := q[strings.Index(q, "FROM current_nodes cn"):]
	for _, heavy := range []string{"cn.filesystem", "cn.cookbooks", "cn.custom_attributes"} {
		if strings.Contains(outerSelect, heavy) {
			t.Errorf("export query must not select heavy JSONB column %q; got:\n%s", heavy, outerSelect)
		}
	}
}

func TestBuildNodeSnapshotExportQuery_FirstPageNoCursor(t *testing.T) {
	q, args := buildNodeSnapshotExportQuery(NodeSnapshotFilter{}, NodeSnapshotCursor{}, 500)
	if strings.Contains(q, "cn.organisation_name, cn.node_name) >") {
		t.Errorf("first page (no cursor) must not include a keyset predicate; got:\n%s", q)
	}
	// Only the LIMIT arg.
	if len(args) != 1 || args[0] != 500 {
		t.Errorf("first page args = %v, want [500]", args)
	}
}

func TestBuildNodeSnapshotExportQuery_KeysetCursor(t *testing.T) {
	q, args := buildNodeSnapshotExportQuery(NodeSnapshotFilter{
		OrganisationNames: []string{"org-a"},
	}, NodeSnapshotCursor{OrganisationName: "org-a", NodeName: "web-01", Valid: true}, 250)

	if !strings.Contains(q, "(cn.organisation_name, cn.node_name) > ($2, $3)") {
		t.Errorf("keyset predicate missing or mis-numbered; got:\n%s", q)
	}
	// args: $1 org filter array, $2 cursor org, $3 cursor node, $4 limit.
	if len(args) != 4 {
		t.Fatalf("args = %v, want 4 (org filter, cursor org, cursor node, limit)", args)
	}
	if args[1] != "org-a" || args[2] != "web-01" || args[3] != 250 {
		t.Errorf("args = %v, want [<orgfilter> org-a web-01 250]", args)
	}
	if !strings.Contains(q, "LIMIT $4") {
		t.Errorf("LIMIT must use $4 after the cursor args; got:\n%s", q)
	}
}
