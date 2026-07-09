// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
)

// ProductionPlatformRow holds a single platform/version/family tuple with
// the count of distinct nodes running a given cookbook. This is the datastore
// representation — callers convert to their own domain type.
type ProductionPlatformRow struct {
	Platform        string
	PlatformVersion string
	PlatformFamily  string
	NodeCount       int
}

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

// productionPlatformsForCookbookQuery aggregates production platforms for a
// cookbook via a JSONB key-existence (`cookbooks ? $1`) scan over
// node_snapshots. Exported at package scope so the EXPLAIN diagnostics catalog
// (query_explain.go) can explain the exact production query without drift.
const productionPlatformsForCookbookQuery = `
	SELECT platform, platform_version, platform_family,
	       COUNT(DISTINCT node_name) AS node_count
	FROM node_snapshots
	WHERE cookbooks::jsonb ? $1
	GROUP BY platform, platform_version, platform_family
	ORDER BY node_count DESC, platform, platform_version
`

// GetProductionPlatformsForCookbook returns aggregated production platform
// tuples for the given cookbook name across all organisations. It queries
// node_snapshots rows whose cookbooks JSONB column contains the cookbook as
// a top-level key, then groups by platform, platform_version, and
// platform_family.
//
// Results are ordered by node_count descending, then platform and
// platform_version ascending. An empty cookbook name returns an error.
// If no matching rows exist, an empty slice (not nil) is returned.
func (db *DB) GetProductionPlatformsForCookbook(ctx context.Context, cookbookName string) ([]ProductionPlatformRow, error) {
	if cookbookName == "" {
		return nil, fmt.Errorf("datastore: cookbook_name is required")
	}

	return scanProductionPlatformRows(db.pool.QueryContext(ctx, productionPlatformsForCookbookQuery, cookbookName))
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanProductionPlatformRow(row interface{ Scan(dest ...any) error }) (ProductionPlatformRow, error) {
	var r ProductionPlatformRow
	err := row.Scan(&r.Platform, &r.PlatformVersion, &r.PlatformFamily, &r.NodeCount)
	return r, err
}

func scanProductionPlatformRows(rows *sql.Rows, err error) ([]ProductionPlatformRow, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying production platforms for cookbook: %w", err)
	}
	defer rows.Close()

	var results []ProductionPlatformRow
	for rows.Next() {
		r, err := scanProductionPlatformRow(rows)
		if err != nil {
			return nil, fmt.Errorf("datastore: scanning production platform row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating production platform rows: %w", err)
	}

	if results == nil {
		results = []ProductionPlatformRow{}
	}
	return results, nil
}
