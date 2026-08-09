// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ConvergeRunFilter holds optional filter criteria for the "Run events"
// top-level view over converge_runs. Every field is view-level and sourced from
// converge_runs itself — the organisation filter here is the delivered org NAME,
// NOT the global organisations-table org filter (which never contains
// ingest-only DMZ orgs; see specifications/run-history.md).
//
// The same filter drives two grains: the node rollup (ListConvergeRunNodesFiltered,
// the top-level list) and the flat run list (ListConvergeRunsFiltered, the
// foundation for a future run explorer).
type ConvergeRunFilter struct {
	// Organisation filters by exact match on the delivered org name.
	Organisation string

	// Status filters by exact match; supports comma-separated multi-select.
	Status string

	// NodeName filters by case-insensitive substring match.
	NodeName string

	// ChefVersion filters by exact match on the run's chef_version — the
	// discriminator between an active-client run and a speculative
	// target-version run (only the dormant client uses the target version).
	ChefVersion string

	// Cookbook filters to runs whose observed cookbooks include this name
	// (JSONB key existence on the cookbooks map).
	Cookbook string

	// FailureMessage filters by case-insensitive substring of the failure
	// reason line (error.message, without the backtrace) — e.g. "not enough
	// space" to isolate authored prereq aborts.
	FailureMessage string

	// EndTimeFrom / EndTimeTo bound end_time (inclusive). Zero means unbounded.
	EndTimeFrom time.Time
	EndTimeTo   time.Time

	// Sort selects the sort column. Valid: "end_time" (default), "start_time",
	// "node_name", "organisation", "status", "chef_version".
	Sort string

	// SortOrder is "asc" or "desc". Empty defaults to "desc" (recency-first).
	SortOrder string

	// Limit caps returned rows. 0 means no limit (used by the export path).
	Limit int

	// Offset skips rows (pagination).
	Offset int
}

// convergeRunColumns is the shared SELECT column list (kept in sync with the
// scan in scanConvergeRunListItem).
const convergeRunColumns = `run_id, organisation, node_name, source_fqdn, chef_server_fqdn, ` +
	`status, chef_version, start_time, end_time, run_list, cookbooks, ` +
	`total_resource_count, updated_resource_count, error, failed_resource, shape`

// ConvergeRunListItem is a persisted converge run shaped for the Run events
// view. It carries organisation + node_name (the list spans nodes and orgs) and
// passes Error / FailedResource through as the stored JSONB verbatim. In the
// node rollup one item is a node's latest matching run.
type ConvergeRunListItem struct {
	RunID                string            `json:"run_id"`
	Organisation         string            `json:"organisation"`
	NodeName             string            `json:"node_name"`
	SourceFQDN           string            `json:"source_fqdn,omitempty"`
	ChefServerFQDN       string            `json:"chef_server_fqdn,omitempty"`
	Status               string            `json:"status"`
	ChefVersion          string            `json:"chef_version,omitempty"`
	StartTime            time.Time         `json:"start_time"`
	EndTime              time.Time         `json:"end_time"`
	RunList              []string          `json:"run_list"`
	Cookbooks            map[string]string `json:"cookbooks"`
	TotalResourceCount   *int              `json:"total_resource_count,omitempty"`
	UpdatedResourceCount *int              `json:"updated_resource_count,omitempty"`
	Error                json.RawMessage   `json:"error,omitempty"`
	FailedResource       json.RawMessage   `json:"failed_resource,omitempty"`
	Shape                string            `json:"shape"`
}

// convergeRunFilterWheres builds the shared WHERE fragments + args used by every
// converge_runs query. nextArg returns the next positional placeholder. The
// fragments are identical across grains so the node rollup, the flat run list,
// and the fallback count all filter identically.
func convergeRunFilterWheres(f ConvergeRunFilter, nextArg func() string) (wheres []string, args []interface{}) {
	if f.Organisation != "" {
		wheres = append(wheres, "organisation = "+nextArg())
		args = append(args, f.Organisation)
	}
	if f.Status != "" {
		wheres = append(wheres, "status = ANY("+nextArg()+")")
		args = append(args, pq.Array(strings.Split(f.Status, ",")))
	}
	if f.NodeName != "" {
		wheres = append(wheres, "LOWER(node_name) LIKE LOWER("+nextArg()+")")
		args = append(args, "%"+f.NodeName+"%")
	}
	if f.ChefVersion != "" {
		wheres = append(wheres, "chef_version = "+nextArg())
		args = append(args, f.ChefVersion)
	}
	if f.Cookbook != "" {
		// JSONB key existence on the cookbooks map (name -> version).
		wheres = append(wheres, "jsonb_exists(cookbooks, "+nextArg()+")")
		args = append(args, f.Cookbook)
	}
	if f.FailureMessage != "" {
		// Substring match on the failure reason line (backtrace excluded).
		wheres = append(wheres, "error->>'message' ILIKE "+nextArg())
		args = append(args, "%"+f.FailureMessage+"%")
	}
	if !f.EndTimeFrom.IsZero() {
		wheres = append(wheres, "end_time >= "+nextArg())
		args = append(args, f.EndTimeFrom)
	}
	if !f.EndTimeTo.IsZero() {
		wheres = append(wheres, "end_time <= "+nextArg())
		args = append(args, f.EndTimeTo)
	}
	return wheres, args
}

// convergeRunSort maps the filter's sort key (whitelist — never interpolated)
// and order to a column + direction. Runs are recency-first by default.
func convergeRunSort(f ConvergeRunFilter) (col, dir string) {
	col = "end_time"
	switch f.Sort {
	case "end_time", "":
		col = "end_time"
	case "start_time":
		col = "start_time"
	case "node_name":
		col = "LOWER(node_name)"
	case "organisation":
		col = "LOWER(organisation)"
	case "status":
		col = "status"
	case "chef_version":
		col = "chef_version"
	}
	dir = "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		dir = "ASC"
	}
	return col, dir
}

// buildConvergeRunNodeFilterQuery constructs the SQL for the node rollup (the
// top-level Run events list). EXISTS / "any matching run" semantics: filter the
// runs, then collapse to one row per node — the node's LATEST matching run via
// DISTINCT ON. The outer COUNT(*) OVER() is the DISTINCT-NODE count for
// pagination. Extracted for unit testing without a database.
func buildConvergeRunNodeFilterQuery(f ConvergeRunFilter) (query string, args []interface{}) {
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	wheres, wargs := convergeRunFilterWheres(f, nextArg)
	args = append(args, wargs...)

	var sb strings.Builder
	sb.WriteString("WITH matching AS (\n  SELECT DISTINCT ON (organisation, node_name) ")
	sb.WriteString(convergeRunColumns)
	sb.WriteString("\n  FROM converge_runs")
	if len(wheres) > 0 {
		sb.WriteString("\n  WHERE ")
		sb.WriteString(strings.Join(wheres, "\n    AND "))
	}
	// DISTINCT ON needs the distinct key first; end_time DESC picks the latest
	// matching run per node. Served by idx_converge_runs_org_node_time.
	sb.WriteString("\n  ORDER BY organisation, node_name, end_time DESC\n)")

	sb.WriteString("\nSELECT ")
	sb.WriteString(convergeRunColumns)
	sb.WriteString(", COUNT(*) OVER () AS total_count")
	sb.WriteString("\nFROM matching")

	col, dir := convergeRunSort(f)
	sb.WriteString("\nORDER BY " + col + " " + dir + ", node_name ASC")

	if f.Limit > 0 {
		sb.WriteString("\nLIMIT " + nextArg())
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		sb.WriteString(" OFFSET " + nextArg())
		args = append(args, f.Offset)
	}
	return sb.String(), args
}

// buildConvergeRunNodeCountQuery counts DISTINCT matching nodes — the fallback
// when a paginated page comes back empty (offset past the end).
func buildConvergeRunNodeCountQuery(f ConvergeRunFilter) (query string, args []interface{}) {
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}
	wheres, wargs := convergeRunFilterWheres(f, nextArg)
	args = append(args, wargs...)

	var sb strings.Builder
	sb.WriteString("SELECT COUNT(*) FROM (SELECT 1 FROM converge_runs")
	if len(wheres) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(wheres, " AND "))
	}
	sb.WriteString(" GROUP BY organisation, node_name) n")
	return sb.String(), args
}

// buildConvergeRunFilterQuery constructs the SQL for the FLAT run list (one row
// per run) — the foundation for a future run explorer, not the top-level view.
func buildConvergeRunFilterQuery(f ConvergeRunFilter) (query string, args []interface{}) {
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}
	wheres, wargs := convergeRunFilterWheres(f, nextArg)
	args = append(args, wargs...)

	var sb strings.Builder
	sb.WriteString("SELECT ")
	sb.WriteString(convergeRunColumns)
	sb.WriteString(", COUNT(*) OVER () AS total_count")
	sb.WriteString("\n  FROM converge_runs")
	if len(wheres) > 0 {
		sb.WriteString("\n WHERE ")
		sb.WriteString(strings.Join(wheres, "\n   AND "))
	}

	col, dir := convergeRunSort(f)
	sb.WriteString("\n ORDER BY " + col + " " + dir + ", run_id ASC")

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

// scanConvergeRunListItem scans one row (in convergeRunColumns order, optionally
// followed by a trailing total-count column).
func scanConvergeRunListItem(rows *sql.Rows, totalCount *int) (ConvergeRunListItem, error) {
	var v ConvergeRunListItem
	var sourceFQDN, chefServerFQDN, chefVersion sql.NullString
	var startTime sql.NullTime
	var runListJSON, cookbooksJSON, errorJSON, failedJSON []byte
	var total, updated sql.NullInt64

	dest := []any{
		&v.RunID, &v.Organisation, &v.NodeName, &sourceFQDN, &chefServerFQDN,
		&v.Status, &chefVersion, &startTime, &v.EndTime, &runListJSON, &cookbooksJSON,
		&total, &updated, &errorJSON, &failedJSON, &v.Shape,
	}
	if totalCount != nil {
		dest = append(dest, totalCount)
	}
	if err := rows.Scan(dest...); err != nil {
		return v, err
	}

	v.SourceFQDN = sourceFQDN.String
	v.ChefServerFQDN = chefServerFQDN.String
	v.ChefVersion = chefVersion.String
	if startTime.Valid {
		v.StartTime = startTime.Time
	}
	if total.Valid {
		t := int(total.Int64)
		v.TotalResourceCount = &t
	}
	if updated.Valid {
		u := int(updated.Int64)
		v.UpdatedResourceCount = &u
	}
	if len(runListJSON) > 0 {
		_ = json.Unmarshal(runListJSON, &v.RunList)
	}
	if len(cookbooksJSON) > 0 {
		_ = json.Unmarshal(cookbooksJSON, &v.Cookbooks)
	}
	if len(errorJSON) > 0 && string(errorJSON) != "null" {
		v.Error = errorJSON
	}
	if len(failedJSON) > 0 && string(failedJSON) != "null" {
		v.FailedResource = failedJSON
	}
	return v, nil
}

// ListConvergeRunNodesFiltered is the top-level Run events list: distinct nodes
// derived from converge_runs, one row per node showing its LATEST run matching
// all active filters (EXISTS semantics). Returns the page + the total distinct
// matching-node count. Reads converge_runs directly (retention-bounded) and
// never touches node_snapshots / organisations, so ingest-only DMZ nodes appear.
func (db *DB) ListConvergeRunNodesFiltered(ctx context.Context, f ConvergeRunFilter) ([]ConvergeRunListItem, int, error) {
	query, args := buildConvergeRunNodeFilterQuery(f)

	rows, err := db.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing converge run nodes: %w", err)
	}
	defer rows.Close()

	var out []ConvergeRunListItem
	var totalCount int
	for rows.Next() {
		v, err := scanConvergeRunListItem(rows, &totalCount)
		if err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning converge run node row: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating converge run node rows: %w", err)
	}

	if len(out) == 0 && f.Offset > 0 {
		countQuery, countArgs := buildConvergeRunNodeCountQuery(f)
		if err := db.q().QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount); err != nil {
			return nil, 0, fmt.Errorf("datastore: counting converge run nodes: %w", err)
		}
	}

	return out, totalCount, nil
}

// ListConvergeRunsFiltered retrieves the FLAT run list (one row per run) matching
// the filter, paginated. Foundation for a future run explorer — the top-level
// view uses ListConvergeRunNodesFiltered.
func (db *DB) ListConvergeRunsFiltered(ctx context.Context, f ConvergeRunFilter) ([]ConvergeRunListItem, int, error) {
	query, args := buildConvergeRunFilterQuery(f)

	rows, err := db.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing filtered converge runs: %w", err)
	}
	defer rows.Close()

	var out []ConvergeRunListItem
	var totalCount int
	for rows.Next() {
		v, err := scanConvergeRunListItem(rows, &totalCount)
		if err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning filtered converge run row: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating filtered converge run rows: %w", err)
	}
	return out, totalCount, nil
}

// GetConvergeRunByID returns the single most-recent run for a run_id (the dedup
// key is (run_id, end_time), so a run_id maps to one logical run). ErrNotFound
// when absent — retention may have dropped it.
func (db *DB) GetConvergeRunByID(ctx context.Context, runID string) (ConvergeRunListItem, error) {
	q := "SELECT " + convergeRunColumns + `
FROM converge_runs
WHERE run_id = $1
ORDER BY end_time DESC
LIMIT 1`
	rows, err := db.q().QueryContext(ctx, q, runID)
	if err != nil {
		return ConvergeRunListItem{}, fmt.Errorf("datastore: getting converge run %q: %w", runID, err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return ConvergeRunListItem{}, fmt.Errorf("datastore: getting converge run %q: %w", runID, err)
		}
		return ConvergeRunListItem{}, ErrNotFound
	}
	v, err := scanConvergeRunListItem(rows, nil)
	if err != nil {
		return ConvergeRunListItem{}, fmt.Errorf("datastore: scanning converge run %q: %w", runID, err)
	}
	return v, nil
}

// ListConvergeRunOrganisations returns the distinct delivered org names present
// in converge_runs, sorted — the org filter's option source for the Run events
// view. It deliberately does NOT read the organisations table, so ingest-only
// DMZ orgs (absent from that table) remain selectable.
func (db *DB) ListConvergeRunOrganisations(ctx context.Context) ([]string, error) {
	const q = `SELECT DISTINCT organisation FROM converge_runs ORDER BY organisation`
	return db.convergeRunDistinctStrings(ctx, q, "organisations")
}

// ListConvergeRunChefVersions returns the distinct non-null chef_version values
// present in converge_runs, sorted — the chef_version filter's option source.
func (db *DB) ListConvergeRunChefVersions(ctx context.Context) ([]string, error) {
	const q = `SELECT DISTINCT chef_version FROM converge_runs WHERE chef_version IS NOT NULL AND chef_version <> '' ORDER BY chef_version`
	return db.convergeRunDistinctStrings(ctx, q, "chef versions")
}

func (db *DB) convergeRunDistinctStrings(ctx context.Context, q, label string) ([]string, error) {
	rows, err := db.q().QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing converge run %s: %w", label, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("datastore: scanning converge run %s: %w", label, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
