# Data Collection — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Node Collection

- [x] Implement Chef Infra Server API client in Go with native RSA signed request authentication (no external signing libraries) — `internal/chefapi/client.go` with v1.3 SHA-256 signing, PKCS#1 and PKCS#8 key support; 74 tests in `client_test.go`
- [x] Implement partial search against node index (`POST /organizations/NAME/search/node`)
- [x] Collect required node attributes:
  - [x] `name`
  - [x] `chef_environment`
  - [x] `automatic.chef_packages.chef.version`
  - [x] `automatic.platform` and `automatic.platform_version`
  - [x] `automatic.platform_family`
  - [x] `automatic.filesystem` (disk space)
  - [x] `automatic.cookbooks` (resolved cookbook list)
  - [x] `run_list`
  - [x] `automatic.roles` (expanded)
  - [x] `policy_name` (Policyfile policy name, top-level attribute)
  - [x] `policy_group` (Policyfile policy group, top-level attribute)
  - [x] `automatic.ohai_time` (Unix timestamp of last Chef client run)
- [x] Support multiple Chef server organisations — `Collector.Run()` loads all orgs from the database and collects each in parallel
- [x] Collect from multiple organisations in parallel using goroutines (one per organisation) — `Collector.Run()` dispatches goroutines per org with a results channel
- [x] Bound organisation collection concurrency with the `concurrency.organisation_collection` worker pool setting — semaphore channel in `Collector.Run()` sized by `cfg.Concurrency.OrganisationCollection`
- [x] Use `errgroup` or equivalent to coordinate goroutines and aggregate errors without cancelling successful collections — `sync.WaitGroup` + results channel; errors collected in `RunResult.Errors` map per org without cancelling other orgs
- [x] Implement concurrent pagination within a single organisation — fetch pages in parallel once total node count is known, bounded by the `concurrency.node_page_fetching` worker pool setting — `CollectAllNodesConcurrent()` with configurable worker pool
- [x] Implement periodic background collection job — `Scheduler` in `internal/collector/scheduler.go` with built-in cron parser, `Start()`/`Stop()` lifecycle, skip-if-running guard, panic recovery
- [x] Implement fault tolerance — failure in one organisation must not block others — each org collected in its own goroutine; errors logged and recorded in `RunResult.Errors` but do not cancel sibling goroutines
- [x] Implement checkpoint/resume so failed jobs can continue without starting over — `ResumeInterruptedRuns()` in `internal/collector/collector.go` evaluates interrupted runs on startup: fresh runs (within 2 collection intervals) are re-collected for incomplete organisations; stale runs are abandoned as failed. `GetInterruptedCollectionRuns()`, `AbandonCollectionRun()`, `ResumeCollectionRun()`, and `ListCompletedRunsForOrganisation()` added to `internal/datastore/collection_runs.go`. `estimateCollectionInterval()` parses the cron schedule to determine freshness cutoff. `runForOrganisations()` executes targeted collection for a subset of orgs. Wired into `main.go` after collector creation and before scheduler start. 13 tests in `collector_test.go`.
- [x] Persist collected node data to datastore with timestamps — `BulkInsertNodeSnapshots` / `BulkInsertNodeSnapshotsReturningIDs` in `collectOrganisation()` with `collected_at` timestamp

## Policyfile Support

- [x] Classify nodes as Policyfile nodes (both `policy_name` and `policy_group` non-null) or classic nodes — `NodeData.IsPolicyfileNode()`
- [x] Persist `policy_name` and `policy_group` in node snapshots — included in `InsertNodeSnapshotParams` and persisted by `BulkInsertNodeSnapshots`
- [x] Ensure cookbook usage analysis works identically for Policyfile and classic nodes (both use `automatic.cookbooks`) — `internal/analysis/usage.go` extracts `CookbookVersions` identically for both node types; Policyfile nodes additionally contribute `PolicyName` and `PolicyGroup` to aggregated sets; tested in `TestEndToEnd_FullAnalysisPipeline` with mixed classic and Policyfile nodes

## Stale Node Detection

- [x] After collection, compare each node's `ohai_time` against the current time — `NodeData.IsStale(threshold)` with `OhaiTimeAsTime()` conversion
- [x] Flag nodes whose `ohai_time` is older than `collection.stale_node_threshold_days` (default: 7) as stale — `NodeData.IsStale()` treats missing ohai_time as stale
- [x] Persist `is_stale` flag on `node_snapshots` rows — `IsStale` field in `InsertNodeSnapshotParams`, persisted by `BulkInsertNodeSnapshots`
- [x] Include stale flagging in the collection run sequence (step 5, after cookbook fetching) — stale threshold evaluated during Step 4 (snapshot param construction) in `collectOrganisation()`

## Cookbook Fetching

- [x] Implement cookbook fetch from Chef server (`GET /organizations/NAME/cookbooks/NAME/VERSION`) — `downloadCookbookVersion()` in `internal/collector/fetcher.go` calls `client.GetCookbookVersionManifest()` to fetch the manifest, then downloads each file from its bookshelf URL via `client.DownloadFileContent()` with SHA-256 checksum validation, writing to `<cookbookCacheDir>/<org_id>/<name>/<version>/`; `fetchCookbooks()` orchestrates parallel downloads of active cookbook versions with pending/failed status; path traversal protection via `hasParentTraversal()` and `isSubPath()`; partial downloads cleaned up on failure via `os.RemoveAll()`
- [x] Extract cookbook file content to disk for CookStyle scanning — `extractCookbookFiles()` downloads every file in the cookbook manifest (recipes, attributes, files, templates, libraries, definitions, resources, providers, root_files) from bookshelf URLs and writes to `<cookbookCacheDir>/<org_id>/<name>/<version>/<path>`; `WithCookbookCacheDir()` option on Collector wires the cache directory; `CookbookVersionManifest` and `CookbookFileRef` types in `internal/chefapi/client.go` parse the manifest; `DownloadFileContent()` performs plain HTTP GET with optional SHA-256 checksum validation; 35 new tests across `fetcher_test.go` and `client_test.go`
- [x] Skip download of cookbook versions already present in the datastore (immutability optimisation) — `ServerCookbookExists()` now checks `download_status = 'ok'`; `ListActiveCookbooksNeedingDownload()` only returns pending/failed versions
- [x] Key all Chef server cookbook data in the datastore by organisation + cookbook name + version — partial unique index `uq_cookbooks_server ON (organisation_id, name, version) WHERE source = 'chef_server'` (existing); download status tracked per this key
- [x] Implement manual rescan option to force re-download and re-analysis of a specific cookbook version — `ResetCookbookDownloadStatus()` in `internal/datastore/cookbooks.go` sets status back to `pending` and clears error
- [x] Implement cookbook clone from git repository — `GitCookbookManager.CloneOrPull()` in `internal/collector/git.go` handles clone via `git clone --quiet` and pull via `git fetch --quiet origin` + `git reset --hard origin/<branch>`; `GitExecutor` interface abstracts git commands for testability; 45 tests in `git_test.go`
- [x] Support multiple configured base git URLs — `fetchGitCookbooks()` iterates all `config.GitBaseURLs`, constructs candidate URLs via `BuildGitCookbookURLs()`, and tries each base URL per cookbook; first successful clone/pull wins and remaining base URLs are skipped for that cookbook
- [x] Pull latest changes from git repositories on every collection run — Step 7c in `collectOrganisation()` calls `fetchGitCookbooks()` on every run; existing repos are fetched + hard-reset to remote HEAD rather than pulled, avoiding merge conflicts per specification
- [x] Run git pull operations across multiple repositories in parallel using goroutines, bounded by the `concurrency.git_pull` worker pool setting — `fetchGitCookbooks()` uses a buffered channel semaphore sized by `concurrency.git_pull` (same pool as Chef server cookbook downloads); `sync.WaitGroup` coordinates goroutines
- [x] Detect default branch automatically (`main` or `master`) — `GitCookbookManager.detectDefaultBranch()` first tries `git symbolic-ref refs/remotes/origin/HEAD --short` and strips the `origin/` prefix; falls back to `git rev-parse --verify origin/main` then `origin/master`; all machine-parseable invocations per specification
- [x] Record HEAD commit SHA for the default branch after each pull — `GitCookbookManager.readHeadSHA()` runs `git rev-parse HEAD` and validates the 40-char SHA; SHA is persisted via `db.UpsertGitCookbook()` with `HeadCommitSHA` field; change detection compares old vs new SHA
- [x] Detect whether a fetched cookbook includes a test suite — `GitCookbookManager.detectTestSuite()` checks for `.kitchen.yml`, `.kitchen.yaml`, `kitchen.yml`, `kitchen.yaml`, `test/`, and `spec/` via `git ls-tree --name-only HEAD -- <path>`; result stored in `HasTestSuite` field via `UpsertGitCookbook()`
- [x] Record `first_seen_at` timestamp for each cookbook version (proxy for upload date if Chef server does not expose one) — `FirstSeenAt` field in `UpsertServerCookbookParams`, set to `now` on first insert, preserved on conflict via `COALESCE(cookbooks.first_seen_at, EXCLUDED.first_seen_at)`
- [x] Flag cookbooks as stale when most recent version's `first_seen_at` is older than `collection.stale_cookbook_threshold_days` (default: 365) — `MarkStaleCookbooksForOrg()` called in Step 7 of `collectOrganisation()`

## Cookbook Download Failure Handling

- [x] Add `download_status` column to `cookbooks` table (`ok`, `failed`, `pending`) with default `pending` — migration `0004_cookbook_download_status.up.sql`; `DownloadStatus` field on `Cookbook` struct; constants `DownloadStatusOK`, `DownloadStatusFailed`, `DownloadStatusPending`
- [x] Add `download_error` column to `cookbooks` table (nullable TEXT — error message and HTTP status code if applicable) — migration `0004_cookbook_download_status.up.sql`; `DownloadError` field on `Cookbook` struct; `formatDownloadError()` in `fetcher.go` formats API errors with HTTP status codes
- [x] Handle corrupted downloads (truncated response, checksum mismatch) — set `download_status = 'failed'` with error detail — `downloadCookbookVersion()` catches all errors from `GetCookbookVersion()` and calls `MarkCookbookDownloadFailed()`
- [x] Handle missing cookbook versions (404 from Chef server) — set `download_status = 'failed'` with error detail — `formatDownloadError()` extracts HTTP status code from `APIError`
- [x] Handle network errors (timeouts, TLS failures) — set `download_status = 'failed'` with error detail — generic errors formatted via `formatDownloadError()`
- [x] Handle permission errors (403 from Chef server ACLs) — set `download_status = 'failed'` with error detail — `formatDownloadError()` extracts HTTP status code from `APIError`
- [x] Ensure download failure for one cookbook version does not fail the collection run — continue with remaining cookbooks and organisations — `fetchCookbooks()` records failures in `CookbookFetchResult.Errors` and continues; Step 7b in `collectOrganisation()` is non-fatal
- [x] Log each download failure at `WARN` severity with `collection_run` scope, including organisation, cookbook name, version, and error detail — `fetchCookbooks()` logs each `CookbookFetchError` at WARN; Step 7b also logs summary and individual errors
- [x] Exclude cookbook versions with `download_status = 'failed'` from CookStyle scanning and compatibility analysis — `ScanCookbooks()` in `internal/analysis/cookstyle.go` filters with `!cb.IsDownloaded()` which rejects any status other than `"ok"` (i.e. both `"failed"` and `"pending"` are excluded); Test Kitchen only processes git-sourced cookbooks so is unaffected; downstream components (complexity scorer, readiness evaluator) read from CookStyle/TK results tables so naturally exclude cookbooks with no scan results
- [x] Display failed cookbook versions in the dashboard with a visual failure indicator — `handleDashboardCookbookDownloadStatus` at `GET /api/v1/dashboard/cookbook-download-status` returns aggregate counts by download status (ok/failed/pending), percentages, a sorted list of failed cookbook versions with error details (active cookbooks first), organisation names, and a human-readable failure message. Only counts `chef_server`-sourced cookbooks (git cookbooks excluded). Registered in `router.go`. 15 tests in `handle_dashboard_test.go`.
- [x] Retry cookbook versions with `failed` or `pending` download status on next collection run (bypass immutability skip) — `ListActiveCookbooksNeedingDownload()` queries `download_status IN ('pending', 'failed')`; `ServerCookbookExists()` only considers `download_status = 'ok'`
- [x] Clear `failed` status and force fresh download on manual rescan — `ResetCookbookDownloadStatus()` sets status to `pending` and clears error

## Cookbook-Node Usage Linkage

- [x] Build cookbook-node usage records linking each node snapshot to the cookbooks it runs — Step 8 in `collectOrganisation()` builds `InsertCookbookNodeUsageParams` from per-node cookbook versions and bulk-inserts via `BulkInsertCookbookNodeUsage`
- [x] Implement batch cookbook ID lookup to avoid N+1 queries — `GetServerCookbookIDMap()` returns `map[name][version]id` for an organisation in a single query
- [x] Return node snapshot IDs from bulk insert for usage record correlation — `BulkInsertNodeSnapshotsReturningIDs()` returns `map[nodeName]snapshotID` using `RETURNING id`
- [x] Track per-node cookbook versions during collection — `nodeCookbookVersions` map built alongside `activeCookbookNames` during Step 4 snapshot param construction
- [x] Handle missing cookbook lookups gracefully — missing cookbook name or version increments a counter and logs a WARN; does not fail the collection run

## Role Dependency Graph Collection

- [x] Fetch full list of roles per organisation using `GET /organizations/ORG/roles` — `Client.GetRoles()`
- [x] Fetch role detail per role using `GET /organizations/ORG/roles/ROLE_NAME` — `Client.GetRole()` returning `RoleDetail` with `RunList` and `EnvRunLists`
- [x] Parse each role's `run_list` to extract cookbook references (`recipe[cookbook::recipe]`) and nested role references (`role[other_role]`) — `ParseRunListEntry()` and `ParseRunList()` in `internal/collector/runlist.go` with regex-based extraction; strips recipe names and version pins to extract cookbook name
- [x] Build directed graph of role → role and role → cookbook dependencies — `BuildRoleDependencies()` in `internal/collector/runlist.go` processes all roles' default and env_run_lists, deduplicates within each role, produces `[]InsertRoleDependencyParams`
- [x] Persist dependency graph to the `role_dependencies` table in the datastore — `ReplaceRoleDependenciesForOrg()` in `internal/datastore/role_dependencies.go` atomically replaces all edges for an org in a single transaction; also provides `BulkUpsertRoleDependencies`, query, and aggregation methods
- [x] Refresh dependency graph on every collection run — Step 9 in `collectOrganisation()` fetches all roles, parses run_lists, and calls `ReplaceRoleDependenciesForOrg()` on every collection cycle