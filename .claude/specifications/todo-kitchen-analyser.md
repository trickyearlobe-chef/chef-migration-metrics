# Kitchen Analyser — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## DB Migration (0012)

- [x] Create `kitchen_analysis_results` table with FK to `git_repos`
- [x] Create `kitchen_discovered_platforms` aggregate table
- [x] Index on `driver_name`
- [x] Up and down migration scripts

## Datastore Layer

- [x] `KitchenAnalysisResult` and `KitchenDiscoveredPlatform` types
- [x] `UpsertKitchenAnalysisResult` — insert or update per-repo analysis
- [x] `GetKitchenAnalysisResult` — single repo lookup
- [x] `ListKitchenAnalysisResults` — all results
- [x] `ListKitchenAnalysisResultsFiltered` — filter by driver, has_local_override
- [x] `DeleteKitchenAnalysisResultsByRepo` — remove analysis for a repo
- [x] `RebuildDiscoveredPlatforms` — rebuild aggregate table from analysis results
- [x] `ListDiscoveredPlatforms` — all platforms with counts
- [x] `ListDiscoveredPlatformsFiltered` — filter by os_family, min_count
- [x] `GetKitchenAnalysisSummary` — aggregate stats (total scanned, driver counts, etc.)
- [x] Validation tests for param checks and edge cases

## YAML Parser

- [x] Add `gopkg.in/yaml.v3` dependency
- [x] Parse `.kitchen.yml` into structured config
- [x] Merge `.kitchen.local.yml` with TK semantics (deep-merge maps, replace arrays)
- [x] Extract driver name and settings
- [x] Extract provisioner name and settings
- [x] Extract platforms with all attributes and extensions
- [x] Extract suites with run_list, excludes, includes
- [x] Extract default transport block
- [x] Detect `.kitchen.*.yml` variant files
- [x] Tests for all extraction paths
- [x] Tests for merge semantics (map merge, array replace, nested merge)
- [x] Tests for malformed/empty YAML handling

## Platform Normaliser

- [x] Lowercase normalisation
- [x] Strip known suffixes (-chef16, -x86_64, -stable, -small, etc.)
- [x] Normalise OS prefixes (win → windows-, centos → centos-, etc.)
- [x] Normalise version formats (2k12 → 2012, 2k16 → 2016, 2k19 → 2019)
- [x] OS family detection (rhel, windows, debian, suse, other)
- [x] OS version extraction
- [x] Tests for normalisation edge cases
- [x] Tests for OS family detection

## Analyser Engine

- [x] `KitchenAnalyser` struct with configurable concurrency
- [x] `AnalyseRepo` — scan single repo directory, return result
- [x] `AnalyseAll` — concurrent analysis of all repos with worker pool
- [x] Store results via datastore upsert
- [x] Rebuild discovered platforms aggregate after full run
- [x] Tests with mock filesystem and sample configs

## Collection Pipeline Integration

- [x] Wire analyser into collector after git clone/fetch
- [x] Run analysis for repos with kitchen files
- [x] Skip analysis for repos with clone errors

## API Endpoints

- [x] `GET /api/v1/kitchen/analysis/summary` — aggregate stats
- [x] `GET /api/v1/kitchen/analysis/platforms` — discovered platforms with filters
- [x] `GET /api/v1/kitchen/analysis/cookbooks` — per-cookbook results, paginated
- [x] `GET /api/v1/kitchen/analysis/cookbooks/:name` — single cookbook detail
- [x] `POST /api/v1/kitchen/analysis/trigger` — re-analyse on demand (admin)
- [x] Add methods to `DataStore` interface in `store.go`
- [x] Register routes in `router.go`
- [x] Handler tests for all endpoints

## Frontend

- [x] `AdminKitchenAnalysisPage.tsx` page component
- [x] Summary cards (total with TK, without TK, platform count, driver breakdown, conflicts)
- [x] Platform table (sortable by name/count/os_family, mapped/unmapped indicator)
- [x] Conflict list (cookbooks with `.kitchen.local.yml` touching driver/platforms)
- [x] Route `/admin/kitchen-analysis` in `App.tsx`
- [x] Sidebar navigation link