# ToDo — Refactor: Split `cookbooks` into `server_cookbooks` and `git_repos`

Status key: [ ] Not started | [~] In progress | [x] Done

## Working Instructions

- All work for this refactor MUST be done on a single branch called `major-refactor`.
- Do NOT merge `major-refactor` to `main` until the refactor has been confirmed as complete and working.
- Check off each todo item (change `[ ]` to `[x]`) as it gets completed and committed.

## Motivation

The current `cookbooks` table conflates two fundamentally different entities behind a `source` discriminator column. This results in null-heavy rows, two partial unique indexes, constant `IsGit()`/`IsChefServer()` branching, `UNION ALL` queries, and a single FK that means different things depending on the source. Splitting makes each entity clean, self-contained, and independently evolvable.

## Design Decisions

| Decision | Choice |
|----------|--------|
| Existing data | DB recreated from scratch — no data migration |
| Migrations | Fresh single `0001_initial_schema.up.sql` |
| Analysis result tables | Separate tables per source |
| API endpoints | Keep `/api/v1/cookbooks` for server cookbooks, add new `/api/v1/git-repos` for git repos |
| Frontend pages | Keep existing Cookbooks page for server cookbooks, add a new Git Repos page |
| Rollout | Big-bang, single branch, individual commits per logical unit |
| Git repo org scoping | Not org-scoped (matched by name across orgs, as today) |
| New server cookbook metadata | Capture `frozen`, `maintainer`, `description`, `license`, `platforms`, `dependencies`, `long_description` from the Chef API cookbook version response |
| Polymorphism strategy | Option A — duplicate functions per source (`ScanServerCookbooks` + `ScanGitRepos`, etc.), no shared interface. Each persists to its own table, so separate signatures are cleaner than a `CookbookIdentity` interface. |
| Callback split | `cookbookDirFn` becomes **two** callbacks: `serverCookbookDirFn func(ServerCookbook) string` + `gitRepoDirFn func(GitRepo) string`, since both pipelines use it independently |

---

## New Server Cookbook Metadata

The Chef API `GET /cookbooks/{name}/{version}` response includes two sources of new data:

1. **Top-level `frozen?` key** — currently silently dropped because `CookbookVersionManifest` has no field for it
2. **`metadata` object** — already captured as `json.RawMessage` but never persisted. Contains: `maintainer`, `description`, `license`, `platforms`, `dependencies`, `long_description`

### Changes to `CookbookVersionManifest`

```go
type CookbookVersionManifest struct {
    // ... existing fields ...
    Frozen   bool            `json:"frozen?"`    // NEW — top-level key
    Metadata json.RawMessage `json:"metadata"`   // existing, now also persisted
}
```

### New Parsed Metadata Type

```go
// CookbookMetadata holds the fields we extract from the Chef API metadata blob.
type CookbookMetadata struct {
    Maintainer      string            `json:"maintainer"`
    Description     string            `json:"description"`
    LongDescription string            `json:"long_description"`
    License         string            `json:"license"`
    Platforms       map[string]string `json:"platforms"`       // e.g. {"ubuntu": ">= 18.04", "centos": ">= 7.0"}
    Dependencies    map[string]string `json:"dependencies"`    // e.g. {"apt": ">= 0.0.0", "yum": "~> 5.0"}
}
```

These fields will be persisted on the `server_cookbooks` table and populated during the streaming pipeline (Step 7b) when the manifest is fetched.

---

## New Database Schema

### `server_cookbooks`

```sql
CREATE TABLE server_cookbooks (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id   UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    name              TEXT        NOT NULL,
    version           TEXT        NOT NULL,
    is_active         BOOLEAN     NOT NULL DEFAULT FALSE,
    is_stale_cookbook  BOOLEAN     NOT NULL DEFAULT FALSE,
    is_frozen         BOOLEAN     NOT NULL DEFAULT FALSE,
    download_status   TEXT        NOT NULL DEFAULT 'pending',
    download_error    TEXT,
    maintainer        TEXT,
    description       TEXT,
    long_description  TEXT,
    license           TEXT,
    platforms         JSONB,
    dependencies      JSONB,
    first_seen_at     TIMESTAMPTZ,
    last_fetched_at   TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_sc_download_status CHECK (download_status IN ('ok', 'failed', 'pending')),
    UNIQUE (organisation_id, name, version)
);
```

### `git_repos`

```sql
CREATE TABLE git_repos (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT        NOT NULL,        -- cookbook name
    git_repo_url      TEXT        NOT NULL,
    head_commit_sha   TEXT,
    default_branch    TEXT,
    has_test_suite    BOOLEAN     NOT NULL DEFAULT FALSE,
    last_fetched_at   TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (name, git_repo_url)
);
```

### Split Analysis Result Tables

```sql
-- Server cookbook analysis results
CREATE TABLE server_cookbook_cookstyle_results (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_cookbook_id    UUID NOT NULL REFERENCES server_cookbooks(id) ON DELETE CASCADE,
    target_chef_version  TEXT NOT NULL,
    passed               BOOLEAN NOT NULL DEFAULT FALSE,
    offence_count        INT NOT NULL DEFAULT 0,
    deprecation_count    INT NOT NULL DEFAULT 0,
    correctness_count    INT NOT NULL DEFAULT 0,
    deprecation_warnings JSONB,
    offences             JSONB,
    process_stdout       TEXT,
    process_stderr       TEXT,
    duration_seconds     INT NOT NULL DEFAULT 0,
    scanned_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_cookbook_id, target_chef_version)
);

CREATE TABLE server_cookbook_autocorrect_previews (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_cookbook_id    UUID NOT NULL REFERENCES server_cookbooks(id) ON DELETE CASCADE,
    cookstyle_result_id  UUID NOT NULL REFERENCES server_cookbook_cookstyle_results(id) ON DELETE CASCADE,
    total_offenses       INT NOT NULL DEFAULT 0,
    correctable_offenses INT NOT NULL DEFAULT 0,
    remaining_offenses   INT NOT NULL DEFAULT 0,
    files_modified       INT NOT NULL DEFAULT 0,
    diff_output          TEXT,
    generated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cookstyle_result_id)
);

CREATE TABLE server_cookbook_complexity (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_cookbook_id      UUID NOT NULL REFERENCES server_cookbooks(id) ON DELETE CASCADE,
    target_chef_version    TEXT NOT NULL,
    complexity_score       INT NOT NULL DEFAULT 0,
    complexity_label       TEXT NOT NULL DEFAULT 'none',
    -- ... (remaining score/blast-radius columns same as current)
    UNIQUE (server_cookbook_id, target_chef_version)
);

-- Git repo analysis results
CREATE TABLE git_repo_cookstyle_results (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_id          UUID NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    target_chef_version  TEXT NOT NULL,
    commit_sha           TEXT,
    passed               BOOLEAN NOT NULL DEFAULT FALSE,
    offence_count        INT NOT NULL DEFAULT 0,
    deprecation_count    INT NOT NULL DEFAULT 0,
    correctness_count    INT NOT NULL DEFAULT 0,
    deprecation_warnings JSONB,
    offences             JSONB,
    process_stdout       TEXT,
    process_stderr       TEXT,
    duration_seconds     INT NOT NULL DEFAULT 0,
    scanned_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (git_repo_id, target_chef_version)
);

CREATE TABLE git_repo_test_kitchen_results (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_id          UUID NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    target_chef_version  TEXT NOT NULL,
    commit_sha           TEXT NOT NULL,
    -- ... (remaining columns identical to current test_kitchen_results)
    UNIQUE (git_repo_id, target_chef_version, commit_sha)
);

CREATE TABLE git_repo_autocorrect_previews (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_id          UUID NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    cookstyle_result_id  UUID NOT NULL REFERENCES git_repo_cookstyle_results(id) ON DELETE CASCADE,
    total_offenses       INT NOT NULL DEFAULT 0,
    correctable_offenses INT NOT NULL DEFAULT 0,
    remaining_offenses   INT NOT NULL DEFAULT 0,
    files_modified       INT NOT NULL DEFAULT 0,
    diff_output          TEXT,
    generated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cookstyle_result_id)
);

CREATE TABLE git_repo_complexity (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_id            UUID NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    target_chef_version    TEXT NOT NULL,
    complexity_score       INT NOT NULL DEFAULT 0,
    complexity_label       TEXT NOT NULL DEFAULT 'none',
    -- ... (remaining score/blast-radius columns same as current)
    UNIQUE (git_repo_id, target_chef_version)
);
```

### `cookbook_node_usage` — Renamed FK

```sql
CREATE TABLE cookbook_node_usage (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_cookbook_id UUID NOT NULL REFERENCES server_cookbooks(id) ON DELETE CASCADE,
    node_snapshot_id  UUID NOT NULL REFERENCES node_snapshots(id) ON DELETE CASCADE,
    cookbook_version   TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Tables With No Schema Change

These reference cookbooks by name (strings) or JSONB, not FKs:

| Table | Cookbook Reference | Change? |
|-------|-------------------|---------|
| `cookbook_usage_analysis` / `cookbook_usage_detail` | `cookbook_name`, `cookbook_version` TEXT | **No** |
| `cookbook_role_usage` | `cookbook_name` TEXT | **No** |
| `role_dependencies` | `dependency_name` TEXT when `dependency_type='cookbook'` | **No** |
| `node_readiness` | `blocking_cookbooks` JSONB | **No** |
| `node_snapshots` | `cookbooks` JSONB blob | **No** |
| `git_repo_committers` | `git_repo_url` TEXT | **No** (optionally add FK to `git_repos` later) |

---

## Go Type Changes

### New Types

```go
// internal/datastore/server_cookbooks.go

type ServerCookbook struct {
    ID              string          `json:"id"`
    OrganisationID  string          `json:"organisation_id"`
    Name            string          `json:"name"`
    Version         string          `json:"version"`
    IsActive        bool            `json:"is_active"`
    IsStaleCookbook bool            `json:"is_stale_cookbook"`
    IsFrozen        bool            `json:"is_frozen"`
    DownloadStatus  string          `json:"download_status"`
    DownloadError   string          `json:"download_error,omitempty"`
    Maintainer      string          `json:"maintainer,omitempty"`
    Description     string          `json:"description,omitempty"`
    LongDescription string          `json:"long_description,omitempty"`
    License         string          `json:"license,omitempty"`
    Platforms       json.RawMessage `json:"platforms,omitempty"`
    Dependencies    json.RawMessage `json:"dependencies,omitempty"`
    FirstSeenAt     time.Time       `json:"first_seen_at,omitempty"`
    LastFetchedAt   time.Time       `json:"last_fetched_at,omitempty"`
    CreatedAt       time.Time       `json:"created_at"`
    UpdatedAt       time.Time       `json:"updated_at"`
}
```

```go
// internal/datastore/git_repos.go

type GitRepo struct {
    ID            string    `json:"id"`
    Name          string    `json:"name"`
    GitRepoURL    string    `json:"git_repo_url"`
    HeadCommitSHA string    `json:"head_commit_sha,omitempty"`
    DefaultBranch string    `json:"default_branch,omitempty"`
    HasTestSuite  bool      `json:"has_test_suite"`
    LastFetchedAt time.Time `json:"last_fetched_at,omitempty"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}
```

### Deleted

- `datastore.Cookbook` — replaced by `ServerCookbook` and `GitRepo`
- `IsGit()`, `IsChefServer()`, `IsDownloaded()`, `NeedsDownload()` helpers
- `Source` field and `chk_cookbooks_source` CHECK constraint

### Chef API Metadata Extraction

```go
// internal/chefapi/client.go — add to CookbookVersionManifest

type CookbookVersionManifest struct {
    // ... existing fields ...
    Frozen   bool            `json:"frozen?"`
    Metadata json.RawMessage `json:"metadata"`
}

// CookbookMetadata holds the fields we extract from the metadata blob.
type CookbookMetadata struct {
    Maintainer      string            `json:"maintainer"`
    Description     string            `json:"description"`
    LongDescription string            `json:"long_description"`
    License         string            `json:"license"`
    Platforms       map[string]string `json:"platforms"`
    Dependencies    map[string]string `json:"dependencies"`
}
```

### Metadata Population Flow

The metadata fields are **not** available from `GET /cookbooks?num_versions=all` (Step 5 — inventory only returns names and versions). They come from `GET /cookbooks/{name}/{version}` which is called in the **streaming pipeline** (Step 7b) via `downloadToTempDir` → `GetCookbookVersionManifest`. The flow:

1. Step 5–6: Upsert `server_cookbooks` rows with name/version/is_active (metadata columns NULL)
2. Step 7b: For each cookbook needing download, fetch manifest → parse `Metadata` and `Frozen` → update the `server_cookbooks` row with the extracted metadata fields → download files to temp dir → scan → delete

This means a new datastore method like `UpdateServerCookbookMetadata(id, params)` is needed, called from the pipeline after the manifest is fetched.

---

## Commit Plan

Individual commits on a single branch, in dependency order:

### Commit 1 — Migrations
- [x] Delete all 11 existing migration files (`0001` through `0011`)
- [x] Write new `0001_initial_schema.up.sql` — all 25+ tables from scratch, incorporating the split
- [x] Write new `0001_initial_schema.down.sql` — `DROP TABLE` in reverse dependency order

### Commit 2 — Chef API Types
- [x] Add `Frozen bool` field (`json:"frozen?"`) to `CookbookVersionManifest` in `internal/chefapi/client.go`
- [x] Add `CookbookMetadata` type in `internal/chefapi/client.go`
- [x] Add `ParseMetadata()` method on `CookbookVersionManifest`

### Commit 3 — Datastore: `server_cookbooks` CRUD
- [x] Create `internal/datastore/server_cookbooks.go` with `ServerCookbook` type
- [x] Implement CRUD: `UpsertServerCookbook`, `ListServerCookbooksByOrganisation`, `GetServerCookbook`, `UpdateServerCookbookDownloadStatus`, `UpdateServerCookbookMetadata`, `DeleteServerCookbook`, `MarkStaleServerCookbooks`

### Commit 4 — Datastore: `git_repos` CRUD
- [x] Create `internal/datastore/git_repos.go` with `GitRepo` type
- [x] Implement CRUD: `UpsertGitRepo`, `ListGitRepos`, `GetGitRepo`, `DeleteGitReposByName`

### Commit 5 — Datastore: Split `cookstyle_results`
- [x] Create `internal/datastore/server_cookbook_cookstyle_results.go`
- [x] Create `internal/datastore/git_repo_cookstyle_results.go`
- [x] Delete old `internal/datastore/cookstyle_results.go`

### Commit 6 — Datastore: Split `test_kitchen_results`
- [x] Create `internal/datastore/git_repo_test_kitchen_results.go`
- [x] Delete/refactor old `internal/datastore/test_kitchen_results.go`

### Commit 7 — Datastore: Split `autocorrect_previews`
- [x] Create `internal/datastore/server_cookbook_autocorrect_previews.go`
- [x] Create `internal/datastore/git_repo_autocorrect_previews.go`
- [x] Delete old `internal/datastore/autocorrect_previews.go`

### Commit 8 — Datastore: Split `cookbook_complexity`
- [x] Create `internal/datastore/server_cookbook_complexity.go`
- [x] Create `internal/datastore/git_repo_complexity.go`
- [x] Delete old `internal/datastore/cookbook_complexity.go`

### Commit 9 — Datastore: Update `cookbook_node_usage`
- [x] Refactor `internal/datastore/cookbook_node_usage.go` — rename `CookbookID` → `ServerCookbookID`

### Commit 10 — Datastore: Delete Unified `cookbooks.go`
- [x] Delete `internal/datastore/cookbooks.go`
- [x] Remove `Cookbook` type, `IsGit()`, `IsChefServer()`, `IsDownloaded()`, `NeedsDownload()` helpers

### Commit 11 — Collector: Server Cookbook Pipeline + Metadata
- [x] Update `internal/collector/server_cookbook_pipeline.go` to work with `ServerCookbook`
- [x] After `GetCookbookVersionManifest`, parse metadata and call `UpdateServerCookbookMetadata`
- [x] `downloadToTempDir` takes `ServerCookbook`

### Commit 12 — Collector: Git Repos
- [x] Refactor `internal/collector/git.go`: `fetchGitCookbooks` → `fetchGitRepos`
- [x] `UpsertGitCookbook` → `UpsertGitRepo`
- [x] Work with `GitRepo` type throughout

### Commit 13 — Collector: Orchestration
- [x] Update `internal/collector/collector.go`: split `cookbookDirFn func(datastore.Cookbook) string` into two callbacks: `serverCookbookDirFn func(datastore.ServerCookbook) string` + `gitRepoDirFn func(datastore.GitRepo) string`
- [x] Update `WithCookbookDirFn` option → `WithServerCookbookDirFn` + `WithGitRepoDirFn`
- [x] Remove source branching — each step already targets one source
- [x] Update `cbByID` map used for autocorrect preview dir resolution to use `GitRepo` type

### Commit 14 — Collector: Ownership + Ownership Assignments
- [x] Update `internal/collector/ownership.go`: `evaluateCookbookNamePatternRule` queries `ListServerCookbooksByOrganisation` + `ListGitRepos`
- [x] `evaluateGitRepoURLPatternRule` queries `ListGitRepos`
- [x] Update `internal/datastore/ownership_assignments.go`: `lookupGitRepoInheritedOwnership` — change query from `FROM cookbooks WHERE name = $1 AND source = 'git'` to `FROM git_repos WHERE name = $1`

### Commit 15 — Analysis: Split Cookstyle Scanner
- [x] Split `internal/analysis/cookstyle.go`: `ScanCookbooks` → `ScanServerCookbooks` + `ScanGitRepos`
- [x] Split `ScanSingleCookbook` → `ScanSingleServerCookbook` + `ScanSingleGitRepo` (called from different pipelines)
- [x] Split `scanOne` → two variants with source-specific `workItem` structs
- [x] `persistResult` → two variants for different tables
- [x] Split `cookbookDir` callback parameter into source-specific callbacks
- [x] Skip logic becomes first-class (immutable version vs HEAD SHA)

### Commit 16 — Analysis: Readiness Evaluator
- [x] Update `internal/analysis/readiness.go`: `checkCookbookCompatibility` queries both cookstyle result tables
- [x] `ReadinessDataStore` interface gets source-specific lookup methods

### Commit 17 — Analysis: Kitchen Scanner
- [x] Update `internal/analysis/kitchen.go` to accept `GitRepo` instead of `Cookbook`
- [x] Persist to `git_repo_test_kitchen_results`

### Commit 18 — Remediation: Autocorrect + Complexity
- [x] Split `internal/remediation/autocorrect.go`: `GeneratePreviews` / `GenerateSinglePreview` parameterised by source
- [x] `persistPreview` writes to the correct table
- [x] Split `internal/remediation/complexity.go`: `ScoreCookbooks` → `ScoreServerCookbooks` + `ScoreGitRepos`
- [x] Persist to source-specific complexity tables
- [x] `loadBlastRadii` is name-based — no change
- [x] Split `cookbookDir` callback parameter into source-specific callbacks

### Commit 19 — Export Layer
- [x] Update `internal/export/export.go`: `DataStore` interface updated, `ListCookbooksByOrganisation` → `ListServerCookbooksByOrganisation` + `ListGitRepos`
- [x] Update `internal/export/cookbook_remediation.go`: Build two maps from both sources, join with source-specific complexity tables
- [x] Update `internal/export/blocked_nodes.go`: Build unified name-keyed maps from both `ServerCookbook` and `GitRepo`

### Commit 20 — Web API: Split Handlers and Routes
- [x] Update `internal/webapi/router.go`: keep `/api/v1/cookbooks` for server cookbooks, add `/api/v1/git-repos` routes
- [x] Refactor `internal/webapi/handle_cookbooks.go` to serve only server cookbooks (remove git cookbook logic and collapse-by-name merging)
- [x] Create new `internal/webapi/handle_git_repos.go` for git repo list, detail, rescan, reset
- [x] Refactor `internal/webapi/handle_cookbook_rescan.go` for server cookbooks only
- [x] Refactor `internal/webapi/handle_cookbook_remediation.go` for server cookbooks only
- [x] Create new `internal/webapi/handle_git_repo_remediation.go` for git repo remediation
- [x] Move `internal/webapi/handle_cookbook_reset_git.go` → `handle_git_repo_reset.go`
- [x] Move committer endpoints under `/api/v1/git-repos/:name/committers`
- [x] Refactor `internal/webapi/handle_dashboard.go`: `handleDashboardCookbookCompatibility` aggregates from both sources; `handleDashboardCookbookDownloadStatus` queries `server_cookbooks` directly (remove in-Go source filter); `handleDashboardComplexityTrend` joins source-specific complexity tables
- [x] Refactor `internal/webapi/handle_admin_rescan_all.go`: `DeleteAllCookbookComplexities` → hit both complexity tables; bulk cookstyle delete → hit both result tables
- [x] Split `EventCookbookStatusChanged` into `EventServerCookbookStatusChanged` + `EventGitRepoStatusChanged` in `internal/webapi/eventhub.go`

### Commit 21 — Web API: Store Interface
- [x] Update `internal/webapi/store.go` with new method signatures

### Commit 22 — Entry Point
- [x] Update `main.go`: split `WithCookbookDirFn` closure into `WithServerCookbookDirFn` + `WithGitRepoDirFn`, remove `cb.IsGit()` branch

### Commit 23 — Frontend: Types and API Client
- [x] Update `frontend/src/types.ts`: refactor `CookbookListItem` to reflect server cookbook metadata fields (frozen, maintainer, description, license, platforms, dependencies). Add new `GitRepoListItem` type.
- [x] Split result types for server cookbook vs git repo analysis
- [x] Update `frontend/src/api.ts`: refactor `fetchCookbooks` to fetch from `/api/v1/cookbooks` (server cookbooks only). Add new `fetchGitRepos` calling `/api/v1/git-repos`.

### Commit 24 — Frontend: Pages
- [x] Refactor `frontend/src/CookbooksPage.tsx` to show server cookbooks only (remove git cookbook rows/logic). Add new metadata columns where appropriate.
- [x] Refactor `frontend/src/CookbookDetailPage.tsx` for server cookbooks only (show metadata fields: frozen, maintainer, description, license, platforms, dependencies)
- [x] Create new `frontend/src/GitReposPage.tsx` — list page for git repos
- [x] Create new `frontend/src/GitRepoDetailPage.tsx` — detail page (cookstyle + test kitchen + autocorrect)
- [x] Create new `frontend/src/GitRepoRemediationPage.tsx` — remediation for git repos
- [x] Update `frontend/src/CookbookRemediationPage.tsx` for server cookbooks only
- [x] Reuse `CookbookCommittersPage.tsx` for git repo committers route (`/git-repos/:name/committers`) — component is context-agnostic so rename was unnecessary
- [x] Update `frontend/src/DashboardPage.tsx` API calls
- [x] Update `frontend/src/App.tsx`: keep `/cookbooks` routes for server cookbooks, add new `/git-repos` routes
- [x] Add Git Repos navigation item to sidebar/nav

### Commit 25 — Tests
- [x] Regenerate or rewrite `internal/webapi/store_mock_test.go` (~136 symbols) to match the new `DataStore` interface
- [x] Update `internal/webapi/handle_cookbooks_test.go` — remove git cookbook test cases, update for `ServerCookbook` type
- [x] Update `internal/webapi/handle_cookbook_remediation_test.go` — `ListCookbooksByNameFn` → server-cookbook-only variant (~12 test functions)
- [x] Rename/refactor `internal/webapi/handle_cookbook_reset_git_test.go` → `handle_git_repo_reset_test.go`
- [x] Update `internal/export/export_test.go` — `fakeStore` uses `ServerCookbook` + `GitRepo` types
- [x] Update `internal/analysis/cookstyle_test.go` — split test cases by source
- [x] Update `internal/analysis/kitchen_test.go` — use `GitRepo` type (~8 test cases constructing `Cookbook{}` literals)
- [x] Update `internal/collector/fetcher_test.go` — remove `IsGit()`/`IsChefServer()` helper tests, update `IsDownloaded()`/`NeedsDownload()` for `ServerCookbook`, update JSON marshal tests
- [x] Update `internal/collector/server_cookbook_pipeline_test.go` — use `ServerCookbook` type
- [x] Update `internal/collector/collector_test.go` — new callback types
- [x] Update `internal/datastore/datastore_test.go` — remove `TestCookbook_SourceHelpers`
- [x] Update all remaining `*_test.go` files for split types

### Commit 26 — Documentation
- [x] Update `.claude/specifications/datastore/Specification.md` — replace section 4 (`cookbooks`) with `server_cookbooks` + `git_repos` tables and split analysis result tables
- [x] Update `.claude/specifications/data-collection/Specification.md` — section 2 ("Cookbook Fetching") clarify server vs git repo paths
- [x] Update `.claude/specifications/web-api/Specification.md` — new endpoint documentation
- [x] Update `README.md`

---

## Multi-Source Cookbook Readiness Enhancement

### Motivation

The current readiness evaluator uses a **short-circuit waterfall** to determine cookbook compatibility: it checks git repo Test Kitchen first, and if that returns any result (pass or fail), it stops. If no TK result exists, it checks server cookbook CookStyle, and stops. It never checks **both** the server version and the git version, which means:

- If the server cookbook version is incompatible but a fixed version exists in git, the operator has no visibility into this.
- Git repo CookStyle results are never checked by the readiness evaluator at all — only Test Kitchen results from git are considered.
- The blocking reason gives no guidance on what action to take (upload git version? fix the cookbook? run tests?).

### Design

Replace the short-circuit approach with a **multi-source evaluation** that checks all available sources and records per-source verdicts.

#### New Types

```go
// CookbookSourceVerdict records the compatibility result from one source.
type CookbookSourceVerdict struct {
    Source          string `json:"source"`                      // "server_cookstyle", "git_cookstyle", "git_test_kitchen"
    Status          string `json:"status"`                      // "compatible", "incompatible", "untested"
    Version         string `json:"version,omitempty"`           // server version or "HEAD" for git
    CommitSHA       string `json:"commit_sha,omitempty"`        // git HEAD SHA (git sources only)
    ComplexityScore int    `json:"complexity_score,omitempty"`
    ComplexityLabel string `json:"complexity_label,omitempty"`
}

// BlockingCookbook — updated to include per-source verdicts.
type BlockingCookbook struct {
    Name            string                  `json:"name"`
    Version         string                  `json:"version"`            // version on the node
    Reason          string                  `json:"reason"`             // overall: "incompatible" or "untested"
    Source          string                  `json:"source"`             // primary source (backward compat)
    ComplexityScore int                     `json:"complexity_score"`
    ComplexityLabel string                  `json:"complexity_label"`
    Verdicts        []CookbookSourceVerdict `json:"verdicts"`           // NEW: all sources checked
}
```

#### Algorithm Change

For each cookbook on a node, check **all three sources** independently:

1. **Git repo Test Kitchen** — query `GetLatestGitRepoTestKitchenResult(gitRepoID, targetVersion)` → record verdict
2. **Git repo CookStyle** — query `GetGitRepoCookstyleResult(gitRepoID, targetVersion)` → record verdict (currently **not queried** by readiness evaluator)
3. **Server cookbook CookStyle** — query `GetServerCookbookCookstyleResult(cookbookID, targetVersion)` → record verdict

Overall status:
- **Compatible** if **any** source says compatible (a compatible version exists somewhere)
- **Incompatible** if all tested sources say incompatible (none compatible)
- **Untested** if no sources have results

#### Readiness Policy

If the server version is incompatible but the git version is compatible, the node is considered **ready** — a compatible version exists and can be uploaded to the Chef Server. The per-source verdicts make it clear what action is needed.

#### Interface Change

Add `GetGitRepoCookstyleResult` to the `ReadinessDataStore` interface (already exists on `datastore.DB`, just not in the interface):

```go
type ReadinessDataStore interface {
    // ... existing methods ...
    GetGitRepoCookstyleResult(ctx context.Context, gitRepoID, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error)
}
```

#### Frontend Changes

Update the node detail page's **Upgrade Readiness** section to render per-source verdicts for each blocking cookbook:

- Show the cookbook name, node version, and overall verdict
- Expandable panel listing each source verdict with status icon and version/commit info
- Action hint when server is incompatible but git is compatible: *"A compatible version exists in git — upload to Chef Server to resolve."*
- Update `NodeReadiness` TypeScript type to include structured `blocking_cookbooks` with `verdicts` array

### Commit Plan

#### Commit 27 — Analysis: Multi-source cookbook readiness evaluation
- [ ] Add `CookbookSourceVerdict` type to `internal/analysis/readiness.go`
- [ ] Add `Verdicts` field to `BlockingCookbook` struct
- [ ] Add `GetGitRepoCookstyleResult` to `ReadinessDataStore` interface
- [ ] Refactor `checkCookbookCompatibility` to check all 3 sources and collect verdicts
- [ ] Update overall status logic: compatible if any source is compatible
- [ ] Update `evaluateCookbooks` to populate verdicts on each blocking entry
- [ ] Update tests in `internal/analysis/readiness_test.go`

#### Commit 28 — Web API: Return enriched readiness data
- [ ] Verify `handleNodeDetail` returns the new `verdicts` field (already passes through `blocking_cookbooks` JSONB — should Just Work if the evaluator populates it)
- [ ] Update any readiness-related API handlers that transform blocking cookbook data

#### Commit 29 — Frontend: Per-source verdicts on node detail page
- [ ] Update `NodeReadiness` TypeScript type to include structured `blocking_cookbooks` with per-source `verdicts`
- [ ] Update `NodeDetailPage.tsx` readiness section to render per-source verdicts
- [ ] Add expandable verdict panel with status icons and action hints
- [ ] Add links from verdict entries to cookbook/git-repo detail pages

#### Commit 30 — Documentation
- [x] Update `.claude/specifications/analysis/Specification.md` — multi-source algorithm
- [x] Update `.claude/specifications/web-api/Specification.md` — node detail response with verdicts
- [x] Update `.claude/specifications/visualisation/Specification.md` — per-source verdict display

### Files Affected

| File | Change |
|------|--------|
| `internal/analysis/readiness.go` | New types, refactored `checkCookbookCompatibility`, updated `BlockingCookbook` |
| `internal/analysis/readiness_test.go` | Updated tests for multi-source evaluation |
| `frontend/src/types.ts` | Updated `NodeReadiness` with structured blocking cookbooks + verdicts |
| `frontend/src/pages/NodeDetailPage.tsx` | Per-source verdict rendering, action hints |

### Risks & Mitigation

| Risk | Mitigation |
|------|-----------|
| Backward compatibility of `blocking_cookbooks` JSONB | Keep `reason` and `source` fields for backward compat; `verdicts` is additive |
| Performance — 3 DB queries per cookbook per node | Queries are simple index lookups; pre-load maps where possible (same pattern as existing code) |
| Policy change — "compatible if any source compatible" may surprise users | Verdicts make it explicit; action hints explain what to do |
| `ReadinessDataStore` interface change | Only adds one method; mock update is trivial |

---

## New API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/cookbooks` | GET | List server cookbooks (paginated, filtered) — includes metadata fields |
| `/api/v1/cookbooks/:name` | GET | Server cookbook detail (all versions + cookstyle) |
| `/api/v1/cookbooks/:name/:version/remediation` | GET | Remediation for a server cookbook version |
| `/api/v1/cookbooks/:name/rescan` | POST | Reset download status, trigger rescan |
| `/api/v1/git-repos` | GET | List git repos (paginated, filtered) |
| `/api/v1/git-repos/:name` | GET | Git repo detail (cookstyle + test kitchen + autocorrect) |
| `/api/v1/git-repos/:name/remediation` | GET | Remediation for a git repo |
| `/api/v1/git-repos/:name/rescan` | POST | Clear results, rescan on next cycle |
| `/api/v1/git-repos/:name/reset` | POST | Delete repo + clone, re-clone next cycle |
| `/api/v1/git-repos/:name/committers` | GET | List committers |
| `/api/v1/git-repos/:name/committers/assign` | POST | Assign committers as owners |
| `/api/v1/admin/rescan-all-cookstyle` | POST | Hits both result tables |
| `/api/v1/dashboard/cookbook-compatibility` | GET | Server cookbook CookStyle compatibility per target version |
| `/api/v1/dashboard/git-repo-compatibility` | GET | Git repo CookStyle compatibility per target version |
| `/api/v1/dashboard/platform-distribution` | GET | Node OS platform distribution |
| `/api/v1/dashboard/cookbook-download-status` | GET | Server cookbooks only (from `/api/v1/cookbooks`) |

---

## File-by-File Change Plan

### Migrations

| Action | Detail |
|--------|--------|
| Delete all 11 existing migration files | `0001` through `0011` |
| Write new `0001_initial_schema.up.sql` | All 25+ tables from scratch, incorporating the split |
| Write new `0001_initial_schema.down.sql` | `DROP TABLE` in reverse dependency order |

### Chef API (`internal/chefapi/`)

| File | Changes |
|------|---------|
| `client.go` | Add `Frozen bool` field to `CookbookVersionManifest`. Add `CookbookMetadata` type. Add `ParseMetadata()` method on `CookbookVersionManifest`. |

### Datastore Layer (`internal/datastore/`)

| Current File | Action | New File(s) |
|-------------|--------|------------|
| `cookbooks.go` (~940 lines) | **Delete** | `server_cookbooks.go` + `git_repos.go` |
| `cookstyle_results.go` | **Split** | `server_cookbook_cookstyle_results.go` + `git_repo_cookstyle_results.go` |
| `test_kitchen_results.go` | **Rename/refactor** | `git_repo_test_kitchen_results.go` |
| `autocorrect_previews.go` | **Split** | `server_cookbook_autocorrect_previews.go` + `git_repo_autocorrect_previews.go` |
| `cookbook_complexity.go` | **Split** | `server_cookbook_complexity.go` + `git_repo_complexity.go` |
| `cookbook_node_usage.go` | **Refactor** — `CookbookID` → `ServerCookbookID` |
| `cookbook_usage_analysis.go` | **No change** — uses cookbook name strings |
| `git_repo_committers.go` | **No change** — already git-specific |
| `ownership_assignments.go` | **Refactor** — `lookupGitRepoInheritedOwnership` queries `git_repos` instead of `cookbooks WHERE source = 'git'` |
| `node_readiness.go` | **No change** — JSONB references |
| `role_dependencies.go` | **No change** — string references |
| `node_snapshots.go` | **No change** — opaque JSONB |

### Key New Datastore Methods

| Method | Purpose |
|--------|---------|
| `UpdateServerCookbookMetadata(ctx, id, params)` | Populate frozen/maintainer/description/license/platforms/dependencies after manifest fetch |
| `UpsertGitRepo(...)` | Replaces `UpsertGitCookbook` |
| `ListGitRepos(...)` | Replaces `ListGitCookbooks` |
| `DeleteGitReposByName(...)` | Replaces `DeleteGitCookbooksByName` |

### Collector Layer (`internal/collector/`)

| File | Changes |
|------|---------|
| `collector.go` | Split `cookbookDirFn` into `serverCookbookDirFn func(datastore.ServerCookbook) string` + `gitRepoDirFn func(datastore.GitRepo) string`. Split `WithCookbookDirFn` → `WithServerCookbookDirFn` + `WithGitRepoDirFn`. Update `cbByID` map for autocorrect to use `GitRepo`. Remove source branching. |
| `git.go` | `fetchGitCookbooks` → `fetchGitRepos`. `UpsertGitCookbook` → `UpsertGitRepo`. Work with `GitRepo` type. |
| `fetcher.go` | `downloadCookbookVersion(... cb datastore.Cookbook ...)` → `downloadServerCookbook(... sc datastore.ServerCookbook ...)`. |
| `server_cookbook_pipeline.go` | Works with `ServerCookbook`. After `GetCookbookVersionManifest`, parse metadata and call `UpdateServerCookbookMetadata`. `downloadToTempDir` takes `ServerCookbook`. |
| `ownership.go` | `evaluateCookbookNamePatternRule` queries `ListServerCookbooksByOrganisation` + `ListGitRepos`. `evaluateGitRepoURLPatternRule` queries `ListGitRepos`. |
| `runlist.go` | **No change** |
| `scheduler.go` | **No change** |

### Analysis Layer (`internal/analysis/`)

| File | Changes |
|------|---------|
| `cookstyle.go` | Split `ScanCookbooks` → `ScanServerCookbooks` + `ScanGitRepos`. Split `ScanSingleCookbook` → `ScanSingleServerCookbook` + `ScanSingleGitRepo`. Split `scanOne` → two variants with source-specific `workItem` structs. `persistResult` → two variants. Split `cookbookDir` callback. Skip logic becomes first-class (immutable version vs HEAD SHA). |
| `readiness.go` | `checkCookbookCompatibility` queries both cookstyle result tables. `ReadinessDataStore` interface gets source-specific lookup methods. |
| `usage.go` | **Minimal** — works from `NodeRecord`/string data, not cookbook entities. |
| `kitchen.go` | Accepts `GitRepo` instead of `Cookbook`. Persists to `git_repo_test_kitchen_results`. |

### Remediation Layer (`internal/remediation/`)

| File | Changes |
|------|---------|
| `autocorrect.go` | `CookstyleResultInfo` gets source awareness. `persistPreview` writes to the correct table. `GeneratePreviews` / `GenerateSinglePreview` parameterised by source. |
| `complexity.go` | `ScoreCookbooks` → `ScoreServerCookbooks` + `ScoreGitRepos`. Persist to source-specific complexity tables. `loadBlastRadii` is name-based — no change. |
| `copmapping.go` | **No change** — pure static mapping. |

### Export Layer (`internal/export/`)

| File | Changes |
|------|---------|
| `export.go` | `DataStore` interface updated. `ListCookbooksByOrganisation` → `ListServerCookbooksByOrganisation` + `ListGitRepos`. |
| `cookbook_remediation.go` | Build two maps from both sources, join with source-specific complexity tables. |
| `blocked_nodes.go` | Build unified name-keyed maps from both `ServerCookbook` and `GitRepo`. |
| `ready_nodes.go` | **No change** |
| `filter.go` | **No change** (optionally add source filter later) |

### Web API (`internal/webapi/`)

| File | Changes |
|------|---------|
| `router.go` | Keep `/api/v1/cookbooks` for server cookbooks. Add `/api/v1/git-repos` group. Move committer endpoints under git-repos. |
| `handle_cookbooks.go` | **Refactor** — serve server cookbooks only, remove git cookbook logic and collapse-by-name merging. |
| `handle_git_repos.go` | **New** — list, detail, rescan handlers for git repos. |
| `handle_cookbook_rescan.go` | **Refactor** — server cookbooks only. |
| `handle_cookbook_remediation.go` | **Refactor** — server cookbooks only. |
| `handle_git_repo_remediation.go` | **New** — remediation handler for git repos. |
| `handle_cookbook_reset_git.go` | **Rename** to `handle_git_repo_reset.go`. |
| `handle_dashboard.go` | **Refactor** — `handleDashboardCookbookCompatibility` aggregates both sources; `handleDashboardCookbookDownloadStatus` queries `server_cookbooks` directly (remove in-Go source filter); `handleDashboardComplexityTrend` joins source-specific complexity tables. |
| `handle_admin_rescan_all.go` | **Refactor** — bulk delete hits both cookstyle result tables and both complexity tables. |
| `eventhub.go` | **Refactor** — consider splitting `EventCookbookStatusChanged` into server + git variants. |
| `store.go` | Updated method signatures. |
| `store_mock_test.go` | **Regenerate** — ~136 symbols must match new `DataStore` interface. |

### Frontend (`frontend/src/`)

| File | Changes |
|------|---------|
| `types.ts` | Refactor `CookbookListItem` for server cookbook metadata fields. Add new `GitRepoListItem`. Split result types. |
| `api.ts` | Refactor `fetchCookbooks` to hit `/api/v1/cookbooks` (server cookbooks only). Add `fetchGitRepos` hitting `/api/v1/git-repos`. |
| `CookbooksPage.tsx` | **Refactor** — server cookbooks only. Remove git cookbook rows/logic. Add metadata columns. |
| `CookbookDetailPage.tsx` | **Refactor** — server cookbooks only. Show metadata (frozen, maintainer, description, license, platforms, dependencies). |
| `CookbookRemediationPage.tsx` | **Refactor** — server cookbooks only. |
| `GitReposPage.tsx` | **New** — list page for git repos. |
| `GitRepoDetailPage.tsx` | **New** — detail page (cookstyle + test kitchen + autocorrect). |
| `GitRepoRemediationPage.tsx` | **New** — remediation for git repos. |
| `CookbookCommittersPage.tsx` | **Rename** to `GitRepoCommittersPage.tsx`. Move under git repo context. |
| `DashboardPage.tsx` | Update API calls. |
| `App.tsx` | Keep `/cookbooks` routes for server cookbooks. Add `/git-repos` routes. Add nav item. |

### Entry Point

| File | Changes |
|------|---------|
| `main.go` | Split `WithCookbookDirFn` closure → `WithServerCookbookDirFn` + `WithGitRepoDirFn`. Remove `cb.IsGit()` branch. |

### Configuration

| File | Changes |
|------|---------|
| `config.go` | **Mostly no change** — already has separate `CookbookCacheDir` / `GitCookbookDir`. **Review** `NotificationChannelFilter.Cookbooks` — post-split, name-based filtering must query both `server_cookbooks` and `git_repos` to match by name. |

---

## Risks & Mitigation

| Risk | Mitigation |
|------|-----------|
| Large blast radius (every layer touched) | Individual commits allow rollback to any point |
| Readiness evaluator crosses source boundary | Thin adapter querying both cookstyle tables, returning unified compatibility status |
| Complexity scorer needs both sources | Separate method calls: `ScoreServerCookbooks` + `ScoreGitRepos` |
| Export joins across ID spaces | Build unified name-keyed maps from both sources before joining |
| Frontend page duplication | Cookbooks page stays for server cookbooks; new dedicated Git Repos page avoids confusion. Shared components (e.g. cookstyle result cards) reused across both. |
| Metadata not available at inventory time | Two-phase: upsert skeleton in Step 6, populate metadata in Step 7b |
| `ScanSingleCookbook` called from both pipelines | Split into `ScanSingleServerCookbook` + `ScanSingleGitRepo` — each persists to its own table, no shared interface needed |
| `cookbookDirFn` callback used polymorphically | Split into two callbacks (`serverCookbookDirFn` + `gitRepoDirFn`), wired separately in `main.go` |
| `ListTestKitchenResultsForOrganisation` cross-source JOIN | Currently correlates git + server cookbooks in one query — after split, needs cross-table subquery (`git_repos.name IN (SELECT name FROM server_cookbooks WHERE organisation_id = $1)`) |
| Dashboard handlers query unified table | `handle_dashboard.go` has 3 handlers querying cookbooks — must be refactored for split tables |
| `ownership_assignments.go` queries `cookbooks` table | `lookupGitRepoInheritedOwnership` must query `git_repos` instead |
| `NotificationChannelFilter.Cookbooks` config | Name-based filter must match against both `server_cookbooks` and `git_repos` — behavioural decision needed |
| Test mock blast radius | `store_mock_test.go` (~136 symbols) must be regenerated; ~8 test files construct `Cookbook{}` literals |