# Blast Radius Visualisation — Component Specification

> **Status: Discovery / Proposal** — the customer does not yet have a clear vision for blast radius visualisation. This spec frames the problem, catalogues what already exists, proposes integration points, and presents visualisation options for discussion.

## Problem

Users need to answer "if cookbook X is broken, what is the impact?" — not just at the cookbook level but cascading through roles and down to nodes. Blast radius data already exists in fragments (complexity scores, affected counts, dependency edges) but there is no dedicated visualisation that ties them together. The information is scattered across the cookbook detail page, remediation priority list, and dependency graph, making it hard to reason about fleet-wide risk or compare impact across cookbooks.

## What Blast Radius Means

Blast radius is not a single number. It has multiple dimensions:

**Direct node impact** — how many nodes include this cookbook in their expanded run_list (already computed as `affected_node_count` in complexity data).

**Role impact** — which roles reference this cookbook directly or transitively via nested roles (already computed as `affected_role_count`). A single incompatible cookbook pulled in by `role:base` can cascade to every node in the fleet.

**Transitive cookbook impact** — if cookbook A depends on cookbook B and B is incompatible, A is also potentially at risk. The dependency chain means a single deep incompatibility can poison many upstream cookbooks.

**Policy impact** — which Policyfile `policy_name` / `policy_group` combinations include this cookbook (already computed as `affected_policy_count`).

**Aggregate impact** — across all incompatible cookbooks, what percentage of the fleet is affected. This is the "big picture" number that tells leadership whether migration is nearly done or barely started.

## Existing Data and Endpoints

The following blast radius data is already collected or derivable:

| Data Point | Source | Location |
|---|---|---|
| `affected_node_count` | Complexity scoring | Cookbook detail API, remediation priority API |
| `affected_role_count` | Complexity scoring | Cookbook detail API, remediation priority API |
| `affected_policy_count` | Complexity scoring | Cookbook detail API, remediation priority API |
| Role → cookbook edges | `role_dependencies` table | Dependency graph API |
| Role → role edges | `role_dependencies` table | Dependency graph API |
| Node → roles mapping | `node_snapshots.roles` | Node detail API |
| Node → cookbooks mapping | `node_snapshots.cookbooks` | Node detail API |
| `priority_score` (combines complexity + blast radius) | Remediation priority endpoint | `GET /api/v1/remediation/priority` |
| Node count per role | Roles spec (in progress) | `GET /api/v1/roles/:name` |
| `total_blocked_nodes` | Remediation summary | `GET /api/v1/remediation/summary` |

### What Is Missing

- **`affected_percentage`** — `affected_node_count / total_nodes * 100`. Currently requires the client to fetch total node count separately and compute the ratio.
- **Aggregate blast radius summary** — no single endpoint returns "top N highest-impact incompatible cookbooks" or "X% of fleet blocked" in a dashboard-ready shape.
- **Transitive cookbook impact chain** — the dependency graph shows role→cookbook edges but does not surface cookbook→cookbook dependency chains and their cumulative blast radius.

## Where Blast Radius Appears

Blast radius is most useful as a component integrated into existing pages rather than a standalone page. Proposed integration points:

### Cookbook Detail Page — "Impact" Section

Add a dedicated section to the existing cookbook detail page showing:

- Node count affected by this cookbook (already available).
- Percentage of total fleet affected (new: `affected_percentage`).
- Visual bar showing the proportion of total fleet — immediately communicates whether this is a 2% problem or an 80% problem.
- Role list with per-role node counts (already available via `nodes_by_role` in cookbook detail response).
- Policy list with per-policy node counts (already available via `nodes_by_policy`).
- Breakdown by environment and platform (already available).

This section reuses data already returned by `GET /api/v1/cookbooks/:name`. The only new data point is the percentage bar, which requires knowing total fleet size.

### Role Detail Page — Blast Radius Section

Already specified in the roles component spec. Shows node count, breakdown by organisation, environment, and platform. No additional work needed here beyond what the roles spec defines.

### Remediation Priority Page — Visual Indicator

The remediation priority list already includes `affected_node_count`, `affected_role_count`, and `affected_policy_count` per cookbook. Enhance with:

- A small inline bar or colour-coded cell showing relative blast radius (this cookbook's affected nodes as a proportion of fleet total).
- Colour intensity or bar width communicates severity at a glance without requiring users to mentally compare raw numbers across rows.

### Dashboard Summary Card (Optional)

A card on the main dashboard showing:

- "X% of fleet blocked" — percentage of nodes that have at least one incompatible cookbook.
- "Top 5 highest-impact incompatible cookbooks" — sorted by affected node count.
- Trend indicator if historical data is available (is the percentage going up or down over time).

This card gives leadership a single place to check overall migration risk without drilling into remediation details.

## Visualisation Options

The customer has not specified a preferred visualisation style. The following options are presented for discussion, ordered from simplest to most complex.

### Option 1: Impact Bars

Simple horizontal bars on cookbook and role pages showing affected node count relative to total fleet. Each bar is a single coloured segment against a grey background representing the full fleet.

- Pros: low effort, high clarity, fits naturally into existing page layouts, no new page needed.
- Cons: does not show dependency chains or cascading impact.
- Best for: quick "how big is this problem" assessment on detail pages.

### Option 2: Heat Map Table

A table with cookbooks as rows and impact dimensions as columns (node count, role count, policy count, complexity). Colour intensity in each cell shows severity relative to other cookbooks.

- Pros: good for comparing blast radius across many cookbooks at once, compact, familiar table format.
- Cons: does not show relationships or dependency chains, requires a dedicated view or section.
- Best for: a "blast radius comparison" tab on the remediation priority page.

### Option 3: Treemap

Area-proportional blocks where each block represents a cookbook and block size corresponds to affected node count. Colour encodes complexity label. Clicking a block drills into the cookbook detail.

- Pros: immediately shows which cookbooks dominate fleet impact, visually striking.
- Cons: less actionable than a sorted list, labels hard to read for small blocks, unfamiliar to some users.
- Best for: a dashboard-level "fleet risk overview" widget.

### Option 4: Cascade / Sankey Diagram

A flow diagram showing how impact cascades from incompatible cookbooks through roles to nodes. Left column = incompatible cookbooks, middle column = roles, right column = node groups. Flow width proportional to node count.

- Pros: shows the "why" — users can trace exactly how a cookbook's incompatibility reaches their nodes.
- Cons: complex to build, potentially confusing for large graphs, requires significant frontend investment.
- Best for: answering "why does cookbook X affect so many nodes?" — the dependency chain question.

### Recommendation

Start with **Option 1 (impact bars)** integrated into existing pages. It is the simplest, most actionable, and least disruptive approach. The data already exists — only the visual rendering and `affected_percentage` calculation are new.

If users request a comparative view across cookbooks, add **Option 2 (heat map table)** as a tab or section on the remediation priority page.

Revisit Options 3 and 4 only if users express a need for a dedicated blast radius dashboard or dependency chain exploration. The existing dependency graph already partially serves the cascade use case.

## API Enhancements

### Extend Cookbook Detail Response

Add `affected_percentage` to each complexity entry in the cookbook detail response:

| Field | Type | Description |
|---|---|---|
| `affected_percentage` | `float` | `affected_node_count / total_nodes * 100`, rounded to one decimal place. Returns `0.0` when total nodes is zero. |

This avoids every client independently fetching total node count and computing the ratio.

### Extend Remediation Priority Response

Add `affected_percentage` to each entry in the remediation priority response, using the same calculation as above.

### New Endpoint: Dashboard Blast Radius Summary

`GET /api/v1/dashboard/blast-radius-summary`

Returns a dashboard-ready summary of fleet-wide blast radius for incompatible cookbooks.

**Query parameters:** `target_chef_version` (required), `limit` (optional, default 5 — number of top cookbooks to return).

**Response shape:**

| Field | Type | Description |
|---|---|---|
| `target_chef_version` | `string` | The target version evaluated |
| `total_nodes` | `integer` | Total nodes in fleet |
| `blocked_nodes` | `integer` | Nodes with at least one incompatible cookbook |
| `blocked_percentage` | `float` | `blocked_nodes / total_nodes * 100` |
| `top_cookbooks` | `array` | Highest-impact incompatible cookbooks |
| `top_cookbooks[].cookbook_name` | `string` | Cookbook name |
| `top_cookbooks[].affected_node_count` | `integer` | Nodes affected |
| `top_cookbooks[].affected_percentage` | `float` | Percentage of fleet |
| `top_cookbooks[].complexity_label` | `string` | `low` / `medium` / `high` / `critical` |
| `top_cookbooks[].affected_role_count` | `integer` | Roles affected |

This endpoint powers the optional dashboard summary card. It can be built entirely from existing complexity and node snapshot data.

## Discovery Questions for Customer

The answers to these questions determine where to invest effort. They should be discussed before committing to anything beyond Option 1.

**"Which cookbooks should we fix first for maximum impact?"** — If this is the primary concern, the remediation priority page already addresses it. Adding impact bars and `affected_percentage` to that page may be sufficient. No new pages needed.

**"I need to understand WHY a cookbook affects so many nodes — show me the dependency chain."** — This points toward enhancing the existing dependency graph with blast radius annotations (node counts on edges, highlighting high-impact paths). The cascade diagram (Option 4) would also serve this but is higher effort.

**"I want a big-picture view of overall fleet risk for leadership reporting."** — This points toward the dashboard summary card and possibly the treemap (Option 3). Relatively self-contained additions.

**"I want to compare blast radius across all incompatible cookbooks side by side."** — This points toward the heat map table (Option 2) on the remediation priority page.

**"I need all of the above."** — Then a phased approach: (1) impact bars + `affected_percentage` on existing pages, (2) dashboard summary card, (3) heat map on remediation page, (4) cascade diagram if still needed.

## Phased Delivery

### Phase 1 — Minimal (recommended starting point)

- Add `affected_percentage` to cookbook detail and remediation priority API responses.
- Render impact bars on the cookbook detail page showing fleet percentage.
- Add inline blast radius indicator (mini bar or colour cell) to remediation priority table rows.

### Phase 2 — Dashboard Card

- Implement `GET /api/v1/dashboard/blast-radius-summary`.
- Add a "Fleet Risk" card to the main dashboard.

### Phase 3 — Comparative View

- Add a heat map table or sortable blast radius columns to the remediation priority page.
- Customer feedback from Phases 1–2 will determine whether this is needed.

### Phase 4 — Advanced (only if requested)

- Treemap widget for dashboard.
- Cascade / Sankey diagram for dependency chain exploration.
- These require significant frontend investment and should only be pursued if simpler options prove insufficient.

## Related Specifications

- [Analysis](analysis.md) § 4.3 — cookbook complexity scoring and blast radius computation
- [Visualisation](visualisation.md) § Dependency Graph, § Remediation Guidance View
- [Roles](roles.md) § Blast Radius Section, § Role Detail Page
- [Web API](web-api.md) § Cookbook Detail, § Remediation Priority, § Dependency Graph Endpoints