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
	// OrganisationNames restricts results to nodes belonging to these orgs.
	// Empty means all organisations (subject to collection run validation).
	OrganisationNames []string

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
	// in the roles JSONB array. Used for freeform text search.
	Role string

	// Roles filters by exact match against any of these role names in the
	// roles JSONB array. Uses EXISTS + ANY SQL. When set, takes precedence
	// over Role (substring).
	Roles []string

	// Environments filters by exact match against any of these environments.
	// Uses ANY($N) SQL. When set, takes precedence over Environment (substring).
	Environments []string

	// Platforms filters by exact match on "platform platform_version" string.
	// Uses ANY($N) SQL. When set, takes precedence over Platform (substring).
	Platforms []string

	// ChefVersions filters by exact match against any of these versions.
	// Uses ANY($N) SQL. When set, takes precedence over ChefVersion (substring).
	ChefVersions []string

	// PolicyNames filters by exact match against any of these policy names.
	// Uses ANY($N) SQL. When set, takes precedence over PolicyName (substring).
	PolicyNames []string

	// PolicyGroups filters by exact match against any of these policy groups.
	// Uses ANY($N) SQL. When set, takes precedence over PolicyGroup (substring).
	PolicyGroups []string

	// Stale filters by exact boolean match on is_stale.
	// nil means no filter (return both stale and fresh nodes).
	// Ignored when StaleTiers is set.
	Stale *bool

	// StaleTiers filters by computed staleness tier. Values: "fresh", "warning", "critical".
	// Empty means no tier filter. When set, Stale is ignored.
	StaleTiers []string
	// StaleWarningHours and StaleCriticalDays are the tier thresholds for
	// SQL-level staleness tier computation. Only used when StaleTiers is set.
	StaleWarningHours int
	StaleCriticalDays int

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

	// ReadinessFilter filters nodes by upgrade readiness status. Requires
	// TargetChefVersion to be set. Values: "ready", "blocked",
	// "cookbooks_blocked", "disk_blocked", "disk_unknown". Empty means no
	// readiness filter. Ignored when TargetChefVersion is empty.
	ReadinessFilter string

	// CookstyleStatusFilter filters by materialised cookstyle_status on
	// node_readiness. Comma-separated values: "passed", "failed", "unknown".
	// Requires TargetChefVersion.
	CookstyleStatusFilter string

	// KitchenStatusFilter filters by materialised kitchen_status on
	// node_readiness. Comma-separated values: "passed", "failed", "partial",
	// "unknown". Requires TargetChefVersion.
	KitchenStatusFilter string

	// TargetChefVersion is the target Chef version used to JOIN with
	// node_readiness for readiness filtering and data enrichment. When set
	// (even without ReadinessFilter), a LEFT JOIN to node_readiness is
	// included so callers can access readiness columns.
	TargetChefVersion string

	// MigrationStates filters by exact match on migration_state column.
	// Values: "omnibus_only", "hab_dormant", "hab_active". Empty means no filter.
	MigrationStates []string

	// TargetConvergeStatuses filters by exact match on target_converge_status.
	// Values: "success", "failed". Empty means no filter.
	TargetConvergeStatuses []string

	// TargetVersions filters by exact match on target_version column.
	// Empty means no filter.
	TargetVersions []string

	// ReadyToActivate when true filters to nodes that are ready to activate
	// (migration_state = 'hab_dormant' AND target_converge_status = 'success').
	ReadyToActivate *bool
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
	lightCols := `cn.collection_run_org, cn.organisation_name, cn.node_name,
		       cn.chef_environment, cn.chef_version,
		       cn.platform, cn.platform_version, cn.platform_family,
		       cn.platform_caption,
		       cn.run_list, cn.roles,
		       cn.policy_name, cn.policy_group,
		       cn.ohai_time, cn.is_stale, cn.collected_at, cn.created_at,
		       cn.migration_state, cn.active_chef_version, cn.dormant_installed,
		       cn.dormant_chef_version, cn.target_version, cn.target_execution_time,
		       cn.target_converge_status,
		       cn.sufficient_disk_space, cn.available_disk_mb, cn.required_disk_mb`

	heavyCols := `cn.collection_run_org, cn.organisation_name, cn.node_name,
		       cn.chef_environment, cn.chef_version,
		       cn.platform, cn.platform_version, cn.platform_family,
		       cn.platform_caption,
		       cn.filesystem, cn.cookbooks, cn.run_list, cn.roles,
		       cn.policy_name, cn.policy_group,
		       cn.ohai_time, cn.custom_attributes,
		       cn.is_stale, cn.collected_at, cn.created_at,
		       cn.migration_state, cn.active_chef_version, cn.dormant_installed,
		       cn.dormant_chef_version, cn.target_version, cn.target_execution_time,
		       cn.target_converge_status,
		       cn.sufficient_disk_space, cn.available_disk_mb, cn.required_disk_mb`

	cols := lightCols
	if f.IncludeHeavyJSON {
		cols = heavyCols
	}

	// Reuse the shared CTE + JOIN + WHERE clause builder.
	cte, join, where, args := buildNodeSnapshotFilterParts(f)

	// Build the full query with COUNT(*) OVER() for total count.
	var sb strings.Builder
	sb.WriteString(cte)
	sb.WriteString("\nSELECT ")
	sb.WriteString(cols)
	sb.WriteString(", COUNT(*) OVER () AS total_count\n  FROM current_nodes cn")
	sb.WriteString(join)
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
	case "migration_state":
		sortCol = "cn.migration_state"
	}
	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}
	sb.WriteString("\n ORDER BY " + sortCol + " " + sortDir + ", cn.node_name ASC")

	// Pagination — argument numbering continues from where
	// buildNodeSnapshotFilterParts left off.
	argN := len(args)
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

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
		var collectionRunOrg sql.NullString
		var chefEnv, chefVer, platform, platformVer, platformFam sql.NullString
		var platformCaption sql.NullString
		var policyName, policyGroup sql.NullString
		var ohaiTime sql.NullFloat64
		var runList, roles []byte
		var rowTotal int
		var migrationState, activeChefVer, dormantChefVer sql.NullString
		var dormantInstalled sql.NullBool
		var targetVer, targetExecTime, targetConvergeStatus sql.NullString
		var sufficientDisk sql.NullBool
		var availableDiskMB, requiredDiskMB sql.NullInt64

		if includeHeavy {
			var filesystem, cookbooks, customAttributes []byte
			if err := rows.Scan(
				&collectionRunOrg,
				&ns.OrganisationName,
				&ns.NodeName,
				&chefEnv,
				&chefVer,
				&platform,
				&platformVer,
				&platformFam,
				&platformCaption,
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
				&migrationState,
				&activeChefVer,
				&dormantInstalled,
				&dormantChefVer,
				&targetVer,
				&targetExecTime,
				&targetConvergeStatus,
				&sufficientDisk,
				&availableDiskMB,
				&requiredDiskMB,
				&rowTotal,
			); err != nil {
				return nil, 0, fmt.Errorf("datastore: scanning filtered node snapshot row (heavy): %w", err)
			}
			ns.Filesystem = jsonFromNullBytes(filesystem)
			ns.Cookbooks = jsonFromNullBytes(cookbooks)
			ns.CustomAttributes = jsonFromNullBytes(customAttributes)
		} else {
			if err := rows.Scan(
				&collectionRunOrg,
				&ns.OrganisationName,
				&ns.NodeName,
				&chefEnv,
				&chefVer,
				&platform,
				&platformVer,
				&platformFam,
				&platformCaption,
				&runList,
				&roles,
				&policyName,
				&policyGroup,
				&ohaiTime,
				&ns.IsStale,
				&ns.CollectedAt,
				&ns.CreatedAt,
				&migrationState,
				&activeChefVer,
				&dormantInstalled,
				&dormantChefVer,
				&targetVer,
				&targetExecTime,
				&targetConvergeStatus,
				&sufficientDisk,
				&availableDiskMB,
				&requiredDiskMB,
				&rowTotal,
			); err != nil {
				return nil, 0, fmt.Errorf("datastore: scanning filtered node snapshot row (light): %w", err)
			}
		}

		ns.CollectionRunOrg = stringFromNull(collectionRunOrg)
		ns.ChefEnvironment = stringFromNull(chefEnv)
		ns.ChefVersion = stringFromNull(chefVer)
		ns.Platform = stringFromNull(platform)
		ns.PlatformVersion = stringFromNull(platformVer)
		ns.PlatformFamily = stringFromNull(platformFam)
		ns.PlatformCaption = stringFromNull(platformCaption)
		ns.PolicyName = stringFromNull(policyName)
		ns.PolicyGroup = stringFromNull(policyGroup)
		ns.OhaiTime = floatFromNull(ohaiTime)
		ns.RunList = jsonFromNullBytes(runList)
		ns.Roles = jsonFromNullBytes(roles)
		ns.MigrationState = stringFromNull(migrationState)
		ns.ActiveChefVersion = stringFromNull(activeChefVer)
		ns.DormantInstalled = boolFromNull(dormantInstalled)
		ns.DormantChefVersion = stringFromNull(dormantChefVer)
		ns.TargetVersion = stringFromNull(targetVer)
		ns.TargetExecutionTime = stringFromNull(targetExecTime)
		ns.TargetConvergeStatus = stringFromNull(targetConvergeStatus)
		ns.SufficientDiskSpace = boolFromNull(sufficientDisk)
		ns.AvailableDiskMB = intPtrFromNull(availableDiskMB)
		ns.RequiredDiskMB = intPtrFromNull(requiredDiskMB)

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
	cte, join, where, args := buildNodeSnapshotFilterParts(f)

	query := fmt.Sprintf(`%s
		SELECT %s AS %s, COUNT(*) AS cnt
		  FROM current_nodes cn
		%s%s
		 GROUP BY %s
		 ORDER BY cnt DESC, %s ASC
	`, cte, expr, alias, join, where, alias, alias)

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

// PlatformDistributionRow holds one row from the detailed platform distribution query.
type PlatformDistributionRow struct {
	Platform        string
	PlatformVersion string
	PlatformFamily  string
	PlatformCaption string
	Count           int
}

// CountNodePlatformDistributionDetailed returns platform distribution with
// individual platform, version, family, and caption columns for accurate resolution.
func (db *DB) CountNodePlatformDistributionDetailed(ctx context.Context, f NodeSnapshotFilter) ([]PlatformDistributionRow, int, error) {
	cte, join, where, args := buildNodeSnapshotFilterParts(f)

	query := fmt.Sprintf(`%s
		SELECT COALESCE(NULLIF(cn.platform, ''), 'unknown') AS platform,
		       COALESCE(cn.platform_version, '') AS platform_version,
		       COALESCE(cn.platform_family, '') AS platform_family,
		       COALESCE(cn.platform_caption, '') AS platform_caption,
		       COUNT(*) AS cnt
		  FROM current_nodes cn
		%s%s
		 GROUP BY 1, 2, 3, 4
		 ORDER BY cnt DESC
	`, cte, join, where)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: counting detailed platform distribution: %w", err)
	}
	defer rows.Close()

	var result []PlatformDistributionRow
	total := 0
	for rows.Next() {
		var r PlatformDistributionRow
		if err := rows.Scan(&r.Platform, &r.PlatformVersion, &r.PlatformFamily, &r.PlatformCaption, &r.Count); err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning detailed platform distribution row: %w", err)
		}
		result = append(result, r)
		total += r.Count
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating detailed platform distribution rows: %w", err)
	}

	return result, total, nil
}

// ---------------------------------------------------------------------------
// Distinct value queries for filter dropdowns
// ---------------------------------------------------------------------------

// DistinctValueOpts holds optional search prefix and result limit for
// distinct-value filter endpoints. Zero values mean no restriction.
type DistinctValueOpts struct {
	// SearchPrefix restricts results to values starting with this prefix
	// (case-insensitive). Empty means no prefix filter.
	SearchPrefix string
	// Limit caps the number of returned values. 0 means no limit.
	Limit int
}

// ListDistinctNodeValues returns sorted distinct non-empty values for the
// given column expression from nodes matching the filter. When
// opts.SearchPrefix is set, only values starting with that prefix are
// returned. When opts.Limit > 0, results are capped.
func (db *DB) ListDistinctNodeValues(ctx context.Context, f NodeSnapshotFilter, columnExpr string, opts DistinctValueOpts) ([]string, error) {
	cte, join, where, args := buildNodeSnapshotFilterParts(f)

	argN := len(args)
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	prefixClause := ""
	if opts.SearchPrefix != "" {
		prefixClause = " AND LOWER((" + columnExpr + ")::text) LIKE LOWER(" + nextArg() + ") || '%'"
		args = append(args, opts.SearchPrefix)
	}

	limitClause := ""
	if opts.Limit > 0 {
		limitClause = " LIMIT " + nextArg()
		args = append(args, opts.Limit)
	}

	query := fmt.Sprintf(`%s
		SELECT DISTINCT %s AS val
		  FROM current_nodes cn
		%s%s
		   AND %s IS NOT NULL AND %s != ''%s
		 ORDER BY val%s
	`, cte, columnExpr, join, where, columnExpr, columnExpr, prefixClause, limitClause)

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
// the roles JSONB array across all nodes matching the filter. When
// opts.SearchPrefix is set, only roles starting with that prefix are
// returned. When opts.Limit > 0, results are capped.
func (db *DB) ListDistinctNodeRoles(ctx context.Context, f NodeSnapshotFilter, opts DistinctValueOpts) ([]string, error) {
	cte, join, where, args := buildNodeSnapshotFilterParts(f)

	argN := len(args)
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	prefixClause := ""
	if opts.SearchPrefix != "" {
		prefixClause = " AND LOWER(r.value) LIKE LOWER(" + nextArg() + ") || '%'"
		args = append(args, opts.SearchPrefix)
	}

	limitClause := ""
	if opts.Limit > 0 {
		limitClause = " LIMIT " + nextArg()
		args = append(args, opts.Limit)
	}

	query := fmt.Sprintf(`%s
		SELECT DISTINCT r.value AS val
		  FROM current_nodes cn
		%s, jsonb_array_elements_text(cn.roles) r(value)
		%s
		   AND jsonb_typeof(cn.roles) = 'array'
		   AND r.value IS NOT NULL AND r.value != ''%s
		 ORDER BY val%s
	`, cte, join, where, prefixClause, limitClause)

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
func buildNodeSnapshotFilterParts(f NodeSnapshotFilter) (cte string, join string, where string, args []interface{}) {
	// Do not gate on collection_runs.status. Node snapshots are upserted in
	// place and are valid once written, even if the collection run later fails.
	// Orphaned nodes are cleaned up by DeleteOrphanedNodeSnapshots which has
	// its own safety guard against empty active-node lists.
	cte = `WITH current_nodes AS (
		SELECT ns.collection_run_org, ns.organisation_name, ns.node_name,
		       ns.chef_environment, ns.chef_version,
		       ns.platform, ns.platform_version, ns.platform_family,
		       ns.platform_caption,
		       ns.filesystem, ns.cookbooks, ns.run_list, ns.roles,
		       ns.policy_name, ns.policy_group,
		       ns.ohai_time, ns.custom_attributes,
		       ns.is_stale, ns.collected_at, ns.created_at,
		       ns.migration_state, ns.active_chef_version, ns.dormant_installed,
		       ns.dormant_chef_version, ns.target_version, ns.target_execution_time,
		       ns.target_converge_status,
		       ns.sufficient_disk_space, ns.available_disk_mb, ns.required_disk_mb
		  FROM node_snapshots ns
	)`

	where = " WHERE 1=1"
	args = []interface{}{}
	argN := 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	if len(f.OrganisationNames) > 0 {
		where += " AND cn.organisation_name = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.OrganisationNames))
	}

	if f.NodeName != "" {
		where += " AND LOWER(cn.node_name) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.NodeName)
	}

	if len(f.Environments) > 0 {
		where += " AND cn.chef_environment = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.Environments))
	} else if f.Environment != "" {
		where += " AND LOWER(cn.chef_environment) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.Environment)
	}

	if len(f.Platforms) > 0 {
		where += " AND (cn.platform || ' ' || COALESCE(cn.platform_version, '')) = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.Platforms))
	} else if strings.EqualFold(f.Platform, "unknown") {
		where += " AND (cn.platform IS NULL OR cn.platform = '')"
	} else if f.Platform != "" {
		where += " AND LOWER(cn.platform || ' ' || COALESCE(cn.platform_version, '')) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.Platform)
	}

	if len(f.ChefVersions) > 0 {
		where += " AND cn.chef_version = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.ChefVersions))
	} else if f.ChefVersionExact != "" {
		where += " AND cn.chef_version = " + nextArg()
		args = append(args, f.ChefVersionExact)
	} else if strings.EqualFold(f.ChefVersion, "unknown") {
		where += " AND (cn.chef_version IS NULL OR cn.chef_version = '')"
	} else if f.ChefVersion != "" {
		where += " AND LOWER(cn.chef_version) LIKE LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.ChefVersion)
	}

	if len(f.PolicyNames) > 0 {
		where += " AND cn.policy_name = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.PolicyNames))
	} else if f.PolicyName != "" {
		where += " AND LOWER(cn.policy_name) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.PolicyName)
	}

	if len(f.PolicyGroups) > 0 {
		where += " AND cn.policy_group = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.PolicyGroups))
	} else if f.PolicyGroup != "" {
		where += " AND LOWER(cn.policy_group) LIKE '%' || LOWER(" + nextArg() + ") || '%'"
		args = append(args, f.PolicyGroup)
	}

	if len(f.Roles) > 0 {
		where += " AND jsonb_typeof(cn.roles) = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(cn.roles) r WHERE r = ANY(" + nextArg() + "))"
		args = append(args, pq.Array(f.Roles))
	} else if f.Role != "" {
		where += " AND jsonb_typeof(cn.roles) = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(cn.roles) r WHERE LOWER(r) LIKE '%' || LOWER(" + nextArg() + ") || '%')"
		args = append(args, f.Role)
	}

	if len(f.StaleTiers) > 0 {
		warningSeconds := f.StaleWarningHours * 3600
		criticalSeconds := f.StaleCriticalDays * 86400

		// Build a set for quick lookup.
		tierSet := make(map[string]bool, len(f.StaleTiers))
		for _, t := range f.StaleTiers {
			tierSet[t] = true
		}

		// Build OR conditions for each requested tier.
		var tierConds []string
		if tierSet["fresh"] {
			tierConds = append(tierConds, fmt.Sprintf("cn.ohai_time > extract(epoch from now()) - %d", warningSeconds))
		}
		if tierSet["warning"] {
			tierConds = append(tierConds, fmt.Sprintf("(cn.ohai_time <= extract(epoch from now()) - %d AND cn.ohai_time > extract(epoch from now()) - %d)", warningSeconds, criticalSeconds))
		}
		if tierSet["critical"] {
			tierConds = append(tierConds, fmt.Sprintf("(cn.ohai_time <= extract(epoch from now()) - %d OR cn.ohai_time = 0 OR cn.ohai_time IS NULL)", criticalSeconds))
		}
		// Legacy single-value "stale" = warning + critical.
		if tierSet["stale"] && !tierSet["warning"] && !tierSet["critical"] {
			tierConds = append(tierConds, fmt.Sprintf("(cn.ohai_time <= extract(epoch from now()) - %d OR cn.ohai_time = 0 OR cn.ohai_time IS NULL)", warningSeconds))
		}
		if len(tierConds) > 0 {
			where += " AND (" + strings.Join(tierConds, " OR ") + ")"
		}
	} else if f.Stale != nil {
		where += " AND cn.is_stale = " + nextArg()
		args = append(args, *f.Stale)
	}

	// Disk readiness filters are version-invariant: the verdict is computed from
	// the node's platform install size + free space and stored per node on the
	// snapshot at collection time (migration 0037), independent of any target
	// version. Resolve them directly from the node_snapshots columns so the list
	// agrees with the detail view and works even when no target version is set.
	switch f.ReadinessFilter {
	case "disk_blocked":
		where += " AND cn.sufficient_disk_space = false"
	case "disk_unknown":
		// Unknown = indeterminate verdict (NULL): missing/unparseable filesystem
		// data, a stale node, or a node not yet collected with the verdict.
		where += " AND cn.sufficient_disk_space IS NULL"
	}

	// Version-scoped readiness filters — JOIN node_readiness when
	// TargetChefVersion is set. These checks (ready/blocked/cookbooks, plus
	// cookstyle/kitchen status) genuinely depend on the target Chef version.
	if f.TargetChefVersion != "" {
		join = "\n LEFT JOIN node_readiness nr ON nr.organisation_name = cn.organisation_name AND nr.node_name = cn.node_name AND nr.target_chef_version = " + nextArg()
		args = append(args, f.TargetChefVersion)

		switch f.ReadinessFilter {
		case "ready":
			where += " AND nr.is_ready = true"
		case "needs_review":
			where += " AND nr.status = 'needs_review'"
		case "blocked":
			// Blocked excludes needs_review: a needs-review node is not ready but
			// is not blocked. Treat a missing readiness row as blocked.
			where += " AND (nr.status = 'blocked' OR nr.status IS NULL)"
		case "cookbooks_blocked":
			where += " AND (nr.all_cookbooks_compatible = false OR nr.all_cookbooks_compatible IS NULL)"
		}

		if f.CookstyleStatusFilter != "" {
			where += " AND COALESCE(nr.cookstyle_status, '') = ANY(" + nextArg() + ")"
			args = append(args, pq.Array(splitCSV(f.CookstyleStatusFilter)))
		}
		if f.KitchenStatusFilter != "" {
			where += " AND COALESCE(nr.kitchen_status, '') = ANY(" + nextArg() + ")"
			args = append(args, pq.Array(splitCSV(f.KitchenStatusFilter)))
		}
	}

	// Parallel deployment tracking filters.
	if len(f.MigrationStates) > 0 {
		where += " AND cn.migration_state = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.MigrationStates))
	}
	if len(f.TargetConvergeStatuses) > 0 {
		// "pending" is a pseudo-value meaning NULL/empty (no converge result yet).
		hasPending := false
		real := make([]string, 0, len(f.TargetConvergeStatuses))
		for _, s := range f.TargetConvergeStatuses {
			if s == "pending" {
				hasPending = true
			} else {
				real = append(real, s)
			}
		}
		switch {
		case hasPending && len(real) > 0:
			where += " AND (cn.target_converge_status = ANY(" + nextArg() + ") OR cn.target_converge_status IS NULL OR cn.target_converge_status = '')"
			args = append(args, pq.Array(real))
		case hasPending:
			where += " AND (cn.target_converge_status IS NULL OR cn.target_converge_status = '')"
		default:
			where += " AND cn.target_converge_status = ANY(" + nextArg() + ")"
			args = append(args, pq.Array(real))
		}
	}
	if len(f.TargetVersions) > 0 {
		where += " AND cn.target_version = ANY(" + nextArg() + ")"
		args = append(args, pq.Array(f.TargetVersions))
	}
	if f.ReadyToActivate != nil && *f.ReadyToActivate {
		where += " AND cn.migration_state = 'hab_dormant' AND cn.target_converge_status = 'success'"
	}

	return cte, join, where, args
}

// DeploymentVersionRow holds per-version deployment state counts from a live
// GROUP BY query on node_snapshots.
type DeploymentVersionRow struct {
	Version         string
	Staged          int
	Activated       int
	ConvergePassing int
	ConvergeFailing int
}

// CountNodesByDeploymentVersion returns per-version deployment state counts
// for nodes matching the given filter. Only nodes with migration_state
// 'hab_dormant' or 'hab_active' (and a non-empty deployed version) are
// included. Also returns totalNodes (all nodes in filter, regardless of state).
func (db *DB) CountNodesByDeploymentVersion(ctx context.Context, f NodeSnapshotFilter) ([]DeploymentVersionRow, int, error) {
	cte, join, where, args := buildNodeSnapshotFilterParts(f)

	// Total nodes (all states).
	totalQuery := fmt.Sprintf(`%s SELECT COUNT(*) FROM current_nodes cn %s%s`, cte, join, where)
	var totalNodes int
	if err := db.pool.QueryRowContext(ctx, totalQuery, args...).Scan(&totalNodes); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting total nodes for deployment version: %w", err)
	}

	// Per-version deployment breakdown.
	query := fmt.Sprintf(`%s
		SELECT
		  CASE WHEN cn.migration_state = 'hab_dormant' THEN cn.dormant_chef_version
		       WHEN cn.migration_state = 'hab_active' THEN cn.active_chef_version
		  END AS deployed_version,
		  COUNT(*) FILTER (WHERE cn.migration_state = 'hab_dormant') AS staged,
		  COUNT(*) FILTER (WHERE cn.migration_state = 'hab_active') AS activated,
		  COUNT(*) FILTER (WHERE cn.target_converge_status = 'success') AS converge_passing,
		  COUNT(*) FILTER (WHERE cn.target_converge_status = 'failed') AS converge_failing
		FROM current_nodes cn
		%s%s
		  AND cn.migration_state IN ('hab_dormant', 'hab_active')
		  AND COALESCE(
		    CASE WHEN cn.migration_state = 'hab_dormant' THEN cn.dormant_chef_version
		         WHEN cn.migration_state = 'hab_active' THEN cn.active_chef_version
		    END, '') != ''
		GROUP BY deployed_version
		ORDER BY (COUNT(*)) DESC, deployed_version ASC
	`, cte, join, where)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: counting deployment version distribution: %w", err)
	}
	defer rows.Close()

	var result []DeploymentVersionRow
	for rows.Next() {
		var r DeploymentVersionRow
		if err := rows.Scan(&r.Version, &r.Staged, &r.Activated, &r.ConvergePassing, &r.ConvergeFailing); err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning deployment version row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating deployment version rows: %w", err)
	}

	return result, totalNodes, nil
}

// splitCSV splits a comma-separated string into trimmed non-empty values.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
