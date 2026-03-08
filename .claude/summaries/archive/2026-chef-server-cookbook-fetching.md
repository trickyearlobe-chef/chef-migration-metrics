# Chef Server Cookbook Fetching

**Date:** 2026-03-06
**Component:** Data Collection — Cookbook fetching pipeline

## Context

The data collection pipeline could collect node data, build cookbook inventories, determine active/unused cookbooks, build role dependency graphs, and run cookbook usage analysis — but it never actually **downloaded** cookbook content from the Chef server. The `GetCookbookVersion()` API method existed in `internal/chefapi/client.go` but was never called from the collection pipeline.

The specification (data-collection § 2.3, § 2.4) requires:
- Downloading cookbook versions from the Chef server with immutability optimisation (skip already-downloaded)
- Tracking download status (`ok`, `failed`, `pending`) per cookbook version
- Non-fatal failure handling with retry on next run
- Manual rescan support
- Only active cookbooks (applied to at least one node) should be fetched

## What Was Done

### 1. Migration `0004_cookbook_download_status` (new)

Added two columns to the `cookbooks` table:
- `download_status TEXT NOT NULL DEFAULT 'pending'` — one of `ok`, `failed`, `pending`
- `download_error TEXT` — nullable, populated only when status = `failed`

Plus a check constraint (`chk_cookbooks_download_status`) and index (`idx_cookbooks_download_status`). Existing rows migrated to `ok` since they were accepted before the download pipeline existed.

Down migration drops the columns, constraint, and index.

### 2. `internal/datastore/cookbooks.go` — Download status support

**New constants:** `DownloadStatusOK`, `DownloadStatusFailed`, `DownloadStatusPending`.

**New fields on `Cookbook` struct:** `DownloadStatus string` and `DownloadError string` (with `omitempty` JSON tag on error).

**New methods on `Cookbook`:**
- `IsDownloaded()` — returns true when status = `ok`
- `NeedsDownload()` — returns true when status = `pending` or `failed`

**Updated all SQL queries and scan functions** (15+ queries, both `scanCookbook` and `scanCookbooks`) to include `download_status` and `download_error` in SELECT and RETURNING clauses.

**Updated `ServerCookbookExists()`** to only consider `download_status = 'ok'` as "already present" — this is the immutability optimisation that ensures failed/pending versions are retried.

**Updated git cookbook upsert** to default `download_status = 'ok'` (git cookbooks are managed via clone/pull, not the download pipeline).

**New repository methods:**
- `UpdateCookbookDownloadStatus(ctx, params)` — update status and error for a cookbook by ID; validates status value
- `MarkCookbookDownloadOK(ctx, id)` — convenience wrapper for successful download
- `MarkCookbookDownloadFailed(ctx, id, error)` — convenience wrapper for failed download
- `ListCookbooksNeedingDownload(ctx, orgID)` — all server cookbooks with pending/failed status
- `ListActiveCookbooksNeedingDownload(ctx, orgID)` — only **active** server cookbooks with pending/failed status (used by the fetcher — unused cookbooks are flagged but not downloaded)
- `ResetCookbookDownloadStatus(ctx, id)` — manual rescan: sets status back to `pending`, clears error

### 3. `internal/collector/fetcher.go` (new)

**`fetchCookbooks()`** — orchestrates parallel download of active cookbook versions:
- Queries `ListActiveCookbooksNeedingDownload()` for the organisation
- Spawns goroutines with semaphore-based bounded concurrency (uses `concurrency.git_pull` config)
- Each goroutine calls `downloadCookbookVersion()`
- Collects results into `CookbookFetchResult` (Total, Downloaded, Failed, Skipped, Duration, Errors)
- Respects context cancellation — stops spawning new downloads on cancel

**`downloadCookbookVersion()`** — fetches a single cookbook version:
- Calls `client.GetCookbookVersion(ctx, name, version)`
- On success: `MarkCookbookDownloadOK()`
- On failure: `MarkCookbookDownloadFailed()` with formatted error detail; uses background context for the DB write if the parent context is cancelled
- Returns nil on success, error on failure

**`formatDownloadError()`** — produces human-readable error for storage:
- For `*chefapi.APIError`: includes HTTP status code (e.g. `"404 GET: cookbook version not found"`)
- For generic errors: uses `err.Error()` directly

**Types:** `CookbookFetchResult`, `CookbookFetchError` (implements `error` interface).

### 4. `internal/collector/collector.go` — Pipeline integration

Inserted **Step 7b** into `collectOrganisation()` between stale cookbook marking (Step 7) and cookbook-node usage records (Step 8):

1. Reads `concurrency.git_pull` for worker pool size (clamped to 1 if ≤ 0)
2. Calls `fetchCookbooks(ctx, client, db, log, org, concurrency)`
3. Logs summary: total, downloaded, failed, skipped, duration
4. Logs individual errors at WARN level
5. Non-fatal — the collection run continues regardless of download failures

### 5. `internal/collector/fetcher_test.go` (new) — 25+ test cases

- `formatDownloadError` with APIError (404, 403, 500, empty body, large body) and generic errors
- `CookbookFetchError.Error()` formatting
- `CookbookFetchResult` zero value and accounting (Total = Downloaded + Failed + Skipped)
- Download status constants validation
- `Cookbook.IsDownloaded()` and `Cookbook.NeedsDownload()` with all status values
- `Cookbook.MarshalJSON()` includes download fields, omits empty error
- `UpdateCookbookDownloadStatusParams` field validation
- Source type interaction with download status (git vs chef_server)

### 6. Updated documentation

- `.claude/Structure.md` — added `fetcher.go`, `fetcher_test.go`, migration 0004; updated cookbooks.go description
- `.claude/specifications/todo/data-collection.md` — marked 14 items as complete (Chef server fetch, immutability skip, keying, manual rescan, download_status/error columns, all failure handling types, non-fatal failures, WARN logging, retry, manual rescan clear)
- `.claude/specifications/ToDo.md` — updated data-collection from 43/67 (64%) to 57/67 (85%); overall from 259/674 (38%) to 273/674 (40%); added token estimates for all remaining tasks

## What Was NOT Done

- **Git cookbook fetching** (clone, pull, default branch detection, HEAD SHA) — next task
- **Exclude failed cookbooks from CookStyle/compatibility analysis** — filter predicate in future analysis code
- **Dashboard display of failed downloads** — Visualisation component
- **Checkpoint/resume** — separate reliability improvement

## Test Results

All tests pass across all packages:
```
internal/analysis      — PASS
internal/chefapi       — PASS
internal/collector     — PASS (includes 25+ new fetcher tests)
internal/config        — PASS
internal/datastore     — PASS
internal/logging       — PASS
internal/secrets       — PASS
```

`go vet ./...` clean. `go build ./...` clean.

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `migrations/0004_cookbook_download_status.up.sql` | Created | Add download_status, download_error columns |
| `migrations/0004_cookbook_download_status.down.sql` | Created | Rollback migration |
| `internal/collector/fetcher.go` | Created | Cookbook version download orchestrator |
| `internal/collector/fetcher_test.go` | Created | 25+ fetcher tests |
| `internal/datastore/cookbooks.go` | Modified | Download status fields, constants, methods, updated all queries/scans |
| `internal/collector/collector.go` | Modified | Step 7b — wired fetchCookbooks into pipeline |
| `.claude/Structure.md` | Modified | Added new files, updated descriptions |
| `.claude/specifications/todo/data-collection.md` | Modified | Marked 14 items complete |
| `.claude/specifications/ToDo.md` | Modified | Updated counts, added token estimates |

## Next Steps — Token Estimates

| # | Task | Input Tokens | Output Tokens | Notes |
|---|------|-------------:|--------------:|-------|
| 1 | **Git cookbook fetching** | ~25k | ~15k | `gitfetcher.go` — `os/exec` git commands, default branch detection, HEAD SHA, test suite detection, parallel `concurrency.git_pull`. Datastore layer exists. |
| 2 | **Embedded tool resolution** | ~15k | ~8k | `internal/embedded/` — PATH lookup, embedded dir check, startup validation for cookstyle/kitchen/docker. |
| 3 | **CookStyle integration** | ~30k | ~20k | Run cookstyle, version profiles, cop mapping, parallel, timeout, immutability skip, persist. |
| 4 | **Test Kitchen integration** | ~30k | ~18k | Run TK on git cookbooks, multiple target versions, skip if HEAD unchanged, parallel, timeout, persist. |
| 5 | **Remediation guidance** | ~25k | ~15k | Auto-correct previews, cop-to-docs mapping, complexity scoring with blast radius. |
| 6 | **Node readiness evaluation** | ~20k | ~10k | Per-node per-version readiness, cookbook compat + disk space, parallel, persist. |
| 7 | **Checkpoint/resume** | ~25k | ~12k | Page-level checkpointing, startup recovery, graceful shutdown. |
| 8 | **Web API** | ~40k | ~30k | Router, middleware, REST endpoints, manual triggers, exports. |
| 9 | **Auth** | ~20k | ~12k | Local, LDAP, SAML providers + RBAC. |
| 10 | **Visualisation (React)** | ~50k | ~40k | Dashboard, filters, deps graph, remediation, exports, log viewer. 2-3 sessions. |
| 11 | **Packaging** | ~30k | ~20k | RPM/DEB, container, Helm, CI/CD. |
| 12 | **Documentation** | ~15k | ~12k | User guide, dev guide, API docs. |

**Recommended next session:** Git cookbook fetching (#1) — completes the data-collection spec § 2, unblocks Test Kitchen.