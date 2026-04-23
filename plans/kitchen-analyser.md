# Plan: Kitchen Analyser (Phase 1)

## Goal

Implement the Kitchen Analyser — scan cloned git repos for TK config files, parse YAML fully, merge `.kitchen.local.yml`, extract platforms/drivers/suites/transport, store in DB, expose via API, show in frontend.

## Specs

- `.claude/specifications/kitchen-analyser.md` — primary spec
- `.claude/specifications/kitchen-refactor.md` — context (Phase 1 only)
- `.claude/specifications/project-conventions.md` — coding standards

## Steps

### 1. DB Migration (0012)

- `kitchen_analysis_results` table (PK: `git_repo_name, git_repo_url`, FK to `git_repos`)
- `kitchen_discovered_platforms` table (PK: `platform_name`)
- Index on `driver_name`
- Up and down scripts

### 2. Datastore Layer

- Types: `KitchenAnalysisResult`, `KitchenDiscoveredPlatform`
- File: `internal/datastore/kitchen_analysis.go`
- Methods: `UpsertKitchenAnalysisResult`, `GetKitchenAnalysisResult`, `ListKitchenAnalysisResults`, `ListKitchenAnalysisResultsFiltered`, `DeleteKitchenAnalysisResultsByRepo`, `RebuildDiscoveredPlatforms`, `ListDiscoveredPlatforms`, `ListDiscoveredPlatformsFiltered`, `GetKitchenAnalysisSummary`
- Tests: `internal/datastore/kitchen_analysis_test.go` — validation, param checks

### 3. YAML Parser + Platform Normaliser

- File: `internal/analysis/kitchen_analyser.go`
- Full YAML parse (add `gopkg.in/yaml.v3` dependency)
- `.kitchen.local.yml` merge with TK semantics (deep-merge maps, replace arrays)
- Extract: driver, provisioner, platforms, suites, transport, extensions, variant files
- Platform normalisation: lowercase, strip suffixes, normalise OS prefixes/versions
- OS family detection: rhel, windows, debian, suse, other
- Tests first: `internal/analysis/kitchen_analyser_test.go`

### 4. Analyser Engine

- `KitchenAnalyser` struct with `AnalyseRepo(dir, gitRepo)` and `AnalyseAll(repos, repoBaseDir)`
- Calls parser, stores results via datastore, rebuilds aggregate table
- Concurrent per-repo analysis with bounded worker pool
- Tests: mock filesystem with sample kitchen configs

### 5. Integration into Collection Pipeline

- Wire analyser into collector after git clone/fetch step
- Run analysis for each repo that has kitchen files

### 6. API Endpoints

- File: `internal/webapi/handle_kitchen_analysis.go`
- `GET /api/v1/kitchen/analysis/summary` — aggregate stats
- `GET /api/v1/kitchen/analysis/platforms` — discovered platforms with filters
- `GET /api/v1/kitchen/analysis/cookbooks` — per-cookbook results, paginated
- `GET /api/v1/kitchen/analysis/cookbooks/:name` — single cookbook detail
- `POST /api/v1/kitchen/analysis/trigger` — re-analyse (admin only)
- Add to `DataStore` interface in `store.go`
- Register routes in `router.go`
- Tests: `handle_kitchen_analysis_test.go`

### 7. Frontend

- File: `frontend/src/pages/AdminKitchenAnalysisPage.tsx`
- Summary cards: total with TK, without TK, platform count, driver breakdown, conflicts
- Platform table: sortable, shows mapped/unmapped, OS family, cookbook count
- Conflict list: cookbooks with `.kitchen.local.yml` touching driver/platforms
- Route: `/admin/kitchen-analysis`
- Add to `App.tsx` routing and sidebar nav

### 8. Todo File

- Create `.claude/specifications/todo-kitchen-analyser.md` tracking all items

## Acceptance Criteria

- All unique platform names across estate are discoverable via API
- Per-cookbook analysis shows driver, platforms, suites, local override presence
- Aggregate summary shows platform counts, driver breakdown, conflict count
- Frontend displays summary cards, platform table, conflict list
- All tests pass
- No regressions in existing 18 packages

## Commit Strategy

One commit per step above. Migrations and datastore together if small enough.