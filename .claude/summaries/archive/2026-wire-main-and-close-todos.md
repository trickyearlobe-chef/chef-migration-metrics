# Wire `main.go` + Close Out Small Todos

**Date:** 2026-03-06
**Components:** `cmd/chef-migration-metrics/main.go`, `internal/analysis/`, `internal/remediation/`, `internal/embedded/`, `internal/logging/`, `internal/datastore/`

## Context

This session finished several small, independent, high-confidence tasks to make the analysis pipeline run automatically end-to-end and close out multiple todo areas at 100%.

## What Was Done

### 1. Wired Analysis Pipeline into `main.go` (Item 1)

Added a new section between secrets/organisation sync and the collection scheduler in `main.go` that:

1. **Resolves external tools at startup** — Creates `embedded.NewResolver(cfg.AnalysisTools.EmbeddedBinDir)` and calls `ValidateAll(ctx)` to check for `cookstyle`, `kitchen`, `docker`, and `git`. Each tool's availability, path, and version are logged at startup.

2. **Constructs pipeline components based on availability:**
   - **CookStyle available** → constructs `CookstyleScanner` (with `cfg.Concurrency.CookstyleScan` and `cfg.AnalysisTools.CookstyleTimeoutMinutes`) + `AutocorrectGenerator` (reuses cookstyle path and timeout)
   - **Kitchen + Docker available** → constructs `KitchenScanner` (with `cfg.Concurrency.TestKitchenRun`, `cfg.AnalysisTools.TestKitchenTimeoutMinutes`, `cfg.AnalysisTools.TestKitchen`)
   - **Always** → constructs `ComplexityScorer` and `ReadinessEvaluator` (both DB-only, no external tool required)

3. **Implements `cookbookDirFn`** — Resolves cookbook filesystem paths:
   - Git cookbooks: `<tempdir>/chef-migration-metrics/git-cookbooks/<name>` (matches the path used by `fetchGitCookbooks()` in Step 7c of `collectOrganisation`)
   - Chef server cookbooks: `<tempdir>/chef-migration-metrics/cookbook-cache/<org_id>/<name>/<version>` (only for downloaded cookbooks with `IsDownloaded() == true`)

4. **Passes all components via `With*` options** to `collector.New()`:
   ```go
   coll := collector.New(db, cfg, logger, credResolver, collOpts...)
   ```

**New imports in `main.go`:** `analysis`, `embedded`, `remediation`

### 2. Verified CookStyle Scanner Excludes Failed Downloads (Item 2)

Confirmed that `ScanCookbooks()` in `internal/analysis/cookstyle.go` already filters with:
```go
if !cb.IsChefServer() || !cb.IsDownloaded() {
    continue
}
```

`IsDownloaded()` returns `true` only when `DownloadStatus == "ok"`, so cookbooks with `"failed"` or `"pending"` status are silently excluded. Test Kitchen only processes git-sourced cookbooks (`cb.IsGit()`), and downstream components (complexity scorer, readiness evaluator) read from results tables — so no scan result exists to process for failed downloads.

**Marked done** in `todo/data-collection.md`.

### 3. Verified Pending Migrations Cause Startup Failure (Item 3)

Confirmed the error flow:
- `db.MigrateUp()` wraps errors with migration version and name: `"datastore: applying migration 0003 (cookbook_usage_analysis): <db error>"`
- `discoverMigrations()` reports missing directories with the path: `"migrations directory does not exist: /path"`
- `main.go` logs these at ERROR severity via the startup scope and exits with code 1

**2 new tests in `datastore_test.go`:**
- `TestDiscoverMigrations_NonexistentDir_DescriptiveError` — verifies error includes the directory path and mentions "does not exist"
- `TestDiscoverMigrations_DuplicateVersion_DescriptiveError` — verifies error mentions duplicate version number

**Marked done** in `todo/project-setup.md`.

### 4. Verified `tls` Log Scope (Item 4)

Confirmed `ScopeTLS` already exists with:
- Constant `ScopeTLS Scope = "tls"` in `logging.go`
- `WithTLSDomain` option and `TLSDomain` field on `Entry`
- Registered in `validScopes` map
- Migration `0002_log_entries_extra_columns` adds `tls_domain` column
- Tested in `TestIsValidScope`, `TestScope_StringValues`, `TestOptions_WithTLSDomain`

**Added `ScopeSecrets` and `ScopeRemediation`** to `TestIsValidScope` and `TestScope_StringValues` (they were defined in `validScopes` map but missing from tests).

**Marked done** in `todo/logging.md`.

Also changed the `[~]` cop-to-version mapping item in `todo/analysis.md` to `[x]` (was already marked NOT NEEDED but still showed as in-progress).

## Final State

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — all 11 packages pass

### Progress Overview

| Area | Done | Total | % | Change |
|------|-----:|------:|--:|--------|
| Specification writing | 32 | 32 | 100% | — |
| Project setup | 16 | 16 | **100%** | was 93% |
| Data collection | 65 | 67 | **97%** | was 95% |
| Analysis | 61 | 61 | **100%** | was 98% |
| Logging | 12 | 12 | **100%** | was 91% |
| Configuration | 43 | 83 | 51% | — |
| Secrets storage | 85 | 150 | 56% | — |
| Packaging | 18 | 97 | 18% | — |
| Visualisation | 0 | 86 | 0% | — |
| Auth | 0 | 5 | 0% | — |
| Testing | 4 | 40 | 10% | — |
| Documentation | 0 | 25 | 0% | — |
| **Total** | **336** | **674** | **49%** | was 46% |

**4 areas now at 100%:** specification, project-setup, analysis, logging

### Collection Pipeline (15 Steps — All Wired End-to-End)

1. Create collection run
2. Build Chef API client
3. Collect nodes via concurrent partial search
4. Convert to snapshot params + build NodeRecord slice
5. Persist node snapshots
6. Fetch cookbook inventory; upsert metadata
7. Mark active/stale cookbooks
7b. Fetch cookbook content from Chef server
7c. Fetch git cookbooks
8. Build cookbook-node usage records
9. Build role dependency graph
10. Run cookbook usage analysis
11. CookStyle scanning (auto-wired from `main.go`)
12. Test Kitchen (auto-wired from `main.go`)
13. Autocorrect previews + Complexity scoring (auto-wired from `main.go`)
14. Node readiness evaluation (auto-wired from `main.go`)
15. Complete collection run

## Known Gaps

- **Chef server cookbook content not written to disk** — `downloadCookbookVersion()` calls `client.GetCookbookVersion()` which returns metadata but doesn't extract file contents to the filesystem. The `cookbookDirFn` for server cookbooks points to `<tempdir>/chef-migration-metrics/cookbook-cache/<org_id>/<name>/<version>` but no code currently writes files there. CookStyle scanning will skip these cookbooks (empty dir → `cookbookDir` returns path but cookstyle finds no files). This needs a content extraction step added to `downloadCookbookVersion()`.
- **Dashboard display of failed cookbook versions** — `todo/data-collection.md` still has 1 visualisation-related item: "Display failed cookbook versions in the dashboard with a visual failure indicator" (belongs in `todo/visualisation.md`).
- **Checkpoint/resume for failed collection jobs** — 1 remaining item in `todo/data-collection.md`.

## Files Modified

### Production code
- `cmd/chef-migration-metrics/main.go` — added analysis pipeline wiring section (~95 lines): tool validation, scanner construction, cookbookDirFn, collOpts passed to `collector.New()`; added `analysis`, `embedded`, `remediation` imports

### Test code
- `internal/datastore/datastore_test.go` — 2 new tests for descriptive migration error messages
- `internal/logging/logging_test.go` — added `ScopeSecrets` and `ScopeRemediation` to `TestIsValidScope` and `TestScope_StringValues`

### Documentation
- `.claude/specifications/todo/data-collection.md` — marked failed-download exclusion from CookStyle as done (65/67)
- `.claude/specifications/todo/project-setup.md` — marked migration failure verification as done (16/16 = 100%)
- `.claude/specifications/todo/logging.md` — marked TLS log scope as done (12/12 = 100%)
- `.claude/specifications/todo/analysis.md` — changed cop-to-version `[~]` to `[x]` (61/61 = 100%)
- `.claude/specifications/ToDo.md` — updated all counts (313→336, 46%→49%)

## Recommended Next Steps

### 1. Implement cookbook content extraction to disk (small ~10k tokens)
- Extend `downloadCookbookVersion()` to download each file in the cookbook manifest and write to `<cookbook-cache>/<org_id>/<name>/<version>/`
- This unblocks CookStyle scanning for Chef server cookbooks
- **Specs:** `data-collection/Specification.md` (§ cookbook fetching), `packaging/Specification.md` (§ filesystem layout)

### 2. Configuration — TLS subsystem (medium ~25k tokens)
- Static TLS cert/key loading, ACME certificate management, HTTP redirect
- Will use the `ScopeTLS` and `WithTLSDomain` logging already prepared
- **Spec:** `configuration/Specification.md` (§ TLS)
- **Todo:** `todo/configuration.md` (43/83 — 40 remaining items, many are TLS)

### 3. Visualisation / Web API handlers (large ~40k tokens)
- All analysis data now flows end-to-end; web API handlers exist but frontend doesn't
- **Spec:** `visualisation/Specification.md`, `web-api/Specification.md`
- **Todo:** `todo/visualisation.md` (0/86)

### 4. Secrets storage — remaining items (medium ~20k tokens)
- 65 remaining items at 56% — mostly around credential CRUD web API, rotation UI, and audit logging
- **Spec:** `secrets-storage/Specification.md`
- **Todo:** `todo/secrets-storage.md` (85/150)