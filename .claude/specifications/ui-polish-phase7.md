# UI Polish — Phase 7

Collected from live testing of the dual-signal branch. Focuses on clarity, consistency, and usability across list views and dashboard.

## Trends Graphs Clarity

### Problem
"Complexity Score Trend" and "Node Readiness Trend" on the dashboard are not immediately clear to customers. They need to help drive migration work forward.

### Current Behaviour
- **Complexity Score Trend**: Line chart of average cookbook complexity score over time per target Chef version. Breakdown shows low/medium/high/critical counts.
- **Node Readiness Trend**: Line chart of % nodes ready over time per target Chef version. Second chart shows absolute ready vs blocked counts.

### Required Changes
- Review axis labels and chart titles for plain-English meaning.
- Add short explanatory subtitles beneath each trend card title (e.g. "Lower is better — average CookStyle offence severity across all cookbooks").
- Consider whether "complexity score" is meaningful to a migration team or if "CookStyle offence count" or "incompatible cookbook count" trend is more actionable.
- Node readiness trend is clearer but should state what "ready" means (all cookbooks compatible + sufficient disk).

## Nodes List — Split Checks into 3 Columns

### Problem
The Checks column uses small SVG icons with overlay characters (✓, ✗, ~, ?, !) that are hard to read. It is a single composite column that cannot be individually sorted or filtered.

### Current Behaviour
One "Checks" column with three icons (disk, CookStyle, TK). Filtering uses a composite `readiness_filter` param (ready, blocked, cookbooks_blocked, disk_blocked, disk_unknown).

### Required Changes
Replace the single Checks column with three separate columns: **Disk**, **CookStyle**, **TK**. Each column gets:
- A badge (using the badge pattern from dual-signal work)
- Independent multi-select filter
- Independent sort

Badge variants:
- **Disk**: `disk_sufficient` (green), `disk_insufficient` (red), `disk_unknown` (grey)
- **CookStyle**: reuse existing `cs_compatible`, `cs_incompatible`, `cs_untested`
- **TK**: reuse existing `tk_passed`, `tk_failed`, `tk_partial`, `tk_untested`

Map from current status values:
- `cookstyle_status: "passed"/"warnings"` → `cs_compatible`
- `cookstyle_status: "failed"/"scan_error"` → `cs_incompatible`
- `cookstyle_status: "unknown"` → `cs_untested`
- `kitchen_status: "passed"` → `tk_passed`
- `kitchen_status: "failed"/"scan_error"` → `tk_failed`
- `kitchen_status: "partial"` → `tk_partial`
- `kitchen_status: "unknown"` → `tk_untested`
- `disk_status: "sufficient"` → `disk_sufficient`
- `disk_status: "insufficient"` → `disk_insufficient`
- `disk_status: "unknown"` → `disk_unknown`

Backend: add `disk_status`, `cookstyle_status`, `kitchen_status` as individual filter params alongside existing `readiness_filter` (keep for backwards compat). Add sort fields for each.

Frontend: replace `CheckStatusIcons` usage in NodesPage with three separate badge columns, each with `SortableColumnHeader` and `FilterMultiCheckbox`.

Preserve existing tooltip detail strings on each badge.

## Cookbooks TK Column Clarity

### Problem
The TK column appears on the server cookbooks list but TK data only exists when a matching `git_repos` entry has kitchen results. This is confusing.

### Current Behaviour
TK status derived by name-matching server cookbooks against `git_repos`, then aggregating `git_kitchen_results`. Cookbooks without a git match show "—".

### Required Changes
- Add a tooltip or footnote explaining: "Test Kitchen results from matching Git repository. Dash means no Git repo found."
- Make TK status filterable on the cookbooks list (multi-select: passed, failed, partial, untested, no-repo).
- Backend: add `tk_status` filter param to cookbook list endpoint.

## List View Filter/Sort Audit

### Principle
Every list view must have filtering and sorting on every meaningful column.

### Current Gaps

**Roles list:**
- TK column: not sortable, not filterable → add both
- Cookbooks column: not sortable → consider adding

**Cookbooks list:**
- TK column: not sortable, not filterable → add both
- Version column: not filterable → low priority, leave for now
- Organisation column: not sortable → leave (org-scoped usage)

**Git Repos list:**
- Git URL: not sortable → add sort
- Compatibility (CookStyle): not sortable → add sort
- TK Status: not sortable → add sort
- TK Results: not sortable → add sort (by passed count or ratio)
- Head Commit: not sortable, not filterable → leave (not meaningful)
- Default Branch: not sortable, not filterable → consider filter
- Last Fetched: not sortable → add sort

**Nodes list:**
- Checks column split into 3 (Disk, CookStyle, TK) — each sortable + filterable (covered in "Split Checks" section above)
- Ohai Time: not filterable → low priority

## Parked Items (Future Phases)

### Node Detail Page Rework
- Add dependency list (run list → roles → cookbooks) and force-directed graph.
- Deduplicate existing complex sections.
- Needs further discussion before specifying.

### Dependencies Page Evaluation
- Assess whether shared-cookbook analysis and org-wide graph justify a standalone page.
- Consider folding unique features into Roles or Node detail.

### Remediation Page Review
- Assess value for cookbook migration teams.
- Bigger discussion, separate phase.
