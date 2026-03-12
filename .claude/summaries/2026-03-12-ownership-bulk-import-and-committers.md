# Ownership: Bulk Import & Cookbook Committers

**Date:** 2026-03-12
**Component:** Ownership tracking — bulk import, git committer datastore, cookbook committer endpoints
**Branch:** ownership/bulk-import-and-committers

---

## Context

The ownership tracking feature has a specification (`ownership/Specification.md`) covering owner entities, assignments, auto-derivation, bulk import, committer collection, and owner-scoped dashboard views. Prior to this session, the following were already implemented:

- **Config:** `OwnershipConfig` struct, defaults, env overrides, auto-rule validation
- **Database migration:** `0009_ownership_tracking.up.sql` — `owners`, `ownership_assignments`, `git_repo_committers`, `ownership_audit_log` tables, `custom_attributes` column on `node_snapshots`
- **Datastore (Go):** `owners.go` (CRUD + list + count), `ownership_assignments.go` (insert/list/get/delete/reassign/lookup), `ownership_audit_log.go` (insert/list/purge)
- **Web API handlers:** `handle_ownership.go` — owner CRUD, assignment CRUD, reassignment, ownership lookup, audit log
- **Route registration:** `/api/v1/owners`, `/api/v1/ownership/{reassign,lookup,audit-log}`
- **Mock store:** All ownership interfaces mocked

The `git_repo_committers` DB table existed in the migration but had **no Go code** to interact with it. Three spec sections were unimplemented: bulk import (§ 4.3), cookbook committer listing (§ 4.2 `GET /api/v1/cookbooks/:name/committers`), and committer-to-owner assignment (§ 4.2 `POST /api/v1/cookbooks/:name/committers/assign`).

---

## What Was Done

### 1. Datastore: `git_repo_committers.go` (new file)

**File:** `internal/datastore/git_repo_committers.go`

Created the full datastore layer for the `git_repo_committers` table:

- **Types:** `GitRepoCommitter` struct (8 fields with JSON tags), `CommitterListFilter` struct (repo URL, since, sort, order, limit, offset)
- **`ListCommittersByRepo(ctx, filter)`** — Paginated listing with dynamic WHERE clause, configurable sort (`last_commit_at`, `commit_count`, `author_name`), order (`asc`/`desc`), and optional `since` filter on `last_commit_at`. Returns total count for pagination.
- **`ReplaceCommittersForRepo(ctx, gitRepoURL, committers)`** — Transactional delete-all + insert for refreshing committer data during collection runs.
- **`GetGitRepoURLForCookbook(ctx, cookbookName)`** — Helper that looks up the `git_repo_url` from the `cookbooks` table for `source = 'git'` cookbooks. Returns `ErrNotFound` if no git-sourced cookbook exists.
- **Scan helpers:** `scanCommitter` (single row) and `scanCommitters` (multi-row)

All methods follow the public → private `queryable` delegation pattern used throughout the codebase.

### 2. Web API: `handle_ownership_import.go` (new file)

**File:** `internal/webapi/handle_ownership_import.go`

Three handlers:

#### `handleOwnershipImport` — `POST /api/v1/ownership/import`

Bulk import ownership assignments from CSV or JSON file upload:

- Accepts `multipart/form-data` with `file` and `format` (`csv`/`json`) fields
- CSV: validates `owner,entity_type,entity_key,organisation,notes` header
- JSON: parses `{"assignments": [...]}`
- 10,000-row limit per request
- Validates entity types (`node`, `cookbook`, `git_repo`, `role`, `policy`)
- Resolves owner names → IDs via `GetOwnerByName()` (caches lookups)
- Resolves organisation names → IDs via `GetOrganisationByName()` (caches lookups)
- Creates assignments with `assignment_source = "import"`, `confidence = "definitive"`
- Skips duplicates (not treated as errors)
- Non-transactional: partial success is allowed; full outcome reported
- Audits each imported assignment
- Response: `{"imported": N, "skipped": N, "errors": [{"line": N, "error": "..."}]}`
- Requires operator or admin role

#### `handleCookbookCommitters` — `GET /api/v1/cookbooks/:name/committers`

List git committers for a cookbook's source repository:

- Looks up git repo URL via `GetGitRepoURLForCookbook()` — 404 if not git-sourced
- Query params: `since` (RFC3339), `page`, `per_page`, `sort` (`last_commit_at`/`commit_count`/`author_name`), `order` (`asc`/`desc`)
- Custom response envelope: `{"cookbook_name", "git_repo_url", "data", "pagination"}`

#### `handleCookbookCommittersAssign` — `POST /api/v1/cookbooks/:name/committers/assign`

Assign committers as owners of a cookbook's git repository:

- Request: `{"committers": [{"author_email", "owner_name", "display_name"}]}`
- Auto-creates owner (`owner_type = "individual"`, `contact_email = author_email`) if it doesn't exist, with race-condition handling via retry
- Creates `git_repo` assignments with `assignment_source = "manual"`, `confidence = "definitive"`
- Skips duplicate assignments
- Response: `{"owners_created", "assignments_created", "skipped"}`
- Requires ownership enabled + operator/admin role

### 3. Route wiring

- **`handle_cookbooks.go`** — Added sub-path dispatch in `handleCookbookDetail` for `/api/v1/cookbooks/:name/committers` and `/api/v1/cookbooks/:name/committers/assign`
- **`handle_ownership.go`** — Added `"/api/v1/ownership/import"` case to `handleOwnershipEndpoints` dispatch switch
- **`router.go`** — Registered `r.protect("/api/v1/ownership/import", r.handleOwnershipEndpoints)`

### 4. Interface & mock updates

- **`store.go`** — Added `GetGitRepoURLForCookbook` and `ListCommittersByRepo` to the `DataStore` interface
- **`store_mock_test.go`** — Added corresponding mock fields and method implementations

---

## Build & Test

- `go build ./...` — all packages compile cleanly
- `go test ./...` — all 14 packages pass (0 failures)

---

## Files Created

| File | Description |
|------|-------------|
| `internal/datastore/git_repo_committers.go` | Datastore layer for `git_repo_committers` table |
| `internal/webapi/handle_ownership_import.go` | Bulk import + cookbook committer handlers |

## Files Modified

| File | Change |
|------|--------|
| `internal/webapi/store.go` | Added `GetGitRepoURLForCookbook`, `ListCommittersByRepo` to `DataStore` interface |
| `internal/webapi/store_mock_test.go` | Added mock fields and methods for new interface methods |
| `internal/webapi/handle_cookbooks.go` | Added committers sub-path dispatch in `handleCookbookDetail` |
| `internal/webapi/handle_ownership.go` | Added import route to `handleOwnershipEndpoints` dispatch |
| `internal/webapi/router.go` | Registered `/api/v1/ownership/import` route |

---

## Remaining Ownership Work

In rough priority/dependency order:

1. **Git committer collection** (Spec § 7.2) — Integrate into the data collection pipeline to extract committer info from git repos and populate `git_repo_committers` via `ReplaceCommittersForRepo()`
2. **Auto-derivation engine** (Spec § 2.3, § 7.3) — Evaluate configured auto-rules after each collection run; pattern matching for all 6 rule types; custom attribute collection from node partial search
3. **Owner filter on existing endpoints** (Spec § 4.5) — Add `owner`/`unowned` query params to ~12 existing dashboard/list endpoints
4. **Owner detail enrichment** (Spec § 4.1) — `readiness_summary`, `cookbook_summary`, `git_repo_summary` fields in `GET /api/v1/owners/:name`
5. **Export integration** (Spec § 8) — Add `owners` columns to CSV/JSON exports; support owner filters in export requests
6. **Notification integration** (Spec § 9) — Owner-scoped notification channels; ownership change event types
7. **Frontend / Dashboard** (Spec § 5) — Owner filter bar, ownership summary view, owner indicators on lists, ownership management UI, committers sub-page, audit log viewer
8. **Auto-rule cleanup on startup** (Spec § 10) — Delete stale auto_rule assignments for removed rules
9. **Audit log retention purge** (Spec § 10) — Daily background job to purge entries older than `retention_days`
