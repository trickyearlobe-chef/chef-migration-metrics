# Kitchen Analyser — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## DB Migration (0012)

- [ ] Create `kitchen_analysis_results` table with FK to `git_repos`
- [ ] Create `kitchen_discovered_platforms` aggregate table
- [ ] Index on `driver_name`
- [ ] Up and down migration scripts

## Datastore Layer

- [ ] `KitchenAnalysisResult` and `KitchenDiscoveredPlatform` types
- [ ] `UpsertKitchenAnalysisResult` — insert or update per-repo analysis
- [ ] `GetKitchenAnalysisResult` — single repo lookup
- [ ] `ListKitchenAnalysisResults` — all results
- [ ] `ListKitchenAnalysisResultsFiltered` — filter by driver, has_local_override
- [ ] `DeleteKitchenAnalysisResultsByRepo` — remove analysis for a repo
- [ ] `RebuildDiscoveredPlatforms` — rebuild aggregate table from analysis results
- [ ] `ListDiscoveredPlatforms` — all platforms with counts
- [ ] `ListDiscoveredPlatformsFiltered` — filter by os_family, min_count
- [ ] `GetKitchenAnalysisSummary` — aggregate stats (total scanned, driver counts, etc.)
- [ ] Validation tests for param checks and edge cases

## YAML Parser

- [ ] Add `gopkg.in/yaml.v3` dependency
- [ ] Parse `.kitchen.yml` into structured config
- [ ] Merge `.kitchen.local.yml` with TK semantics (deep-merge maps, replace arrays)
- [ ] Extract driver name and settings
- [ ] Extract provisioner name and settings
- [ ] Extract platforms with all attributes and extensions
- [ ] Extract suites with run_list, excludes, includes
- [ ] Extract default transport block
- [ ] Detect `.kitchen.*.yml` variant files
- [ ] Tests for all extraction paths
- [ ] Tests for merge semantics (map merge, array replace, nested merge)
- [ ] Tests for malformed/empty YAML handling

## Platform Normaliser

- [ ] Lowercase normalisation
- [ ] Strip known suffixes (-chef16, -x86_64, -stable, -small, etc.)
- [ ] Normalise OS prefixes (win → windows-, centos → centos-, etc.)
- [ ] Normalise version formats (2k12 → 2012, 2k16 → 2016, 2k19 → 2019)
- [ ] OS family detection (rhel, windows, debian, suse, other)
- [ ] OS version extraction
- [ ] Tests for normalisation edge cases
- [ ] Tests for OS family detection

## Analyser Engine

- [ ] `KitchenAnalyser` struct with configurable concurrency
- [ ] `AnalyseRepo` — scan single repo directory, return result
- [ ] `AnalyseAll` — concurrent analysis of all repos with worker pool
- [ ] Store results via datastore upsert
- [ ] Rebuild discovered platforms aggregate after full run
- [ ] Tests with mock filesystem and sample configs

## Collection Pipeline Integration

- [ ] Wire analyser into collector after git clone/fetch
- [ ] Run analysis for repos with kitchen files
- [ ] Skip analysis for repos with clone errors

## API Endpoints

- [ ] `GET /api/v1/kitchen/analysis/summary` — aggregate stats
- [ ] `GET /api/v1/kitchen/analysis/platforms` — discovered platforms with filters
- [ ] `GET /api/v1/kitchen/analysis/cookbooks` — per-cookbook results, paginated
- [ ] `GET /api/v1/kitchen/analysis/cookbooks/:name` — single cookbook detail
- [ ] `POST /api/v1/kitchen/analysis/trigger` — re-analyse on demand (admin)
- [ ] Add methods to `DataStore` interface in `store.go`
- [ ] Register routes in `router.go`
- [ ] Handler tests for all endpoints

## Frontend

- [ ] `AdminKitchenAnalysisPage.tsx` page component
- [ ] Summary cards (total with TK, without TK, platform count, driver breakdown, conflicts)
- [ ] Platform table (sortable by name/count/os_family, mapped/unmapped indicator)
- [ ] Conflict list (cookbooks with `.kitchen.local.yml` touching driver/platforms)
- [ ] Route `/admin/kitchen-analysis` in `App.tsx`
- [ ] Sidebar navigation link