// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// query_explain.go implements the admin EXPLAIN runner (performance-diagnostics
// "Layer 4"). It captures query plans for the hot queries the pg-stats dashboard
// surfaces, so "this query is slow" becomes "it seq-scans node_snapshots".
//
// Safety: every EXPLAIN runs inside a READ ONLY transaction with a per-run
// statement_timeout, then rolls back. READ ONLY is the write backstop even for
// EXPLAIN (ANALYZE …), which executes the statement — a SELECT runs but cannot
// write, and an accidental ANALYZE of a mutating statement errors out.
//
// The canned catalog reuses the live production query builders (buildRoleListQuery,
// buildNodeSnapshotFilterQuery, and the lifted SQL consts) so an explained plan is
// exactly what the app runs and cannot drift across migrations. Parameters are
// resolved from live values (all orgs, a sample node/cookbook, the default target)
// so no environment identifiers are hardcoded in source; all params are bound ($N),
// so plan text shows placeholders, never the resolved values.

// ErrExplainUnavailable is returned by ResolveCatalogExplain when a catalog entry
// cannot be resolved because the live sample data it needs is absent (e.g. empty DB).
var ErrExplainUnavailable = errors.New("datastore: explain catalog entry unavailable — no live sample data")

// ErrExplainNotExplainable is returned by ValidateFreeformExplainSQL for anything
// that is not a single explainable data statement.
var ErrExplainNotExplainable = errors.New("datastore: only a single data statement (SELECT/WITH/INSERT/UPDATE/DELETE/MERGE/TABLE/VALUES) may be explained")

// explainableLeadingKeywords are the statement kinds PostgreSQL's EXPLAIN accepts.
// Utility statements (COPY, VACUUM, SET, DDL, …) are not explainable and are
// rejected. Writes are allowed because plan-only EXPLAIN never executes them, and
// the READ ONLY transaction in RunExplain blocks any write under ANALYZE.
var explainableLeadingKeywords = []string{
	"SELECT", "WITH", "INSERT", "UPDATE", "DELETE", "MERGE", "TABLE", "VALUES",
}

const (
	defaultExplainTimeoutMs = 15000
	defaultExplainMaxBytes  = 1 << 20 // 1 MiB of plan text
	explainSampleLimit      = 50      // representative page size for list-query catalog entries
)

// Catalog entry keys.
const (
	explainRolesList     = "roles_list"
	explainNodeListHeavy = "node_list_heavy"
	explainNodeListLight = "node_list_light"
	explainCookbookCover = "cookbook_coverage_containment"
	explainNodeSingleRow = "node_single_full_row"
	explainDistinctRoles = "distinct_node_roles"
)

// ExplainOptions controls a RunExplain invocation.
type ExplainOptions struct {
	Analyze            bool // EXPLAIN (ANALYZE, BUFFERS) — executes the query
	RunTwice           bool // run a second time to show buffer-cache warmth
	StatementTimeoutMs int  // per-run cap; <=0 uses the default
	MaxPlanBytes       int  // truncate plan text beyond this; <=0 uses the default
}

// ExplainRun is a single EXPLAIN execution's plan text and wall-clock duration.
type ExplainRun struct {
	PlanText   string  `json:"plan_text"`
	DurationMs float64 `json:"duration_ms"`
	Truncated  bool    `json:"truncated"`
}

// ExplainResult holds the plan run(s) from RunExplain.
type ExplainResult struct {
	Run1 ExplainRun  `json:"run1"`
	Run2 *ExplainRun `json:"run2,omitempty"`
}

// ExplainCatalogEntry describes a canned explain the admin UI can run by key.
type ExplainCatalogEntry struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Analyzable  bool   `json:"analyzable"`
}

// CatalogParams carries values the catalog resolver cannot reach from the
// datastore (they live in webapi config), injected by the handler.
type CatalogParams struct {
	DefaultTargetVersion string
}

// ExplainCatalog returns the static list of canned explains. No DB access, so it
// is safe for the catalog-listing endpoint.
func (db *DB) ExplainCatalog() []ExplainCatalogEntry {
	return []ExplainCatalogEntry{
		{explainRolesList, "Roles list", "Materialised roles-list read path (role_summary rollup).", true},
		{explainNodeListHeavy, "Node list (heavy JSON)", "Node-snapshots list with the heavy JSONB projection (filesystem, cookbooks, custom_attributes) — a known slow full-row path.", true},
		{explainNodeListLight, "Node list (light)", "Node-snapshots list without the heavy JSONB columns, for comparison.", true},
		{explainCookbookCover, "Cookbook coverage containment", "The `cookbooks ? <name>` JSONB key-existence scan over node_snapshots (a known sequential-scan hotspot).", true},
		{explainNodeSingleRow, "Single node full row", "Most-recent full-row snapshot fetch for one node by org + name.", true},
		{explainDistinctRoles, "Distinct node roles", "Distinct role names unnested from node_snapshots.roles.", true},
	}
}

// ResolveCatalogExplain returns the SQL + args for a catalog key, plus a human
// label and a param summary (counts only — never raw identifier values). Returns
// ErrExplainUnavailable when the entry needs live sample data that is absent.
func (db *DB) ResolveCatalogExplain(ctx context.Context, key string, p CatalogParams) (sqlText string, args []interface{}, label, paramSummary string, err error) {
	switch key {
	case explainRolesList:
		orgs, oerr := db.ListOrganisations(ctx)
		if oerr != nil {
			return "", nil, "", "", fmt.Errorf("datastore: resolving roles_list orgs: %w", oerr)
		}
		names := make([]string, 0, len(orgs))
		for _, o := range orgs {
			names = append(names, o.Name)
		}
		q, a := buildRoleListQuery(RoleFilter{OrganisationNames: names, Limit: explainSampleLimit})
		return q, a, "Roles list", fmt.Sprintf("orgs=%d, limit=%d", len(names), explainSampleLimit), nil

	case explainNodeListHeavy:
		q, a := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{IncludeHeavyJSON: true, Limit: explainSampleLimit})
		return q, a, "Node list (heavy JSON)", fmt.Sprintf("heavy JSONB projection, limit=%d", explainSampleLimit), nil

	case explainNodeListLight:
		q, a := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{IncludeHeavyJSON: false, Limit: explainSampleLimit})
		return q, a, "Node list (light)", fmt.Sprintf("light projection, limit=%d", explainSampleLimit), nil

	case explainDistinctRoles:
		q, a := buildDistinctNodeRolesQuery(NodeSnapshotFilter{}, DistinctValueOpts{})
		return q, a, "Distinct node roles", "all nodes", nil

	case explainCookbookCover:
		name, cerr := db.sampleActiveCookbookName(ctx)
		if cerr != nil {
			return "", nil, "", "", fmt.Errorf("datastore: resolving sample cookbook: %w", cerr)
		}
		if name == "" {
			return "", nil, "", "", ErrExplainUnavailable
		}
		return productionPlatformsForCookbookQuery, []interface{}{name}, "Cookbook coverage containment", "1 sample cookbook", nil

	case explainNodeSingleRow:
		org, node, nerr := db.sampleNodeIdentity(ctx)
		if nerr != nil {
			return "", nil, "", "", fmt.Errorf("datastore: resolving sample node: %w", nerr)
		}
		if org == "" || node == "" {
			return "", nil, "", "", ErrExplainUnavailable
		}
		return nodeSnapshotByNameQuery, []interface{}{org, node}, "Single node full row", "1 sample node (latest)", nil

	default:
		return "", nil, "", "", fmt.Errorf("datastore: unknown explain catalog key %q", key)
	}
}

// RunExplain runs EXPLAIN on sqlText+args inside a READ ONLY transaction bounded
// by statement_timeout, then rolls back. It never commits and never mutates.
// FORMAT TEXT returns the plan as many rows; the whole body is joined.
func (db *DB) RunExplain(ctx context.Context, sqlText string, args []interface{}, opts ExplainOptions) (ExplainResult, error) {
	if opts.StatementTimeoutMs <= 0 {
		opts.StatementTimeoutMs = defaultExplainTimeoutMs
	}
	if opts.MaxPlanBytes <= 0 {
		opts.MaxPlanBytes = defaultExplainMaxBytes
	}

	tx, err := db.pool.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ExplainResult{}, fmt.Errorf("datastore: beginning read-only explain transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// statement_timeout must be a literal (not a bind param); it is an int, so
	// fmt is injection-safe. SET LOCAL is transaction-scoped and resets on rollback.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", opts.StatementTimeoutMs)); err != nil {
		return ExplainResult{}, fmt.Errorf("datastore: setting explain statement_timeout: %w", err)
	}

	prefix := "EXPLAIN (FORMAT TEXT) "
	if opts.Analyze {
		prefix = "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) "
	}
	prefixed := prefix + sqlText

	runOnce := func() (ExplainRun, error) {
		start := time.Now()
		rows, qerr := tx.QueryContext(ctx, prefixed, args...)
		if qerr != nil {
			return ExplainRun{}, qerr
		}
		defer rows.Close()

		var b strings.Builder
		truncated := false
		for rows.Next() {
			var line string
			if serr := rows.Scan(&line); serr != nil {
				return ExplainRun{}, serr
			}
			if b.Len() < opts.MaxPlanBytes {
				b.WriteString(line)
				b.WriteByte('\n')
			} else {
				truncated = true
			}
		}
		if rerr := rows.Err(); rerr != nil {
			return ExplainRun{}, rerr
		}
		return ExplainRun{
			PlanText:   b.String(),
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
			Truncated:  truncated,
		}, nil
	}

	var result ExplainResult
	if result.Run1, err = runOnce(); err != nil {
		return ExplainResult{}, fmt.Errorf("datastore: running explain: %w", err)
	}
	if opts.RunTwice {
		run2, rerr := runOnce()
		if rerr != nil {
			return ExplainResult{}, fmt.Errorf("datastore: running explain (second run): %w", rerr)
		}
		result.Run2 = &run2
	}
	return result, nil
}

// ValidateFreeformExplainSQL guards admin-entered SQL: exactly one statement whose
// leading keyword is an explainable data statement. Writes are permitted — plan-only
// EXPLAIN never executes them and the READ ONLY transaction in RunExplain blocks any
// write under ANALYZE. Utility statements (COPY, VACUUM, DDL, …) are rejected because
// EXPLAIN cannot plan them. Returns the cleaned SQL.
func ValidateFreeformExplainSQL(sqlText string) (string, error) {
	s := strings.TrimSpace(sqlText)
	s = strings.TrimSpace(strings.TrimSuffix(s, ";"))
	if s == "" {
		return "", ErrExplainNotExplainable
	}
	if strings.Contains(s, ";") { // any remaining semicolon = multi-statement
		return "", ErrExplainNotExplainable
	}
	upper := strings.ToUpper(s)
	for _, kw := range explainableLeadingKeywords {
		// Match the keyword only as a leading word (followed by whitespace or "("),
		// so e.g. "UPDATED" or "SELECTED" as an identifier prefix is not matched.
		if upper == kw || strings.HasPrefix(upper, kw+" ") || strings.HasPrefix(upper, kw+"\t") ||
			strings.HasPrefix(upper, kw+"\n") || strings.HasPrefix(upper, kw+"(") {
			return s, nil
		}
	}
	return "", ErrExplainNotExplainable
}

// SQLWritesUnderReadOnly reports whether a RunExplain error is the read-only
// transaction rejecting a write (i.e. ANALYZE on an INSERT/UPDATE/DELETE/MERGE).
// The handler uses this to fall back to a plan-only EXPLAIN.
func SQLWritesUnderReadOnly(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "read-only")
}

// sampleActiveCookbookName returns any active server cookbook name, or "" if none.
func (db *DB) sampleActiveCookbookName(ctx context.Context) (string, error) {
	var name string
	err := db.pool.QueryRowContext(ctx,
		`SELECT name FROM server_cookbooks WHERE is_active = true LIMIT 1`).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// sampleNodeIdentity returns the most-recently-collected node's org + name, or
// empty strings if there are no node snapshots.
func (db *DB) sampleNodeIdentity(ctx context.Context) (org, node string, err error) {
	err = db.pool.QueryRowContext(ctx,
		`SELECT organisation_name, node_name FROM node_snapshots ORDER BY collected_at DESC LIMIT 1`).Scan(&org, &node)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return org, node, err
}
