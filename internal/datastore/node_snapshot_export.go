// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
	"strings"
)

// NodeSnapshotCursor is a keyset pagination cursor over the stable
// (organisation_name, node_name) ordering used by streaming exports. The zero
// value (Valid == false) starts a stream from the beginning.
type NodeSnapshotCursor struct {
	OrganisationName string
	NodeName         string
	Valid            bool
}

// buildNodeSnapshotExportQuery builds a keyset-paginated query over the filter,
// ordered by the unique (organisation_name, node_name) tuple. It deliberately
// ignores f.Sort/f.SortOrder/f.Limit/f.Offset and f.IncludeHeavyJSON: streamed
// exports use a fixed unique order with an explicit page limit and never load
// heavy JSONB. It carries no total_count column — scanFilteredNodeSnapshots
// scans the light projection only (the P3 count split removed the trailing
// total, so emitting one here would over-supply the scan). A valid cursor
// restricts to rows strictly after it.
func buildNodeSnapshotExportQuery(f NodeSnapshotFilter, after NodeSnapshotCursor, limit int) (string, []interface{}) {
	f.IncludeHeavyJSON = false
	cte, join, where, args := buildNodeSnapshotFilterParts(f)

	if after.Valid {
		a1 := fmt.Sprintf("$%d", len(args)+1)
		a2 := fmt.Sprintf("$%d", len(args)+2)
		where += " AND (cn.organisation_name, cn.node_name) > (" + a1 + ", " + a2 + ")"
		args = append(args, after.OrganisationName, after.NodeName)
	}

	limitArg := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, limit)

	var sb strings.Builder
	sb.WriteString(cte)
	sb.WriteString("\nSELECT ")
	sb.WriteString(nodeSnapshotLightCols)
	sb.WriteString("\n  FROM current_nodes cn")
	sb.WriteString(join)
	sb.WriteString(where)
	sb.WriteString("\n ORDER BY cn.organisation_name ASC, cn.node_name ASC")
	sb.WriteString("\n LIMIT " + limitArg)
	return sb.String(), args
}

// ListNodeSnapshotsForExport returns up to limit node snapshots matching f,
// ordered by the unique (organisation_name, node_name) tuple and starting
// strictly after the given cursor. It uses the light projection only. Callers
// stream the full filtered set by looping: start with a zero cursor, and after
// each page advance the cursor to the last returned row until fewer than limit
// rows come back. This holds only one page in memory at a time.
func (db *DB) ListNodeSnapshotsForExport(ctx context.Context, f NodeSnapshotFilter, after NodeSnapshotCursor, limit int) ([]NodeSnapshot, error) {
	query, args := buildNodeSnapshotExportQuery(f, after, limit)
	snaps, err := db.scanFilteredNodeSnapshots(ctx, query, args, false)
	if err != nil {
		return nil, err
	}
	return snaps, nil
}

// CountNodeSnapshotsFiltered returns the number of node snapshots matching f
// without loading any rows. It reuses the shared CTE + JOIN + WHERE builder, so
// the count agrees with ListNodeSnapshotsFiltered/ListNodeSnapshotsForExport for
// the same filter. Used to decide sync vs async export dispatch.
func (db *DB) CountNodeSnapshotsFiltered(ctx context.Context, f NodeSnapshotFilter) (int, error) {
	query, args := buildNodeSnapshotCountQuery(f)
	var n int
	if err := db.pool.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("datastore: counting filtered node snapshots: %w", err)
	}
	return n, nil
}
