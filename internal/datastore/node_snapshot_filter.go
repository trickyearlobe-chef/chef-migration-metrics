// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// NodeSnapshotFilter holds optional filter criteria for listing node snapshots
// with SQL WHERE clause push-down. All string filters use case-insensitive
// substring (ILIKE-equivalent) matching to maintain behavioural parity with
// export.FilterNodes.
type NodeSnapshotFilter struct {
	// OrganisationIDs restricts results to nodes belonging to these orgs.
	// Empty means all organisations (subject to collection run validation).
	OrganisationIDs []string

	// NodeName filters by case-insensitive substring match on node_name.
	NodeName string

	// Environment filters by case-insensitive substring match on chef_environment.
	Environment string

	// Platform filters by case-insensitive substring match on the combined
	// "platform platform_version" string, matching export.FilterNodes behaviour.
	Platform string

	// ChefVersion filters by case-insensitive substring match on chef_version.
	ChefVersion string

	// ChefVersionExact filters by exact match on chef_version. Takes precedence
	// over ChefVersion when both are set.
	ChefVersionExact string

	// PolicyName filters by case-insensitive substring match on policy_name.
	PolicyName string

	// PolicyGroup filters by case-insensitive substring match on policy_group.
	PolicyGroup string

	// Role filters by case-insensitive substring match against any element
	// in the roles JSONB array.
	Role string

	// Stale filters by exact boolean match on is_stale.
	// nil means no filter (return both stale and fresh nodes).
	Stale *bool

	// IncludeHeavyJSON controls whether heavy JSONB columns (filesystem,
	// cookbooks, custom_attributes) are included in the result. Default false
	// returns a lightweight projection suitable for list/table views.
	IncludeHeavyJSON bool

	// Limit caps the number of returned rows. 0 means use a sensible default.
	Limit int

	// Offset is the number of rows to skip (for pagination).
	Offset int

	// Sort specifies the column to sort by. Valid values: "node_name",
	// "chef_environment", "chef_version", "platform", "ohai_time".
	// Empty string defaults to "node_name".
	Sort string

	// SortOrder specifies the sort direction: "asc" or "desc".
	// Empty string defaults to "asc".
	SortOrder string
}

// buildNodeSnapshotFilterQuery constructs the SQL query and args for
// ListNodeSnapshotsFiltered. It is extracted as a standalone function
// to enable unit testing of the WHERE clause builder without a database.
//
// It returns:
//   - selectQuery: the data query (with COUNT(*) OVER() for total count)
//   - args: the positional parameter values
func buildNodeSnapshotFilterQuery(f NodeSnapshotFilter) (selectQuery string, args []interface{}) {
	// Determine column list based on whether heavy JSON is requested.
	lightCols := `cn.id, cn.collection_run_id, cn.organisation_id, cn.node_name,
		       cn.chef_environment, cn.chef_version,
		       cn.platform, cn.platform_version, cn.platform_family,
		       cn.run_list, cn.roles,
		       cn.policy_name, cn.policy_group,
		       cn.ohai_time, cn.is_stale, cn.collected_at, cn.created_at`

	heavyCols := `cn.id, cn.collection_run_id, cn.organisation_id, cn.node_name,
		       cn.chef_environment, cn.chef_version,
		       cn.platform, cn.platform_version, cn.platform_family,
		       cn.filesystem, cn.cookbooks, cn.run_list, cn.roles,
		       cn.policy_name, cn.policy_group,
		       cn.ohai_time, cn.custom_attributes,
		       cn.is_stale, cn.collected_at, cn.created_at`

	cols := lightCols
	if f.IncludeHeavyJSON {
		cols = heavyCols
	}

	// CTE: select nodes from the latest completed collection run per org.
	// The correlated subquery picks the max started_at per organisation,
	// so this works correctly across multiple orgs in a single query.
	cte := `WITH completed_nodes AS (
		SELECT ns.id, ns.collection_run_id, ns.organisation_id, ns.node_name,
		       ns.chef_environment, ns.chef_version,
		       ns.platform, ns.platform_version, ns.platform_family,
		       ns.filesystem, ns.cookbooks, ns.run_list, ns.roles,
		       ns.policy_name, ns.policy_group,
		       ns.ohai_time, ns.custom_attributes,
		       ns.is_stale, ns.collected_at, ns.created_at
		  FROM node_snapshots ns
		 INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
		 WHERE cr.status = 'completed'
		   AND cr.started_at = (
		         SELECT MAX(cr2.started_at)
		           FROM collection_runs cr2
		          WHERE cr2.organisation_id = ns.organisation_id
		            AND cr2.status = 'completed'
		       )
	)`

	// Dynamic WHERE clause builder following the ListLogEntries pattern.
	where := " WHERE 1=1"
	args = []interface{}{}
	argN := 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	// Organisation filter.
	if len(f.OrganisationIDs) > 0 {
		where += " AND cn.organisation_id = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.OrganisationIDs))
	}

	// Node name filter (case-insensitive substring).
	if f.NodeName != "" {
		where += " AND LOWER(cn.node_name) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.NodeName)
	}

	// Environment filter (case-insensitive substring).
	if f.Environment != "" {
		where += " AND LOWER(cn.chef_environment) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.Environment)
	}

	// Platform filter — special case: "unknown" matches nodes with NULL or
	// empty platform, mirroring the CASE WHEN cn.platform IS NULL OR
	// cn.platform = '' THEN 'unknown' used in the dashboard platform
	// distribution aggregation.
	if strings.EqualFold(f.Platform, "unknown") {
		where += " AND (cn.platform IS NULL OR cn.platform = '')"
	} else if f.Platform != "" {
		where += " AND LOWER(cn.platform || ' ' || COALESCE(cn.platform_version, '')) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.Platform)
	}

	// Chef version filter — exact match takes precedence over substring.
	// Special case: "unknown" matches nodes with NULL or empty chef_version,
	// mirroring the COALESCE(NULLIF(chef_version, ''), 'unknown') used in
	// the dashboard version distribution aggregation.
	if f.ChefVersionExact != "" {
		where += " AND cn.chef_version = " + nextArg()
		args = append(args, f.ChefVersionExact)
	} else if strings.EqualFold(f.ChefVersion, "unknown") {
		where += " AND (cn.chef_version IS NULL OR cn.chef_version = '')"
	} else if f.ChefVersion != "" {
		where += " AND LOWER(cn.chef_version) LIKE LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.ChefVersion)
	}

	// Policy name filter (case-insensitive substring).
	if f.PolicyName != "" {
		where += " AND LOWER(cn.policy_name) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.PolicyName)
	}

	// Policy group filter (case-insensitive substring).
	if f.PolicyGroup != "" {
		where += " AND LOWER(cn.policy_group) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.PolicyGroup)
	}

	// Role filter (case-insensitive substring match on any role in the JSONB array).
	// Guard against JSONB null values (jsonb_typeof = 'null') which cause
	// jsonb_array_elements_text to error with "cannot extract elements from a scalar".
	if f.Role != "" {
		where += " AND jsonb_typeof(cn.roles) = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(cn.roles) r WHERE LOWER(r) LIKE '%' || LOWER(" + nextArg() + ") || '%')"
		args = append(args, f.Role)
	}

	// Stale filter (exact boolean match).
	if f.Stale != nil {
		where += " AND cn.is_stale = " + nextArg()
		args = append(args, *f.Stale)
	}

	// Build the full query with COUNT(*) OVER() for total count.
	var sb strings.Builder
	sb.WriteString(cte)
	sb.WriteString("\nSELECT ")
	sb.WriteString(cols)
	sb.WriteString(", COUNT(*) OVER () AS total_count\n  FROM completed_nodes cn")
	sb.WriteString(where)
	// Dynamic sort column with whitelist validation.
	sortCol := "cn.node_name"
	switch f.Sort {
	case "node_name":
		sortCol = "cn.node_name"
	case "chef_environment":
		sortCol = "cn.chef_environment"
	case "chef_version":
		sortCol = "cn.chef_version"
	case "platform":
		sortCol = "cn.platform"
	case "ohai_time":
		sortCol = "cn.ohai_time"
	}
	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}
	sb.WriteString("\n ORDER BY " + sortCol + " " + sortDir)

	// Pagination.
	if f.Limit > 0 {
		sb.WriteString("\n LIMIT " + nextArg())
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		sb.WriteString(" OFFSET " + nextArg())
		args = append(args, f.Offset)
	}

	return sb.String(), args
}

// ListNodeSnapshotsFiltered retrieves node snapshots matching the given
// filter, ordered by node_name ascending. It returns:
//   - the page of matching snapshots,
//   - the total count of all matching rows (for pagination metadata),
//   - any error encountered.
//
// Only nodes from completed collection runs are returned.
// When IncludeHeavyJSON is false, the filesystem, cookbooks, and
// custom_attributes fields will be nil in the returned snapshots.
func (db *DB) ListNodeSnapshotsFiltered(ctx context.Context, f NodeSnapshotFilter) ([]NodeSnapshot, int, error) {
	query, args := buildNodeSnapshotFilterQuery(f)
	return db.scanFilteredNodeSnapshots(ctx, query, args, f.IncludeHeavyJSON)
}

// scanFilteredNodeSnapshots executes the given query and scans results into
// NodeSnapshot structs. It handles both the lightweight and full projection
// based on the includeHeavy flag. The query must include a trailing
// total_count column from COUNT(*) OVER().
func (db *DB) scanFilteredNodeSnapshots(ctx context.Context, query string, args []interface{}, includeHeavy bool) ([]NodeSnapshot, int, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: querying filtered node snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []NodeSnapshot
	totalCount := 0

	for rows.Next() {
		var ns NodeSnapshot
		var chefEnv, chefVer, platform, platformVer, platformFam sql.NullString
		var policyName, policyGroup sql.NullString
		var ohaiTime sql.NullFloat64
		var runList, roles []byte
		var rowTotal int

		if includeHeavy {
			var filesystem, cookbooks, customAttributes []byte
			if err := rows.Scan(
				&ns.ID,
				&ns.CollectionRunID,
				&ns.OrganisationID,
				&ns.NodeName,
				&chefEnv,
				&chefVer,
				&platform,
				&platformVer,
				&platformFam,
				&filesystem,
				&cookbooks,
				&runList,
				&roles,
				&policyName,
				&policyGroup,
				&ohaiTime,
				&customAttributes,
				&ns.IsStale,
				&ns.CollectedAt,
				&ns.CreatedAt,
				&rowTotal,
			); err != nil {
				return nil, 0, fmt.Errorf("datastore: scanning filtered node snapshot row (heavy): %w", err)
			}
			ns.Filesystem = jsonFromNullBytes(filesystem)
			ns.Cookbooks = jsonFromNullBytes(cookbooks)
			ns.CustomAttributes = jsonFromNullBytes(customAttributes)
		} else {
			if err := rows.Scan(
				&ns.ID,
				&ns.CollectionRunID,
				&ns.OrganisationID,
				&ns.NodeName,
				&chefEnv,
				&chefVer,
				&platform,
				&platformVer,
				&platformFam,
				&runList,
				&roles,
				&policyName,
				&policyGroup,
				&ohaiTime,
				&ns.IsStale,
				&ns.CollectedAt,
				&ns.CreatedAt,
				&rowTotal,
			); err != nil {
				return nil, 0, fmt.Errorf("datastore: scanning filtered node snapshot row (light): %w", err)
			}
		}

		ns.ChefEnvironment = stringFromNull(chefEnv)
		ns.ChefVersion = stringFromNull(chefVer)
		ns.Platform = stringFromNull(platform)
		ns.PlatformVersion = stringFromNull(platformVer)
		ns.PlatformFamily = stringFromNull(platformFam)
		ns.PolicyName = stringFromNull(policyName)
		ns.PolicyGroup = stringFromNull(policyGroup)
		ns.OhaiTime = floatFromNull(ohaiTime)
		ns.RunList = jsonFromNullBytes(runList)
		ns.Roles = jsonFromNullBytes(roles)

		totalCount = rowTotal
		snapshots = append(snapshots, ns)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating filtered node snapshot rows: %w", err)
	}

	return snapshots, totalCount, nil
}

// ---------------------------------------------------------------------------
// Aggregate queries with SQL filter push-down
// ---------------------------------------------------------------------------

// CountNodeVersionDistribution returns a map of chef_version → count for
// nodes matching the given filter. Unlike ListNodeSnapshotsFiltered, this
// performs the aggregation in SQL and never loads individual node rows.
func (db *DB) CountNodeVersionDistribution(ctx context.Context, f NodeSnapshotFilter) (map[string]int, int, error) {
	return db.countNodeDistribution(ctx, f,
		"COALESCE(NULLIF(cn.chef_version, ''), 'unknown')",
		"version",
	)
}

// CountNodePlatformDistribution returns a map of "platform version" → count
// for nodes matching the given filter.
func (db *DB) CountNodePlatformDistribution(ctx context.Context, f NodeSnapshotFilter) (map[string]int, int, error) {
	return db.countNodeDistribution(ctx, f,
		`CASE WHEN cn.platform IS NULL OR cn.platform = '' THEN 'unknown'
		      WHEN cn.platform_version IS NOT NULL AND cn.platform_version != '' THEN cn.platform || ' ' || cn.platform_version
		      ELSE cn.platform
		 END`,
		"label",
	)
}

// countNodeDistribution is a shared helper for distribution queries. It
// builds the same CTE and WHERE clause as ListNodeSnapshotsFiltered but
// wraps the output with GROUP BY for aggregation.
func (db *DB) countNodeDistribution(ctx context.Context, f NodeSnapshotFilter, expr, alias string) (map[string]int, int, error) {
	// Build the filter query (we only need the CTE + WHERE, not the SELECT).
	// We re-use buildNodeSnapshotFilterQuery and then wrap it.
	// However, the filter query includes SELECT columns we don't need.
	// Instead, build the CTE and WHERE clause directly.
	cte, where, args := buildNodeSnapshotFilterParts(f)

	query := fmt.Sprintf(`%s
		SELECT %s AS %s, COUNT(*) AS cnt
		  FROM completed_nodes cn
		%s
		 GROUP BY %s
		 ORDER BY cnt DESC, %s ASC
	`, cte, expr, alias, where, alias, alias)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: counting node distribution: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	total := 0
	for rows.Next() {
		var label string
		var count int
		if err := rows.Scan(&label, &count); err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning node distribution row: %w", err)
		}
		result[label] = count
		total += count
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating node distribution rows: %w", err)
	}

	return result, total, nil
}

// ---------------------------------------------------------------------------
// Distinct value queries for filter dropdowns
// ---------------------------------------------------------------------------

// ListDistinctNodeValues returns sorted distinct non-empty values for the
// given column expression from nodes matching the filter.
func (db *DB) ListDistinctNodeValues(ctx context.Context, f NodeSnapshotFilter, columnExpr string) ([]string, error) {
	cte, where, args := buildNodeSnapshotFilterParts(f)

	query := fmt.Sprintf(`%s
		SELECT DISTINCT %s AS val
		  FROM completed_nodes cn
		%s
		   AND %s IS NOT NULL AND %s != ''
		 ORDER BY val
	`, cte, columnExpr, where, columnExpr, columnExpr)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing distinct node values: %w", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("datastore: scanning distinct node value: %w", err)
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

// ListDistinctNodeRoles returns sorted distinct non-empty role names from
// the roles JSONB array across all nodes matching the filter.
func (db *DB) ListDistinctNodeRoles(ctx context.Context, f NodeSnapshotFilter) ([]string, error) {
	cte, where, args := buildNodeSnapshotFilterParts(f)

	query := fmt.Sprintf(`%s
		SELECT DISTINCT r.value AS val
		  FROM completed_nodes cn,
		       jsonb_array_elements_text(cn.roles) r(value)
		%s
		   AND jsonb_typeof(cn.roles) = 'array'
		   AND r.value IS NOT NULL AND r.value != ''
		 ORDER BY val
	`, cte, where)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing distinct node roles: %w", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("datastore: scanning distinct node role: %w", err)
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

// ---------------------------------------------------------------------------
// Internal query builder helpers
// ---------------------------------------------------------------------------

// buildNodeSnapshotFilterParts returns the CTE, WHERE clause, and args
// separately so callers can compose custom SELECT/GROUP BY queries around
// the same filtering logic. The WHERE clause always starts with " WHERE 1=1"
// so additional conditions can be appended with AND.
func buildNodeSnapshotFilterParts(f NodeSnapshotFilter) (cte string, where string, args []interface{}) {
	cte = `WITH completed_nodes AS (
		SELECT ns.id, ns.collection_run_id, ns.organisation_id, ns.node_name,
		       ns.chef_environment, ns.chef_version,
		       ns.platform, ns.platform_version, ns.platform_family,
		       ns.filesystem, ns.cookbooks, ns.run_list, ns.roles,
		       ns.policy_name, ns.policy_group,
		       ns.ohai_time, ns.custom_attributes,
		       ns.is_stale, ns.collected_at, ns.created_at
		  FROM node_snapshots ns
		 INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
		 WHERE cr.status = 'completed'
		   AND cr.started_at = (
		         SELECT MAX(cr2.started_at)
		           FROM collection_runs cr2
		          WHERE cr2.organisation_id = ns.organisation_id
		            AND cr2.status = 'completed'
		       )
	)`

	where = " WHERE 1=1"
	args = []interface{}{}
	argN := 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	if len(f.OrganisationIDs) > 0 {
		where += " AND cn.organisation_id = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.OrganisationIDs))
	}

	if f.NodeName != "" {
		where += " AND LOWER(cn.node_name) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.NodeName)
	}

	if f.Environment != "" {
		where += " AND LOWER(cn.chef_environment) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.Environment)
	}

	if strings.EqualFold(f.Platform, "unknown") {
		where += " AND (cn.platform IS NULL OR cn.platform = '')"
	} else if f.Platform != "" {
		where += " AND LOWER(cn.platform || ' ' || COALESCE(cn.platform_version, '')) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.Platform)
	}

	if f.ChefVersionExact != "" {
		where += " AND cn.chef_version = " + nextArg()
		args = append(args, f.ChefVersionExact)
	} else if strings.EqualFold(f.ChefVersion, "unknown") {
		where += " AND (cn.chef_version IS NULL OR cn.chef_version = '')"
	} else if f.ChefVersion != "" {
		where += " AND LOWER(cn.chef_version) LIKE LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.ChefVersion)
	}

	if f.PolicyName != "" {
		where += " AND LOWER(cn.policy_name) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.PolicyName)
	}

	if f.PolicyGroup != "" {
		where += " AND LOWER(cn.policy_group) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.PolicyGroup)
	}

	if f.Role != "" {
		where += " AND jsonb_typeof(cn.roles) = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(cn.roles) r WHERE LOWER(r) LIKE '%' || LOWER(" + nextArg() + ") || '%')"
		args = append(args, f.Role)
	}

	if f.Stale != nil {
		where += " AND cn.is_stale = " + nextArg()
		args = append(args, *f.Stale)
	}

	return cte, where, args
}
