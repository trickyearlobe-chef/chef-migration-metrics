# Data Collection - Component Specification

> **Implementation language:** Go. See `../../Claude.md` for language and concurrency rules.

> Component specification for the Data Collection component of Chef Migration Metrics.
> See the [top-level specification](../Specification.md) for project overview and scope.

---

## TL;DR

Periodic background job collects node data from Chef Infra Server orgs via **partial search** and fetches cookbooks from **git repos** and/or the **Chef server**. Supports multiple orgs (parallel), Policyfile nodes (`policy_name`/`policy_group`), stale node detection (`ohai_time`), stale cookbook detection (`first_seen_at`), and role dependency graph building. Key config: `collection.schedule`, `collection.stale_node_threshold_days` (7), `collection.stale_cookbook_threshold_days` (365). Concurrency is bounded per task type. Related specs: `chef-api/`, `configuration/`, `datastore/`, `logging/`.

---

## Overview

The Data Collection component is responsible for:

1. Periodically collecting node data from one or more Chef Infra Server organisations using the Chef Infra Server API
2. Fetching cookbooks in active use from git repositories and/or directly from the Chef Infra Server

All Chef Infra Server API interactions must conform to [`../chef-api/Specification.md`](../chef-api/Specification.md).

---

## 1. Node Collection

### 1.1 Scheduling

- Node collection runs as a **periodic background job**.
- The collection interval is configurable (see [Configuration Specification](../configuration/Specification.md)).
- Each collection run is assigned a unique run ID and logged (see [Logging Specification](../logging/Specification.md)).

### 1.2 Multi-Organisation Support

- Multiple Chef Infra Server organisations must be supported.
- Each organisation is independently configured with its own Chef server URL, organisation name, client name, and private key path.
- Each organisation is collected independently. A failure collecting from one organisation must not prevent collection from continuing for others.
- Organisations must be collected **in parallel** using goroutines — one goroutine per organisation. An `errgroup` or equivalent must be used to coordinate goroutines and aggregate errors without cancelling successful collections.
- Concurrency must be bounded by the `concurrency.organisation_collection` worker pool setting (see [Configuration Specification](../configuration/Specification.md)).

### 1.3 Partial Search

- Node data must be collected using **partial search** (`POST /organizations/<ORG>/search/node`) to minimise payload size and Chef server load.
- The required attributes to collect are defined in [`../chef-api/Specification.md`](../chef-api/Specification.md).
- Results must be **paginated** using the `rows` and `start` parameters. The recommended batch size is 1000 nodes per request.
- Pages within a single organisation may be fetched concurrently using goroutines once the total node count is known from the first response. Concurrency must be bounded by the `concurrency.node_page_fetching` worker pool setting (see [Configuration Specification](../configuration/Specification.md)).

### 1.4 Collected Attributes

The following attributes must be collected per node:

| Attribute | Source path | Purpose |
|-----------|-------------|---------|
| `name` | `name` | Node identity |
| `chef_environment` | `chef_environment` | Environment grouping |
| `chef_version` | `automatic.chef_packages.chef.version` | Version tracking |
| `platform` | `automatic.platform` | Platform filtering |
| `platform_version` | `automatic.platform_version` | Platform filtering |
| `platform_family` | `automatic.platform_family` | Platform grouping |
| `filesystem` | `automatic.filesystem` | Disk space readiness check |
| `cookbooks` | `automatic.cookbooks` | Resolved cookbook list (preferred for cookbook usage analysis) |
| `run_list` | `run_list` | Raw run list (used for Test Kitchen config generation) |
| `roles` | `automatic.roles` | Role association |
| `policy_name` | `policy_name` | Policyfile policy name (null for non-Policyfile nodes) |
| `policy_group` | `policy_group` | Policyfile policy group (null for non-Policyfile nodes) |
| `ohai_time` | `automatic.ohai_time` | Unix timestamp of last Chef client run (used for stale check-in detection) |

> **Note:** See `../chef-api/Specification.md` for the difference in JSON structure between `GET /nodes/:name` and search responses. Automatic attributes are hoisted to the root of the `data` object in search responses.

### 1.5 Policyfile Support

Many modern Chef deployments use **Policyfiles** instead of roles and run-lists. Policyfile nodes have a `policy_name` and `policy_group` instead of a traditional `chef_environment` and role-based run-list. Policyfiles lock cookbook versions in a policy lock file, producing deterministic cookbook sets per policy.

- The `policy_name` and `policy_group` attributes must be collected for every node.
- Nodes are classified as either **Policyfile nodes** (both `policy_name` and `policy_group` are non-null) or **classic nodes** (using roles and run-lists).
- For Policyfile nodes, the `cookbooks` attribute (`automatic.cookbooks`) still contains the fully-resolved set of cookbooks and versions. This is the authoritative source for cookbook usage analysis regardless of whether the node uses Policyfiles or classic mode.
- The dashboard must support filtering by `policy_name` and `policy_group` in addition to environment and role (see [Visualisation Specification](../visualisation/Specification.md)).

### 1.6 Stale Check-in Detection

Nodes that have not checked in recently may have stale attribute data (especially `filesystem` for disk space evaluation). The `ohai_time` attribute records the Unix timestamp of the node's last successful Chef client run.

- After collection, compute the age of each node's data by comparing `ohai_time` against the current time.
- Nodes whose `ohai_time` is older than a configurable threshold (`collection.stale_node_threshold_days`, default: 7 days) must be flagged as **stale** in the datastore.
- Stale nodes are still included in analysis and dashboard views but must be visually distinguished so that operators know the data may be outdated.
- The dashboard must support filtering to show only stale nodes for investigation (see [Visualisation Specification](../visualisation/Specification.md)).

### 1.7 Persistence

- Collected node data must be persisted to the datastore with a timestamp recording when it was collected.
- Timestamped snapshots enable historical trending in the dashboard.

### 1.8 Fault Tolerance and Recovery

- The collection job must be fault-tolerant. Errors communicating with one organisation must be caught, logged, and skipped so that collection continues for remaining organisations.
- The collection job must support **checkpoint/resume** — if a run is interrupted mid-way, it must be able to resume without restarting from scratch.

---

## 2. Cookbook Fetching

> **Machine-parseable output policy:** Where possible, git commands invoked by batch processes must use flags that emit machine-parseable output (porcelain formats, `-z` NUL-delimited output, or explicit format strings) rather than relying on git's human-readable defaults. This makes parsing robust against locale changes, terminal width variations, and future git output format changes. See the command reference below for the specific flags required for each operation.

### 2.1 Determining Cookbooks to Fetch

- After node collection, the set of cookbooks in active use is determined from the `cookbooks` attribute collected from nodes.
- Only cookbooks in active use need to be fetched. Unused cookbooks (present on the Chef server but not applied to any node) must be identified and flagged but do not need to be fetched for analysis.

### 2.2 Git Repositories

- Cookbooks with a known git repository are cloned and kept up to date.
- Multiple base git URLs must be supported (e.g. internal GitLab, GitHub).
- On every collection run, each git repository must be **pulled** to fetch the latest changes.
- The **default branch** (`main` or `master`) must be detected automatically.
- The **HEAD commit SHA** of the default branch must be recorded after each pull. This is used by the Analysis component to skip test runs when HEAD has not changed.
- Git-sourced cookbooks include test suites (Test Kitchen, InSpec profiles, etc.) and are eligible for full compatibility testing.
- All git operations must be logged (see [Logging Specification](../logging/Specification.md)).
- Git pull operations across multiple repositories must run **in parallel** using goroutines, bounded by the `concurrency.git_pull` worker pool setting (see [Configuration Specification](../configuration/Specification.md)).

#### Git Command Reference (Machine-Parseable Invocations)

All git commands must be invoked with the flags listed below to ensure output is machine-parseable. Do not parse git's default human-readable output.

| Operation | Command | Notes |
|-----------|---------|-------|
| Clone | `git clone --quiet <URL> <DIR>` | `--quiet` suppresses progress output to stderr; exit code determines success. |
| Detect default branch | `git symbolic-ref refs/remotes/origin/HEAD --short` | Emits a single line like `origin/main`. Strip the `origin/` prefix. If this fails (detached HEAD or missing ref), fall back to checking for `main` then `master` via `git rev-parse --verify origin/main` / `git rev-parse --verify origin/master`. |
| Fetch latest | `git fetch --quiet origin` | `--quiet` suppresses progress; exit code determines success. Prefer `fetch` + `reset` over `pull` to avoid merge conflicts. |
| Reset to remote HEAD | `git reset --hard origin/<BRANCH>` | After `fetch`, hard-reset to the remote tracking branch. Exit code determines success. |
| Read HEAD commit SHA | `git rev-parse HEAD` | Emits exactly the 40-character SHA on stdout. No parsing ambiguity. |
| Read HEAD commit metadata | `git log -1 --format=json` is **not** supported by git. Instead use: `git log -1 --format='{"sha":"%H","author":"%aN","date":"%aI","subject":"%s"}'` | Produces a single-line JSON object. The `%aI` placeholder emits ISO 8601 date. Escape any double quotes in `%s` (subject) by post-processing or by using `%f` (sanitised subject) if exact fidelity is not required. |
| List changed files since last known SHA | `git diff --name-only --diff-filter=ACMRT <OLD_SHA>..<NEW_SHA>` | One file path per line. Use `--diff-filter` to exclude deleted files. For NUL-delimited output (safer with unusual filenames): add `-z`. |
| Check if path exists | `git ls-tree --name-only HEAD -- <PATH>` | Exit code 0 and non-empty output means the path exists at HEAD. |
| Verify repository health | `git status --porcelain` | `--porcelain` produces stable, machine-parseable output. Empty output means a clean working tree. |

### 2.3 Chef Infra Server

- Cookbook versions not available via git are downloaded directly from the Chef server using `GET /organizations/<ORG>/cookbooks/<NAME>/<VERSION>`.
- Cookbook versions on the Chef server are **immutable** — once uploaded, their content never changes. Therefore:
  - A given cookbook version only needs to be **downloaded once**. Subsequent collection runs must skip versions already present in the datastore.
  - A **manual rescan** option must be provided to force re-download of a specific cookbook version, for exceptional cases such as data corruption or tooling bugs.
- The same cookbook name and version may differ in content between organisations. Cookbook versions must be keyed in the datastore by **organisation + cookbook name + version**.
- Cookbooks downloaded from the Chef server do **not** include test suites and are not eligible for Test Kitchen testing. They are eligible for CookStyle linting only.

### 2.4 Cookbook Download Failure Handling

Individual cookbook version downloads can fail for reasons including but not limited to:

- **Corrupted files** — The Chef server returns a response but the content is truncated, malformed, or fails integrity checks (e.g. mismatched checksum).
- **Missing files** — The Chef server returns a 404 or similar error for a cookbook version that appeared in the inventory listing. This can occur when a cookbook version is deleted between the inventory fetch and the download, or due to backend storage issues on the Chef server.
- **Network errors** — Transient connectivity failures, timeouts, or TLS errors during the download.
- **Permission errors** — The API client lacks permission to read a specific cookbook (e.g. ACL restrictions on the Chef server).

These failures must be handled as follows:

1. **Non-fatal** — A download failure for one cookbook version must **not** cause the collection run to fail. Collection must continue for all remaining cookbooks and organisations.
2. **Flagged** — Each failed cookbook version must be flagged in the datastore with a `download_status` indicating the failure. Valid statuses are `ok`, `failed`, and `pending`. The failure reason (error message, HTTP status code if applicable) must be recorded alongside the flag.
3. **Logged** — Each failure must be logged at `WARN` severity with the `collection_run` scope, including the organisation name, cookbook name, cookbook version, and error detail.
4. **Excluded from analysis** — Cookbook versions with a `failed` download status must be excluded from CookStyle scanning and compatibility analysis. They must still appear in the dashboard with a visual indicator showing the download failure, so operators can investigate.
5. **Retried on next run** — Cookbook versions with a `failed` or `pending` download status must be retried on the next collection run (they are not treated as "already present" by the immutability optimisation). If the retry succeeds, the status is updated to `ok` and the cookbook becomes eligible for analysis.
6. **Manual rescan** — The existing manual rescan option (§ 2.3) must also clear a `failed` status and force a fresh download attempt.

### 2.5 Cookbook Upload Date Tracking

- When fetching the cookbook inventory from the Chef server (`GET /organizations/<ORG>/cookbooks?num_versions=all`), record the **version creation timestamp** for each cookbook version if available from the Chef server API.
- If the Chef server does not expose a creation timestamp directly, record the timestamp of the first time the application observed the cookbook version during collection. This serves as a proxy for upload date.
- The upload/first-seen date is stored in the `cookbooks` table and displayed in the dashboard to help teams identify stale cookbooks that may be candidates for cleanup or consolidation as part of the migration project.
- Cookbooks whose most recent version was uploaded more than a configurable threshold ago (`collection.stale_cookbook_threshold_days`, default: 365 days) are flagged as **stale cookbooks** in the dashboard. This is distinct from the active/unused flag — a cookbook can be actively used but stale (i.e. it hasn't been updated in a long time and may need attention).

---

## 3. Background Job Scheduling and Recovery

### 3.1 Scheduler

- The collection job is driven by a **cron-style scheduler** embedded in the Go application process. The schedule is configured via the `collection.schedule` setting (see [Configuration Specification](../configuration/Specification.md)).
- The scheduler must use a cron parsing library (e.g. `robfig/cron`) to evaluate the schedule expression. The expression follows standard five-field cron syntax (`minute hour day-of-month month day-of-week`).
- Only **one collection run** may be active at a time. If a run is still in progress when the next scheduled tick fires, the tick must be skipped and a `WARN` log emitted. This prevents overlapping runs from competing for resources or producing duplicate data.
- A collection run may also be triggered manually via the Web API (see [Web API Specification](../web-api/Specification.md)). Manual triggers are subject to the same single-run constraint.

### 3.2 Run Lifecycle

Each collection run proceeds through the following states:

```
scheduled → running → completed
                   ↘ failed
                   ↘ interrupted
```

| State | Description |
|-------|-------------|
| `running` | The run is in progress. A `collection_runs` row exists with `status = 'running'` |
| `completed` | All organisations were collected (some may have individual errors, but the overall run finished) |
| `failed` | The run encountered a fatal error that prevented it from continuing (e.g. database unavailable) |
| `interrupted` | The application was shut down or crashed while the run was in progress |

State transitions are persisted to the `collection_runs` table immediately so that recovery can inspect them on restart.

### 3.3 Checkpoint/Resume

The collection job must be able to resume an interrupted run without discarding work already completed. This is critical for large fleets where a full collection run may take a significant amount of time.

**Organisation-level checkpointing:**

- Before collecting from an organisation, record a `collection_runs` row with `status = 'running'`.
- After successfully collecting from an organisation, update the row to `status = 'completed'`.
- On startup, the application queries for `collection_runs` rows with `status = 'running'` or `status = 'interrupted'`. These represent interrupted runs.
- For interrupted runs, the application identifies which organisations have already been completed and resumes collection for the remaining organisations only.

**Page-level checkpointing (within an organisation):**

- After each page of nodes is fetched and persisted, update the `checkpoint_start` field on the `collection_runs` row with the `start` offset of the **next** page to fetch.
- On resume, if a `collection_runs` row has `status = 'running'` and a non-null `checkpoint_start`, the collection resumes pagination from that offset rather than from page zero.
- This avoids re-fetching and re-persisting pages of nodes that were already successfully collected before the interruption.

**Graceful shutdown:**

- On receiving a shutdown signal (`SIGTERM`, `SIGINT`), the application must:
  1. Stop accepting new work (no new organisations, no new pages).
  2. Allow in-flight HTTP requests to complete (with a configurable grace period, default 30 seconds).
  3. Update any `running` collection runs to `interrupted` with the current `checkpoint_start`.
  4. Exit cleanly.

### 3.4 Recovery on Startup

On startup, after database migrations have been applied, the application must:

1. **Detect stale runs** — Query for `collection_runs` where `status = 'running'`. These represent runs that were in progress when the application crashed without a graceful shutdown. Update their status to `interrupted`.
2. **Evaluate resumable runs** — For each `interrupted` run, determine whether it should be resumed:
   - If the run's `started_at` is within the last two collection intervals (i.e. the data is still reasonably fresh), resume the run.
   - If the run is older than two collection intervals, mark it as `failed` with an error message indicating it was abandoned due to age, and allow the next scheduled run to start fresh.
3. **Resume or discard** — Resumable runs are re-queued for immediate execution. The scheduler then continues with its normal cron schedule after the resumed run completes.

### 3.5 Sequence Within a Run

A single collection run proceeds in the following order:

1. **Node collection** — Collect node data from all configured organisations (parallel, bounded by `concurrency.organisation_collection`). Pages within each organisation are fetched in parallel (bounded by `concurrency.node_page_fetching`).
2. **Cookbook inventory** — Fetch the full cookbook listing from each organisation (`GET /organizations/<ORG>/cookbooks?num_versions=all`).
3. **Active/unused determination** — Compare the cookbook versions observed in the collected node data against the full inventory to flag active vs. unused cookbooks.
4. **Cookbook fetching** — Fetch cookbooks from git (pull) and Chef server (download new versions only), in parallel per source type (bounded by `concurrency.git_pull`).
5. **Stale node flagging** — Compare each node's `ohai_time` against the configured stale threshold and flag stale nodes.
6. **Hand off to analysis** — Signal the analysis component that new data is available. The analysis component runs cookbook usage analysis, CookStyle scans, Test Kitchen tests, and readiness evaluation (see [Analysis Specification](../analysis/Specification.md)).
7. **Metric snapshot** — After analysis completes, write pre-aggregated metric snapshots for historical trending.
8. **Log retention purge** — Purge log entries older than the configured retention period.

Steps 1–5 are the responsibility of this component. Steps 6–8 are coordinated by the application's top-level orchestrator.

---

## 4. Active/Unused Cookbook Tracking

- For every cookbook version present on the Chef server, record whether it is **actively used** (applied to at least one node) or **unused** (present on the server but applied to no nodes).
- This flag is stored per organisation + cookbook name + version.
- The dashboard uses this flag to allow users to hide unused cookbooks that are not relevant to the upgrade project (default: hide unused).

---

## 5. Role Dependency Graph Collection

To support the dependency graph view in the dashboard, the data collection component must collect role details that reveal the dependency chain from roles to cookbooks.

### 5.1 Role Expansion

- For each organisation, fetch the full list of roles using `GET /organizations/<ORG>/roles`.
- For each role, fetch the role detail using `GET /organizations/<ORG>/roles/<ROLE_NAME>` to obtain the `run_list` and `env_run_lists`.
- Parse each role's `run_list` to extract:
  - **Cookbook references** — entries like `recipe[cookbook::recipe]` or `recipe[cookbook]`
  - **Nested role references** — entries like `role[other_role]`
- Build a directed graph of role → role and role → cookbook dependencies.
- Persist the dependency graph to the datastore (see [Datastore Specification](../datastore/Specification.md)).

### 5.2 Dependency Graph Use Cases

The dependency graph enables the dashboard to answer questions such as:
- "Cookbook X is incompatible — which roles include it, and through what chain?"
- "If I fix cookbook X, how many roles and nodes become unblocked?"
- "Which roles have the deepest dependency chains and therefore the most complex upgrade path?"

See the [Visualisation Specification](../visualisation/Specification.md) for how the dependency graph is rendered in the dashboard.

---

## Related Specifications

- [Analysis Specification](../analysis/Specification.md)
- [Logging Specification](../logging/Specification.md)
- [Configuration Specification](../configuration/Specification.md)
- [Chef API Specification](../chef-api/Specification.md)
- [Datastore Specification](../datastore/Specification.md)
- [Web API Specification](../web-api/Specification.md)