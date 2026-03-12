# Data Visualisation - Component Specification

> **TL;DR:** React web dashboard consumed via the Web API. Views: Chef Client version distribution, cookbook compatibility matrix, node upgrade readiness summary, role→cookbook dependency graph, remediation priority list with auto-correct diff previews. Interactive filters (org, environment, role, policy name/group, platform, target version, stale status, complexity label). Data exports (CSV, JSON, Chef search query). Notifications (webhook, email) on status changes and milestones. Historical trend charts. Integrated log viewer scoped per job/cookbook/scan, and ownership summary view (per-owner migration progress, coverage metrics, ownership badges on node/cookbook lists).

## Overview

The data visualisation component provides a web dashboard for monitoring and managing Chef Client upgrade projects. It consumes data produced by the Data Collection and Analysis components and presents it through interactive views, filters, drill-downs, a log viewer, historical trend charts, remediation guidance, and data exports.

This component has no write path to Chef servers or cookbook repositories — it is a read-side presentation layer over the datastore, with the exception of triggering exports and manual rescan operations.

---

## Dashboard Views

### Chef Client Version Distribution

- Display the count and percentage of nodes running each Chef Client version across the fleet.
- Support trend over time showing how the version distribution has changed across collection runs.
- Scope by organisation, environment, role, and platform using the interactive filters (see below).

### Cookbook Compatibility Status

- Display each cookbook (and version) alongside its compatibility status for each configured target Chef Client version.
- Compatibility status values:
  - **Compatible** — Test Kitchen converge and tests passed at HEAD
  - **Incompatible** — Test Kitchen converge or tests failed at HEAD
  - **CookStyle only** — Cookbook sourced from Chef server; no Test Kitchen results available. Show CookStyle pass/fail and any deprecation warnings.
  - **Untested** — No scan or test results available yet
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

- Each incompatible or CookStyle-only cookbook must display its **complexity score** and **complexity label** (`low`, `medium`, `high`, `critical`) alongside the compatibility status.
- The complexity score is computed by the Analysis component (see [Analysis Specification](../analysis/Specification.md)).
- Cookbooks must be sortable by complexity score to help teams identify quick wins (low complexity) and plan for harder remediation (high complexity).

**Stale cookbook indicator:**

- Cookbooks whose most recent version was first observed longer ago than the configured `collection.stale_cookbook_threshold_days` (default: 365 days) must display a visual stale indicator (e.g. a clock icon or "stale" badge).
- This signals to practitioners that the cookbook may need attention beyond just compatibility fixes — it may be unmaintained or a candidate for replacement.

### Node Upgrade Readiness

- Display a summary of how many nodes are ready vs. blocked per target Chef Client version.
- Show blocking reasons per node:
  - One or more cookbooks in the expanded run-list are incompatible with the target version
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
| Target Chef Client version | Select which target upgrade version to evaluate readiness against |
| Active/unused cookbook status | Show or hide cookbooks not applied to any node (default: hide unused) |
| Stale node status | Show all nodes, only stale nodes, or only fresh nodes (default: all) |
| Complexity label | Filter cookbooks by complexity label (`low`, `medium`, `high`, `critical`) |
| Owner | Filter by owner name(s) or show only unowned entities. Multi-select. See [Ownership Specification](../ownership/Specification.md) § 5.1. Only visible when `ownership.enabled` is `true`. |

---

## Drill-Downs

- From the version distribution view → list of nodes running a specific Chef Client version
- From the cookbook compatibility view → detail view for a specific cookbook showing test history, CookStyle results, remediation guidance, auto-correct preview, and which nodes use it
- From the node readiness summary → per-node detail view showing run-list, blocking cookbooks (with complexity scores), disk space status, stale data flag, and Chef Client version
- From the dependency graph → cookbook detail or role node list
- From the remediation guidance view → cookbook detail view with full deprecation documentation and auto-correct diff
- From a blocking cookbook in the node detail → remediation guidance for that specific cookbook

---

## Ownership Views

When `ownership.enabled` is `true`, the dashboard includes ownership-aware views and indicators. These are fully specified in the [Ownership Specification](../ownership/Specification.md) § 5 and summarised here:

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
  - Count of nodes ready vs. blocked per target Chef Client version over time
  - Aggregate complexity score trend over time (showing whether remediation effort is reducing overall complexity)
  - Count of stale nodes over time
- Trend charts must be scoped by the same interactive filters as the summary views.

---

## Log Viewer

The log viewer allows operators to diagnose failures without requiring access to the underlying host or log files. See also the [Logging component specification](../logging/Specification.md).

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

The dashboard must support exporting data for use in external upgrade automation workflows. This bridges the gap between "knowing what's ready" and "performing the upgrade."

### Ready Node Export

- Export a list of nodes that are ready to upgrade for a given target Chef Client version.
- Export formats: **CSV**, **JSON**, and **Chef search query string**.
- The Chef search query string can be used directly with `knife ssh` or similar tools to target ready nodes for upgrade.
- The export must respect all currently active filters (organisation, environment, role, platform, policy name, policy group).
- The export must include the following fields per node: node name, organisation, environment, platform, platform version, current Chef version, policy name (if applicable), policy group (if applicable).

### Blocked Node Export

- Export a list of blocked nodes with their blocking reasons.
- Export formats: **CSV**, **JSON**.
- Include blocking cookbook names, versions, complexity scores, and disk space status.
- Useful for creating tickets or work items in project management tools.

### Cookbook Remediation Export

- Export the full remediation report for all incompatible cookbooks.
- Export formats: **CSV**, **JSON**.
- Include cookbook name, version, complexity score, blast radius, auto-correctable offense count, manual fix count, and top deprecation cops.
- Useful for generating work items or sprint planning.

---

## Notifications

The dashboard must support configuring notifications that alert practitioners when significant events occur. This integrates the tool into existing development workflows.

### Notification Triggers

| Trigger | Description |
|---------|-------------|
| Cookbook status change | A cookbook's compatibility status changed (e.g. incompatible → compatible, or compatible → incompatible after a new commit) |
| Readiness milestone | The percentage of ready nodes crossed a configured threshold (e.g. 50%, 75%, 90%) |
| New incompatible cookbook detected | A previously untested or compatible cookbook is now incompatible |
| Collection failure | A collection run failed for one or more organisations |
| Stale node threshold exceeded | The number of stale nodes exceeded a configured count |

### Notification Channels

| Channel | Description |
|---------|-------------|
| Webhook | HTTP POST to a configurable URL with a JSON payload. Supports Slack, Microsoft Teams, PagerDuty, and generic webhook receivers. |
| Email | Send notifications to configured email addresses (requires SMTP configuration). |

### Notification Configuration

Notifications are configured under the `notifications` key in the application configuration (see [Configuration Specification](../configuration/Specification.md)). Each notification rule specifies a trigger, optional filters (e.g. only for specific organisations or cookbooks), and one or more channels.

The notification history must be viewable in the dashboard so that operators can see what notifications have been sent and when.

---

## Real-Time Updates

The dashboard receives live event notifications from the backend via a WebSocket connection (see [Web API specification § WebSocket Real-Time Events](../web-api/Specification.md#websocket-real-time-events)). This eliminates polling and makes the UI feel immediately responsive to backend activity.

### Update Behaviour

- When a **collection completes** (`collection_complete` event), all visible dashboard summary views (version distribution, readiness, cookbook compatibility) automatically re-fetch their data from the REST API.
- During a **collection run** (`collection_progress` events), the dashboard displays a progress indicator showing the organisation name and node count.
- When a **cookbook status changes** (`cookbook_status_changed` event), the cookbook compatibility view highlights the affected row and refreshes its data.
- When **readiness counts change** (`readiness_updated` event), the readiness summary and trend views refresh.
- The **log viewer** appends new entries in real time when `log_entry` events arrive, without requiring a manual refresh. Entries matching the current filter scope are appended; others are silently counted and shown as a "N new entries" badge.
- **Export progress** is tracked via `export_started` / `export_complete` / `export_failed` events, replacing the previous polling-based approach. The UI shows a progress state and offers the download link immediately when the export completes.
- **Notification delivery** results (`notification_sent` / `notification_failed`) appear in the notification history view in real time.

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

- [Top-level Specification](../Specification.md)
- [Analysis component specification](../analysis/Specification.md)
- [Logging component specification](../logging/Specification.md)
- [Data Collection component specification](../data-collection/Specification.md)
- [Configuration Specification](../configuration/Specification.md)
- [Web API Specification](../web-api/Specification.md) — REST endpoints and WebSocket real-time events
- [Ownership Specification](../ownership/Specification.md) — ownership views, filters, and management UI