# Plan: UI Polish Phase 7

## Goal
Improve clarity, consistency, and usability across list views and dashboard based on live testing feedback.

## Specs to Read
- `.claude/specifications/ui-polish-phase7.md`
- `.claude/specifications/dual-compatibility-signals.md` (badge patterns)
- `.claude/specifications/project-conventions.md`

## Prerequisites
- Merge `feature/dual-compatibility-signals` into main first (user permission required).
- Create new branch `feature/ui-polish-phase7` from main.

## Steps

### Trends graphs clarity
1. Add explanatory subtitles to trend cards.
2. Review axis labels for plain-English meaning.
3. Consider renaming "Complexity Score" to something more actionable.

### Nodes list — split Checks into 3 columns
4. Add `DiskBadge` component and `disk_*` variants to StatusBadge.
5. Add backend filter params: `disk_status`, `cookstyle_status`, `kitchen_status`. Add sort fields.
6. Replace single Checks column in NodesPage with 3 columns (Disk, CookStyle, TK) — each with badge, SortableColumnHeader, and FilterMultiCheckbox. Preserve tooltip detail strings.

### Cookbooks TK clarity + filter
7. Add tooltip explaining TK data provenance on cookbooks list.
8. Add `tk_status` filter param to cookbook list backend endpoint.
9. Add TK multi-select filter to CookbooksPage frontend.

### List view filter/sort audit
10. Roles list: add TK filter + sort (backend + frontend).
11. Cookbooks list: add TK sort (backend + frontend).
12. Git repos list: add sort on compatibility, TK status, last fetched, git URL.

## Acceptance Criteria
- All trend charts have clear subtitles explaining what they measure.
- Nodes list uses badge components instead of SVG icons.
- Cookbooks TK column has filter and explanatory tooltip.
- All list views have sort+filter on every meaningful column.
- All existing tests pass; new tests for added filters/sorts.

## Parked (not this phase)
- Node detail page rework (needs discussion).
- Dependencies page evaluation.
- Remediation page review.
