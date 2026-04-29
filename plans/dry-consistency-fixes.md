# DRY Consistency Fixes

## Problem

Several values are computed in multiple places with divergent logic, risking inconsistencies between detail views, list views, and the dashboard.

## Fixes

### Fix 1: CookStyle Error → Consistent Status

`getCookbookCompatMap` (role detail) maps `error_message != '' → "untested"` while cookbook list uses `"scan_error"` and dashboard uses `"errored"`. Standardise on `"scan_error"` — it distinguishes "never tested" from "tested but scanner crashed", which is actionable.

- Role detail `getCookbookCompatMap`: change `THEN 'untested'` → `THEN 'scan_error'`
- Ensure consuming code handles `scan_error` as non-blocking (same as untested for readiness purposes)

### Fix 2: Shared TK Status Helper

Extract `ComputeTKStatus(passed, failed int) string` into `internal/gitkitchen/status.go`. All 4 in-Go aggregation paths call it.

Paths to update:
- `handle_dashboard_compatibility.go` (dashboard TK aggregation)
- `handle_git_repos.go` (git repos list)
- `role_detail.go` `getGitKitchenStatusMap` scan loop
- `git_kitchen_results.go` `ListGitKitchenStatusesByTargetVersions` scan loop

The SQL queries in `role_filter.go` use `CASE WHEN` in SQL which can't call Go — leave as-is (SQL is authoritative, Go helper mirrors it).

### Fix 3: Node Count Consistency

Role detail `getRoleBlastRadius` uses `COUNT(*)` which could double-count a node that appears in multiple snapshots. Change to `COUNT(DISTINCT organisation_name || '/' || node_name)` matching the role list CTE.

### Fix 4: Complexity Label Default

Role detail SQL uses `COALESCE(scc.complexity_label, 'none')`. Readiness leaves it as empty string when no record exists. Standardise: readiness should default to `"none"` when complexity record exists but label is null. When no record → leave empty (no complexity data). The role detail `COALESCE` is correct for its context (always has a LEFT JOIN row).

This is a display-only concern — both mean "no offenses". No code change needed if frontend handles both. Verify frontend treats empty and `"none"` identically.

## Out of Scope

- Org-scope mismatch in role detail (uses `orgs[0]`) — this is intentional design (role detail shows a single org's view). Document in spec only.
