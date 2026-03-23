# Specification: Readiness Evaluator Bulk-Load Optimisation

## Problem

The readiness evaluator (`internal/analysis/readiness.go`) uses an N+1 query
pattern that makes it unable to complete evaluation of large organisations in a
reasonable time.

For each of the ~60,000 nodes × 2 target versions = 120,000 work items,
`checkCookbookCompatibility` fires 5–7 individual database queries **per
cookbook on the node**:

1. `GetGitRepoByName` — resolve cookbook name → git repo
2. `GetLatestGitRepoTestKitchenResult` — TK result for the git repo
3. `GetGitRepoCookstyleResult` — CookStyle result for the git repo
4. `GetServerCookbookCookstyleResult` — CookStyle result for the server cookbook
5. `GetServerCookbookComplexity` — complexity for the server cookbook (enrichment)
6. `GetGitRepoComplexity` — complexity for the git repo (enrichment)

With ~20 cookbooks per node, this produces ~100+ queries per work item, or
~12 million total queries per evaluation run. The evaluator takes hours to
complete and dominates database load.

The same cookbook name+version combination is queried thousands of times across
different nodes — `apt 7.4.0` might appear on 50,000 nodes, but the CookStyle
result is the same every time.

## Solution

Bulk-load all lookup data into in-memory maps at the start of
`EvaluateOrganisation`, before the fan-out loop. Pass the maps into
`evaluateOne` and `checkCookbookCompatibility` instead of the
`ReadinessDataStore` interface. Replace ~12 million individual queries with
~5 bulk queries + in-memory lookups.

The `ReadinessDataStore` interface gains new bulk methods. The existing
per-item methods remain unchanged (they are used by other callers and tests).

## Data to Bulk-Load

All data is loaded once per `EvaluateOrganisation` call, scoped to the
target chef versions being evaluated.

### 1. Git Repos (all rows — small table)

- **Query**: `SELECT <columns> FROM git_repos`
- **Map**: `map[string]datastore.GitRepo` keyed by `name`
- **Interface method**: `ListAllGitRepos(ctx) ([]datastore.GitRepo, error)`
- **Note**: Already exists in datastore — verify it returns all fields needed
  (`ID`, `Name`, `HeadCommitSHA`).

### 2. Git Repo Test Kitchen Results

- **Query**: Load latest TK result per `(git_repo_id, target_chef_version)`.
  Use `DISTINCT ON (git_repo_id, target_chef_version) ORDER BY started_at DESC`
  to get only the latest per combination.
- **Filter**: `target_chef_version IN ($targetVersions)`
- **Map**: `map[string]*datastore.GitRepoTestKitchenResult` keyed by
  `gitRepoID + "|" + targetChefVersion`
- **Interface method**:
  `ListGitRepoTestKitchenResults(ctx, targetChefVersions []string) ([]datastore.GitRepoTestKitchenResult, error)`

### 3. Git Repo CookStyle Results

- **Query**: `SELECT <columns> FROM git_repo_cookstyle_results WHERE target_chef_version IN ($targetVersions)`
- **Map**: `map[string]*datastore.GitRepoCookstyleResult` keyed by
  `gitRepoID + "|" + targetChefVersion`
- **Interface method**:
  `ListGitRepoCookstyleResults(ctx, targetChefVersions []string) ([]datastore.GitRepoCookstyleResult, error)`

### 4. Server Cookbook CookStyle Results

- **Query**: `SELECT <columns> FROM server_cookbook_cookstyle_results WHERE server_cookbook_id IN (SELECT id FROM server_cookbooks WHERE organisation_id = $orgID) AND (target_chef_version IN ($targetVersions) OR target_chef_version IS NULL)`
- **Scope**: Only cookbooks belonging to the organisation being evaluated.
  The `target_chef_version IS NULL` clause is needed because some server
  cookbooks are scanned without a target version profile (the existing
  per-item query has this same fallback).
- **Map**: `map[string]*datastore.ServerCookbookCookstyleResult` keyed by
  `serverCookbookID + "|" + targetChefVersion`. For rows where
  `target_chef_version IS NULL`, use the key `serverCookbookID + "|"` (empty
  target version string) to match the existing fallback lookup pattern.
- **Interface method**:
  `ListServerCookbookCookstyleResults(ctx, organisationID string, targetChefVersions []string) ([]datastore.ServerCookbookCookstyleResult, error)`

### 5. Server Cookbook Complexity

- **Query**: `SELECT <columns> FROM server_cookbook_complexity WHERE server_cookbook_id IN (SELECT id FROM server_cookbooks WHERE organisation_id = $orgID) AND target_chef_version IN ($targetVersions)`
- **Scope**: Only cookbooks belonging to the organisation being evaluated.
- **Map**: `map[string]*datastore.ServerCookbookComplexity` keyed by
  `serverCookbookID + "|" + targetChefVersion`
- **Interface method**:
  `ListServerCookbookComplexities(ctx, organisationID string, targetChefVersions []string) ([]datastore.ServerCookbookComplexity, error)`

### 6. Git Repo Complexity

- **Query**: `SELECT <columns> FROM git_repo_complexity WHERE target_chef_version IN ($targetVersions)`
- **Map**: `map[string]*datastore.GitRepoComplexity` keyed by
  `gitRepoID + "|" + targetChefVersion`
- **Interface method**:
  `ListGitRepoComplexities(ctx, targetChefVersions []string) ([]datastore.GitRepoComplexity, error)`

## Readiness Cache Type

A new unexported struct holds all pre-loaded data:

```
type readinessCache struct {
    gitRepos         map[string]datastore.GitRepo                       // name → repo
    tkResults        map[string]*datastore.GitRepoTestKitchenResult     // gitRepoID|target → result
    gitCSResults     map[string]*datastore.GitRepoCookstyleResult       // gitRepoID|target → result
    serverCSResults  map[string]*datastore.ServerCookbookCookstyleResult // cookbookID|target → result
    serverComplexity map[string]*datastore.ServerCookbookComplexity     // cookbookID|target → complexity
    gitComplexity    map[string]*datastore.GitRepoComplexity            // gitRepoID|target → complexity
}
```

The cache is built once and shared read-only across all goroutines in the
fan-out (no mutex needed — maps are safe for concurrent reads).

## Changes to `EvaluateOrganisation`

Current flow:
1. Load node snapshots
2. Load cookbook ID map
3. Fan out work items — each calls `evaluateOne(ctx, snapshot, target, cookbookIDMap)`

New flow:
1. Load node snapshots
2. Load cookbook ID map
3. **Bulk-load all lookup data into `readinessCache`**
4. Fan out work items — each calls `evaluateOne(snapshot, target, cookbookIDMap, cache)`

The `ctx` parameter is no longer passed to `evaluateOne`,
`evaluateCookbooks`, or `checkCookbookCompatibility` because they no longer
make database calls. This eliminates the risk of context cancellation
corrupting in-flight evaluations.

## Changes to `evaluateOne` / `evaluateCookbooks` / `checkCookbookCompatibility`

These methods change from taking `ctx context.Context` + doing DB lookups via
`e.db` to taking a `*readinessCache` parameter + doing map lookups. The
logic (compatibility determination, verdict building, complexity enrichment)
remains identical — only the data source changes.

## Changes to `ReadinessDataStore` Interface

Six new methods are added:

- `ListAllGitRepos(ctx context.Context) ([]datastore.GitRepo, error)`
- `ListGitRepoTestKitchenResults(ctx context.Context, targetChefVersions []string) ([]datastore.GitRepoTestKitchenResult, error)`
- `ListGitRepoCookstyleResults(ctx context.Context, targetChefVersions []string) ([]datastore.GitRepoCookstyleResult, error)`
- `ListServerCookbookCookstyleResults(ctx context.Context, organisationID string, targetChefVersions []string) ([]datastore.ServerCookbookCookstyleResult, error)`
- `ListServerCookbookComplexities(ctx context.Context, organisationID string, targetChefVersions []string) ([]datastore.ServerCookbookComplexity, error)`
- `ListGitRepoComplexities(ctx context.Context, targetChefVersions []string) ([]datastore.GitRepoComplexity, error)`

The existing per-item methods remain on the interface (they are still used
for persistence via `UpsertNodeReadiness` and may be used by other callers).

## Bulk Query Considerations

### Excluding Heavy Columns

The CookStyle result tables contain heavy columns (`process_stdout`,
`process_stderr`, `offences`, `deprecation_warnings`) that can be KB-sized
per row. The readiness evaluator only needs `passed` (bool), `git_repo_id` /
`server_cookbook_id`, and `target_chef_version`.

The bulk queries must **exclude** heavy columns. Use explicit column lists
in the SELECT, not `SELECT *`. The returned structs will have zero values
for the excluded fields, which is fine — the evaluator never reads them.

Similarly for TK results: only `git_repo_id`, `target_chef_version`,
`compatible`, `commit_sha`, and `started_at` (for ordering) are needed.
Exclude `process_stdout`, `process_stderr`, `converge_output`,
`verify_output`, `destroy_output`.

Similarly for complexity: all fields are small, no exclusions needed.

### Target Version Filtering

All bulk queries filter by `target_chef_version IN (...)` to avoid loading
results for versions we are not evaluating. Typically 1–2 target versions.

### Organisation Scoping

Server cookbook results and complexity are scoped to the organisation via a
subquery on `server_cookbooks.organisation_id`. Git repo data is global
(not org-scoped) because git repos are shared across organisations.

## Performance Impact

| Metric | Before | After |
|--------|--------|-------|
| DB queries per evaluation run | ~12,000,000 | ~7 |
| Evaluation time (estimated) | Hours | Minutes |
| Memory overhead | Minimal | Bounded by result count (typically <10K rows total across all tables) |
| Concurrency safety | DB connection pool contention | Maps are concurrent-read safe, no contention |

## Test Changes

### Fake DataStore

The `fakeReadinessDS` in `readiness_test.go` gains implementations of the
six new interface methods. These return the same data already stored in the
fake's maps, just as slices instead of individual lookups.

### Existing Tests

All existing tests continue to work — the evaluator's external behaviour
(inputs, outputs, persistence) is unchanged. Only the internal data access
pattern changes.

### New Tests

- `TestEvaluateOrganisation_BulkLoadError` — verify that a failure in any
  bulk-load query returns an error and does not proceed with partial data.
- `TestEvaluateOrganisation_EmptyCache` — verify correct behaviour when bulk
  queries return no results (all cookbooks show as untested).
- `TestBuildReadinessCache` — unit test the cache construction from raw
  slices, verifying correct map keys and pointer semantics.

## Files Changed

| File | Change |
|------|--------|
| `internal/analysis/readiness.go` | Add `readinessCache` type, `buildReadinessCache` function, refactor `EvaluateOrganisation` to bulk-load, refactor `evaluateOne`/`evaluateCookbooks`/`checkCookbookCompatibility` to use cache |
| `internal/analysis/readiness_test.go` | Add bulk methods to `fakeReadinessDS`, add new tests |
| `internal/datastore/git_repos.go` | Add `ListAllGitRepos` (may already exist — verify) |
| `internal/datastore/git_repo_test_kitchen_results.go` | Add `ListGitRepoTestKitchenResults` |
| `internal/datastore/git_repo_cookstyle_results.go` | Add `ListGitRepoCookstyleResults` |
| `internal/datastore/server_cookbook_cookstyle_results.go` | Add `ListServerCookbookCookstyleResults` |
| `internal/datastore/server_cookbook_complexity.go` | Add `ListServerCookbookComplexities` |
| `internal/datastore/git_repo_complexity.go` | Add `ListGitRepoComplexities` |