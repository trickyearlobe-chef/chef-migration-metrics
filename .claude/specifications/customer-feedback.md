# Customer Feedback — Snags and Enhancement Requests

Captured during review session. Items need specs before implementation.

Status key: [ ] Needs spec | [~] Spec in progress | [x] Spec complete

---

## Enhancement Requests

- [x] **Role compatibility page** — new page showing compatibility status rolled up to the role level. A role is compatible only if every cookbook it references (directly and transitively) is compatible. Should show which specific cookbooks are blocking. Builds on existing `role_dependencies` data. **Spec: `roles.md`**
- [x] **Role detail page** — new page combining: compatibility status (rolled up), dependency graph, blast radius (which nodes use this role), nested role chain. **Spec: `roles.md`**
- [x] **Blast radius visualisation** — show impact footprint of an incompatibility (cookbook → roles affected → nodes affected, transitive dependency ripple). Needs discovery session with customer to determine the right visual approach. Raw data exists across `role_dependencies`, `node_snapshots` run_lists, and dependency graph. **Spec: `blast-radius.md`**
- [x] **Version distribution battery bars** — redesign the version summary bar graphs as battery-style bars grouped by major version, with minor versions shown as coloured segments within each bar. Click-through from major → minor → filtered node list. **Spec: `version-battery-bars.md`**
- [x] **Windows platform friendly names** — implemented. Configurable mapping table in config store with 22 built-in defaults (Windows Desktop/Server, CentOS, Oracle, Amazon Linux). Prefix matching with longest-match-wins. Admin UI at `/admin/config/platform-display-names` for add/edit/delete/reset. `PlatformLabel` component shows friendly name with raw tooltip. Enriched on node list, node detail, and all API responses via `platform_display_name` field. **Spec: `platform-display-names.md`**
- [x] **Two-tier staleness** — implemented. Three tiers (Fresh/Warning/Critical) computed at query time from ohai_time. Config: `stale_node_warning_hours` (72h) + `stale_node_critical_days` (7d). StaleBadge shows "Missing" (amber) or "Gone" (red) with age. Filter supports fresh/warning/critical. Trend chart shows three series. **Spec: `staleness-tiers.md`**
- [x] **Multi-select filters** — low-cardinality filters (platform, version, staleness, status) become multi-select checkboxes/tags. High-cardinality filters (roles, possibly environments) stay as type-ahead with backend search. Freeform text for node name search. **Spec: `filter-ux-overhaul.md`**
- [x] **Promote global filters to top bar** — cross-cutting filters like staleness tier (and possibly target chef version) move alongside the org selector in the top bar. Per-page filter bars keep page-specific concerns only. **Spec: `filter-ux-overhaul.md`**
- [x] **Role filter as type-ahead** — role filter must be a searchable text input hitting a backend endpoint, not a dropdown. Thousands of roles make dropdowns unusable. **Spec: `filter-ux-overhaul.md`**
- [x] **Node list check-status icons** — implemented. `CheckStatusIcons` component renders three compact icons (disk, CookStyle, TK) per node row with colour-coded status (green/red/amber/orange/grey) and shape overlays (✓/✗/~/!/?) for accessibility. Per-check status derived from readiness data in `check_status.go`. `scan_error` state (orange) distinguished from `failed` (red). CookbooksPage also surfaces `scan_error` as "Scan Error" (orange) instead of "Incompatible" (red). **Spec: `node-list-enhancements.md`**
- [x] **Node-scoped dependency graph** — move the force-directed graph from the fleet-wide page to the node detail page. Shows this node's full dependency chain: run_list → roles → nested roles → cookbooks → transitive deps. Manageable size, actually readable. Fleet-wide page demoted to table-only or removed. **Spec: `dependency-graph-refactor.md`**
- [x] **Role-scoped dependency graph** — same force-directed graph on the role detail page showing everything the role pulls in. Ties into the role compatibility page (compatibility + deps + blast radius + nested roles in one view). **Spec: `dependency-graph-refactor.md`**

## Bugs / Snags

- [x] **Complexity trend card shows only today's data** — fixed. Backend now writes `complexity_summary` metric snapshots and trend handler reads from `metric_snapshots` with live-data fallback.
- [x] **Readiness trend card uses fake timestamps** — fixed. Frontend types now include `completed_at` and trend cards use real timestamps.
- [x] **Filter combobox prefix matching** — not a bug. `FilterCombobox` is a plain `<select>` element, not a searchable combobox. No client-side substring matching occurs. Will be revisited when `filter-ux-overhaul.md` lands type-ahead components.
- [x] **chef-vault 1.3.1 CookStyle crash** — fixed. New `CookstyleResultRow` component distinguishes three states: Passed (green), Failed (red), Scan Error (orange). Error message displayed inline; `process_stderr` available via expandable detail. Backend already stored `error_message` and `process_stderr` — only the frontend was ignoring them.
- [ ] **Fleet-wide dependency graph unusable at scale** — force-directed simulation with thousands of roles/cookbooks is slow to render and produces an unreadable hairball. Addressed by `dependency-graph-refactor.md`.

## Not Yet Captured

Customer system is currently down — additional issues were observed but not yet documented. Will capture when access is restored.