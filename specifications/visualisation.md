# Data Visualisation - Component Specification

> **TL;DR:** React web dashboard consumed via the Web API. Views: Chef Client version distribution, cookbook compatibility matrix, node upgrade readiness summary, role→cookbook dependency graph, remediation priority list with auto-correct diff previews. Interactive filters (org, environment, role, policy name/group, platform, target version, stale status, complexity label). Data exports (CSV, JSON, Chef search query). Historical trend charts. Integrated log viewer scoped per job/cookbook/scan, and ownership summary view (per-owner migration progress, coverage metrics, ownership badges on node/cookbook lists).

## Overview

The data visualisation component provides a web dashboard for monitoring and managing Chef Client upgrade projects. It consumes data produced by the Data Collection and Analysis components and presents it through interactive views, filters, drill-downs, a log viewer, historical trend charts, remediation guidance, and data exports.

This component has no write path to Chef servers or cookbook repositories — it is a read-side presentation layer over the datastore, with the exception of triggering exports and manual rescan operations.

### Charting Conventions

- All graphs and charts must use **zero-based axes**. The y-axis must always start at zero. Do not truncate axes to exaggerate trends.

---

## Dashboard Views

### Chef Client Version Distribution

- Display the count and percentage of nodes running each Chef Client version across the fleet.
- Support trend over time showing how the version distribution has changed across collection runs.
- Scope by organisation, environment, role, and platform using the interactive filters (see below).

### Cookbook Compatibility Status

CookStyle (CS) and Test Kitchen (TK) are **separate signals** and must never be
merged into one verdict (see
[cop-classification.md](cop-classification.md) and
[dual-compatibility-signals.md](dual-compatibility-signals.md)). The CookStyle
signal uses the three-state-plus-untested **rollup status** vocabulary
(🟢 Ready / 🟠 Needs review / 🔴 Blocked / ⚪ Untested) — canonical in
[cop-classification.md](cop-classification.md) § CookStyle Rollup Status,
replacing the former compatible/incompatible/passed/failed wording.

- Display each cookbook (and version) alongside both its TK compatibility status
  and its CookStyle rollup status for each configured target Chef Client version.
- Test Kitchen compatibility status values:
  - **Compatible** — Test Kitchen converge and tests passed at HEAD
  - **Incompatible** — Test Kitchen converge or tests failed at HEAD
  - **Untested** — No Test Kitchen results available yet
- CookStyle rollup status values (the CS badge): **Ready** / **Needs review** /
  **Blocked** / **Untested**. For a Chef-server-sourced cookbook with no TK
  results, the CookStyle rollup status is the only available signal — show it
  with its deprecation warnings, and treat it as a weaker signal than a TK pass
  (see confidence indicators below).
- The boolean `passed` field remains available for back-compat
  (`passed = status not in {Blocked}`) but the UI renders the four-state badge,
  not pass/fail.
- Show the HEAD commit SHA and timestamp of the last test run for git-sourced cookbooks.
- Show the organisation + cookbook name + version key for Chef server-sourced cookbooks.
- Default view hides unused cookbooks (see active/unused filter below).

**Confidence indicators:**

The compatibility status must include a visual confidence indicator to prevent false confidence:

| Status | Confidence | Visual Treatment | Meaning |
|--------|------------|------------------|---------|
| Compatible (Test Kitchen) | High | Green | Full integration test passed — high confidence |
| Compatible (CookStyle only) | Medium | Amber/Yellow | No integration test; static analysis only — lower confidence |
| Incompatible | N/A | Red | Known to be incompatible |
| Untested | N/A | Grey | No data available |

The distinction between green (Test Kitchen pass) and amber (CookStyle-only pass) must be unmissable in the UI. CookStyle catches known deprecation patterns but cannot guarantee convergence success. Users must understand that "CookStyle only" is a weaker signal than a full Test Kitchen pass.

**Complexity scoring:**

- Each cookbook not in the Ready state (Blocked / Needs review) must display its **complexity score** and **complexity label** (`low`, `medium`, `high`, `critical`) alongside the status badges.
- The complexity score is **classification-weighted** (Blocker offenses dominate; Review low; Noise ~0) — computed by the Analysis component (see [Analysis Specification](analysis.md) and [cop-classification.md](cop-classification.md) § Complexity Weighting).
- Cookbooks must be sortable by complexity score to help teams identify quick wins (low complexity) and plan for harder remediation (high complexity).

**Stale cookbook indicator:**

- Cookbooks whose most recent version was first observed longer ago than the configured `collection.stale_cookbook_threshold_days` (default: 365 days) must display a visual stale indicator (e.g. a clock icon or "stale" badge).
- This signals to practitioners that the cookbook may need attention beyond just compatibility fixes — it may be unmaintained or a candidate for replacement.

### Node Upgrade Readiness

- Display a summary of node readiness per target Chef Client version using the
  three-state node readiness vocabulary: **ready** / **needs_review** / **blocked**
  (canonical in [cop-classification.md](cop-classification.md); replaces the
  former binary ready-vs-blocked split). The CookStyle contribution to readiness
  uses the rollup status; CS and TK remain separate inputs and are not merged.
- Show blocking / review reasons per node:
  - One or more cookbooks in the expanded run-list are **Blocked** (or, for
    needs_review, **Needs review**) for the target version
  - Insufficient disk space for the Habitat bundle
- Support drill-down from the summary to a per-node detail view showing which specific cookbooks are blocking and why.

**Stale node indicators:**

- Nodes whose last Chef client check-in (`ohai_time`) exceeds the configured `collection.stale_node_threshold_days` (default: 7 days) must be visually flagged as **stale** in all node list and detail views.
- Stale nodes must display the age of their data (e.g. "Last check-in: 12 days ago") so operators can immediately see how outdated the information is.
- Stale nodes with unknown disk space must be shown in a distinct category separate from nodes with confirmed sufficient or insufficient disk space.
- The readiness summary must break out stale nodes as a separate count so that operators know how many nodes need investigation before the upgrade can proceed.

### Dependency Graph

The dependency graph view shows the relationship chain from roles to cookbooks (and from roles to other roles), enabling practitioners to understand the **blast radius** of an incompatible cookbook.

- Display an interactive directed graph where:
  - **Role nodes** are shown as one shape/colour
  - **Cookbook nodes** are shown as a different shape/colour
  - **Edges** represent "includes" relationships (role → role, role → cookbook)
- Incompatible cookbooks and the roles that depend on them must be visually highlighted (e.g. red border or background) so the impact chain is immediately visible.
- Clicking a cookbook node in the graph must link to the cookbook detail view.
- Clicking a role node must show the list of nodes assigned that role.
- The graph must support:
  - **Filtering by cookbook** — highlight the subgraph reachable from/to a specific cookbook
  - **Filtering by role** — highlight the subgraph reachable from a specific role
  - **Filtering by compatibility status** — show only paths that include incompatible or untested cookbooks
- For large dependency graphs, provide a search/filter to focus on a subset rather than rendering the entire graph.
- An alternative **table view** must be available for users who prefer a flat list over a graph visualisation. The table view shows each role with its direct and transitive cookbook dependencies and the aggregate compatibility status.

### Remediation Guidance View

A dedicated view for practitioners actively working on making cookbooks compatible. This view aggregates all the analysis component's remediation outputs into a single actionable interface.

**Cookbook remediation list:**

- Display all incompatible and CookStyle-flagged cookbooks sorted by a **priority score** that combines complexity and blast radius (i.e. a low-complexity cookbook affecting many nodes should rank higher than a high-complexity cookbook affecting few nodes).
- For each cookbook, show:
  - Complexity score and label
  - Blast radius (affected node count, role count, policy count)
  - Number of auto-correctable offenses vs. manual-fix offenses
  - Quick summary of the most impactful deprecation warnings

**Auto-correct preview:**

- For cookbooks with CookStyle offenses, display the auto-correct preview generated by the Analysis component.
- Show a unified diff of the changes that `cookstyle --auto-correct` would make.
- Display statistics: total offenses, auto-correctable, remaining after auto-correct.
- Include a prominent notice that auto-correct is a **preview only** — the tool does not modify cookbook source files. Practitioners must apply the changes through their normal development workflow.

**Migration documentation:**

- For each deprecation offense, display the enriched remediation data from the Analysis component:
  - Human-readable description of the deprecation
  - Link to the Chef migration documentation
  - The Chef version where the deprecation was introduced and (if known) removed
  - Before/after code example showing the replacement pattern
- Group deprecation offenses by cop name so that practitioners see a consolidated view of each type of issue rather than individual file-level occurrences.

**Effort estimation summary:**

- At the top of the remediation view, display an aggregate effort estimation:
  - Total cookbooks needing remediation
  - Estimated quick wins (cookbooks that can be fixed entirely by auto-correct)
  - Estimated manual fixes needed
  - Total nodes blocked and the projected count that would become unblocked if each cookbook were fixed

---

## Interactive Filters

All dashboard views must support filtering by the following dimensions. Filters must be combinable and applied consistently across all views on the page.

| Filter | Description |
|--------|-------------|
| Chef server organisation | Limit view to nodes and cookbooks from one or more organisations |
| Environment | Limit view to nodes in a specific Chef environment |
| Role | Limit view to nodes assigned a specific role |
| Policy name | Limit view to nodes using a specific Policyfile policy name |
| Policy group | Limit view to nodes in a specific Policyfile policy group |
| Platform / platform version | Limit view to nodes running a specific OS platform or version |
| Target Chef Client version | **Read-only indicator** — displays the single active target set in admin config. Not a user-selectable filter. |
| Active/unused cookbook status | Show or hide cookbooks not applied to any node (default: hide unused) |
| Stale node status | Show all nodes, only stale nodes, or only fresh nodes (default: all) |
| Complexity label | Filter cookbooks by complexity label (`low`, `medium`, `high`, `critical`) |
| Owner | Filter by owner name(s) or show only unowned entities. Multi-select. See [Ownership Specification](ownership.md) § 5.1. Only visible when `ownership.enabled` is `true`. |

---

## Drill-Downs

- From the version distribution view → clicking a version bar navigates to the nodes list filtered by that Chef Client version
- From the platform distribution view → clicking a platform bar navigates to the nodes list filtered by that platform (matches platform + version precisely)
- From the cookbook compatibility view → detail view for a specific cookbook showing test history, CookStyle results, remediation guidance, auto-correct preview, and which nodes use it
- From the node readiness summary → clicking the ready/blocked count or progress bar segment navigates to the nodes list with the readiness filter and target version pre-set
- From the node detail → "View Filesystem Details" link navigates to the disk detail sub-page showing all mounted filesystems
- From the dependency graph → cookbook detail or role node list
- From the remediation guidance view → cookbook detail view with full deprecation documentation and auto-correct diff
- From a blocking cookbook in the node detail → remediation guidance for that specific cookbook

### Node Detail — Per-Source Compatibility Verdicts

The node detail page's **Upgrade Readiness** section must display per-source compatibility verdicts for each blocking (or resolved) cookbook. This gives operators actionable insight into which source is compatible and what remediation steps to take.

**Display requirements:**

- For each blocking cookbook, show the cookbook name, the version running on the node, and the overall verdict (incompatible / untested).
- Below the overall verdict, render an expandable **source verdicts** panel listing each source that was checked:

  | Source | Verdict Display | Action Hint |
  |--------|----------------|-------------|
  | Git Test Kitchen: Compatible | Green — "TK Pass (HEAD `a1b2c3d`)" | None needed |
  | Git Test Kitchen: Incompatible | Red — "TK Fail (HEAD `a1b2c3d`)" | Fix cookbook source |
  | Git CookStyle: Compatible | Amber — "CookStyle Pass (HEAD)" | Consider adding Test Kitchen |
  | Git CookStyle: Incompatible | Red — "CookStyle Fail (HEAD)" | Fix cookbook source |
  | Server CookStyle: Compatible | Amber — "CookStyle Pass (v5.1.0)" | Upload to server if git is also compatible |
  | Server CookStyle: Incompatible | Red — "CookStyle Fail (v5.1.0)" | Upload fixed version from git |
  | Any source: Untested | Grey — "Not tested" | Run CookStyle / Test Kitchen |

- When the **server version is incompatible but the git version is compatible**, show a prominent action hint: *"A compatible version exists in git — upload to Chef Server to resolve."*
- When **all sources are incompatible**, show: *"All sources report incompatibility — remediation required."*
- When **no sources have been tested**, show: *"No CookStyle or Test Kitchen results — run analysis to determine compatibility."*
- Complexity score and label should be shown from the highest-confidence source that has data (Test Kitchen > CookStyle).

### Node Disk Detail Sub-Page

Accessible from the node detail page via "View Filesystem Details" links in both the info grid and the disk space panel. Route: `/nodes/:org/:name/disks`.

**Display requirements:**

- Breadcrumb: Nodes → {node_name} → Disk Detail
- Header showing node name and platform
- Toggle checkbox to show/hide virtual/pseudo filesystems (proc, sysfs, tmpfs, squashfs, cgroup, etc.)
- Filesystem table with columns: Mount Point, Device, FS Type, Size, Used, Available, % Used (with colour-coded bar: green < 75%, amber 75–90%, red ≥ 90%)
- Windows nodes show additional columns: Drive Type, Encryption Status (auto-detected from data)
- Expandable rows for inode details (click to toggle inline inode usage)
- Warning icon (⚠) on mount points where free inodes are below 70%
- Human-readable size formatting (KB → MB → GB → TB)
- All columns left-aligned for consistency

---

## Ownership Views

When `ownership.enabled` is `true`, the dashboard includes ownership-aware views and indicators. These are fully specified in the [Ownership Specification](ownership.md) § 5 and summarised here:

- **Owner filter** — An Owner multi-select filter in the filter bar, applied consistently across all views (§ 5.1).
- **Ownership summary view** — A top-level "Ownership" navigation item showing per-owner migration progress, ownership coverage metrics, and drill-down to owner-scoped dashboards (§ 5.2).
- **Ownership indicators** — Owner badges on node lists, cookbook lists, node detail, cookbook detail, and remediation priority views. `definitive` owners show a solid badge; `inferred` owners show a dashed-outline badge (§ 5.3, § 1.4).
- **Committer sub-page** — On cookbook detail for git-sourced cookbooks, a sub-page listing git committers with an "Assign as Owners" workflow (§ 5.3).
- **Ownership management UI** — Admin section for owner CRUD, assignment management, bulk import, bulk reassignment, auto-rule status, and audit log (§ 5.4).
- **Ownership audit log** — A filterable, paginated table showing all ownership mutations with actor, timestamp, and details (§ 5.4).

---

## Historical Trending

- Store timestamped metric snapshots at the end of each collection run.
- Provide trend charts for:
  - Chef Client version distribution over time
  - Count of nodes by readiness state (**ready** / **needs_review** / **blocked**)
    per target Chef Client version over time
  - CookStyle rollup status mix over time (**Ready** / **Needs review** /
    **Blocked** / **Untested**), keeping CS distinct from TK
  - Aggregate **classification-weighted** complexity score trend over time
    (showing whether remediation effort is reducing overall complexity)
  - Count of stale nodes over time
- Trend charts must be scoped by the same interactive filters as the summary views.

### Retroactive recompute is forward-only

Trends recompute under the **current** classification criteria **going forward**;
**past trend points are frozen** and are not retroactively recomputed. The
offense-level inputs needed to re-derive old points were never retained
(snapshots store rolled-up aggregates), so historical points stay as captured and
may reflect older criteria. A criteria change (reclassification, custom-cop edit,
target-version change) recomputes current state and appends a new point under the
new criteria; it does not rewrite history. An audit event marks
criteria changes so a step in a trend is explainable. See
[cop-classification.md](cop-classification.md) § History and
[enriched-metric-snapshots.md](enriched-metric-snapshots.md).

---

## Log Viewer

The log viewer allows operators to diagnose failures without requiring access to the underlying host or log files. See also the [Logging component specification](logging.md).

- Display logs scoped to the following job types:
  - **Collection job run** — per organisation, per run
  - **Cookbook git operation** — per repository (clone or pull)
  - **Test Kitchen run** — per cookbook + target Chef Client version
  - **CookStyle scan** — per cookbook version (organisation + name + version)
- Each log entry displays: timestamp, severity level, and contextual metadata (organisation, cookbook name, commit SHA as applicable).
- Raw stdout/stderr captured from external processes (Test Kitchen, CookStyle, git) is displayed inline within the relevant log scope.
- Logs are filterable by job type, organisation, cookbook name, and date/time range.
- Failed jobs are visually highlighted to draw attention without requiring manual scanning.

---

## Data Exports

Each list view (Nodes, Cookbooks, Roles, Git Repos) has a single **Export** control
that exports **the current filtered list** — the exact rows the list is showing,
with the same filters applied (or the full list when unfiltered). The purpose is to
hand the migration engineer a clean, complete dataset to explore in external tools
(pivot tables, spreadsheets, ticketing imports), bridging "knowing what's ready" and
"performing the upgrade."

### Invariants

- **Filter parity.** The export MUST return exactly the set of entities the list
  view currently shows for the active filters. This is guaranteed by construction:
  the export reuses the same query parameters and the same datastore query as the
  list endpoint (see [web-api-exports.md](web-api-exports.md)). No filter may be
  honoured by the list but silently dropped by the export.
- **One control per list view.** No separate "ready" vs "blocked" buttons — readiness
  is a column and a list filter, so a user narrows the list first, then exports what
  they see.
- **Column scope.** Every export carries the list's columns plus the migration-useful
  fields: the three-state CookStyle rollup and readiness `status` (Ready / Needs
  review / Blocked / Untested), Test Kitchen status, and — for nodes — the disk
  detail (`available_disk_mb` free at the install path, `required_disk_mb`,
  `sufficient_disk_space`, `install_path`). Disk figures let the engineer judge
  whether tuning the required-space threshold would move nodes out of the blocked
  state. Heavy per-node JSONB (raw filesystem / cookbook maps / attributes) is out of
  scope.
- **Formats.** **CSV** and **JSON** for all four list views. **Chef search query
  string** is additionally available on the **Nodes** export only: it emits
  `name:<node> OR …` for the node names in the current filtered set, for use with
  `knife ssh` / search to target exactly those nodes.

The CookStyle vocabulary and the back-compat `passed`/`ready` boolean are canonical
from [cop-classification.md](cop-classification.md); row shapes are owned by the
export Go types. See [web-api-exports.md](web-api-exports.md) for the HTTP contract.

---

## Real-Time Updates

The dashboard receives live event notifications from the backend via a WebSocket connection (see [Web API specification § WebSocket Real-Time Events](web-api.md#websocket-real-time-events)). This eliminates polling and makes the UI feel immediately responsive to backend activity.

### Update Behaviour

- When a **collection completes** (`collection_complete` event), all visible dashboard summary views (version distribution, readiness, cookbook compatibility) automatically re-fetch their data from the REST API.
- During a **collection run** (`collection_progress` events), the dashboard displays a progress indicator showing the organisation name and node count.
- When a **cookbook status changes** (`cookbook_status_changed` event), the cookbook compatibility view highlights the affected row and refreshes its data.
- When **readiness counts change** (`readiness_updated` event), the readiness summary and trend views refresh.
- The **log viewer** appends new entries in real time when `log_entry` events arrive, without requiring a manual refresh. Entries matching the current filter scope are appended; others are silently counted and shown as a "N new entries" badge.
- **Export progress** is tracked via `export_started` / `export_complete` / `export_failed` events, replacing the previous polling-based approach. The UI shows a progress state and offers the download link immediately when the export completes.

### Connection Status Indicator

The dashboard must display a connection status indicator (e.g. in the header or footer) showing:

| State | Indicator | Description |
|-------|-----------|-------------|
| Connected | Green dot | WebSocket is connected and receiving events |
| Reconnecting | Amber dot (pulsing) | Connection lost, attempting to reconnect |
| Disconnected | Red dot | WebSocket is disabled or repeatedly failed to connect |

When reconnecting after a disconnection, the frontend must re-fetch all visible REST endpoints to catch any events that were missed during the gap. The server does not replay missed events.

### Graceful Degradation

If the WebSocket connection cannot be established (e.g. the server has `server.websocket.enabled: false`, or a proxy strips WebSocket headers), the dashboard must fall back to periodic polling with a configurable interval (default: 30 seconds). The connection status indicator should show the disconnected state but the dashboard must remain fully functional.

---

## Scalability Considerations

Chef organisations can contain many thousands of nodes. The dashboard must remain responsive at this scale.

- Summary views must be computed from pre-aggregated data in the datastore, not computed on demand from raw node records.
- Pagination or virtualised rendering must be used for any view that lists individual nodes or cookbooks.
- Filters must be applied server-side; the full dataset must never be transferred to the browser for client-side filtering.
- The dependency graph view must use lazy loading or level-of-detail rendering for large graphs (hundreds of roles/cookbooks). Consider collapsing sub-trees by default and expanding on demand.
- Export operations for large datasets must be handled asynchronously — the API returns a job ID and the frontend receives a `export_complete` WebSocket event (or polls for completion as a fallback), then offers a download link.
- WebSocket event delivery uses bounded per-client send buffers with drop-on-full semantics to protect the server from slow consumers. Dropped clients reconnect and re-fetch state from the REST API.

---

## References

- [Top-level Specification](overview.md)
- [Analysis component specification](analysis.md)
- [Logging component specification](logging.md)
- [Data Collection component specification](data-collection.md)
- [Configuration Specification](configuration.md)
- [Web API Specification](web-api.md) — REST endpoints and WebSocket real-time events
- [Ownership Specification](ownership.md) — ownership views, filters, and management UI