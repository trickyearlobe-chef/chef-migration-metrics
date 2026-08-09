# Plan: Split Cop Analysis into Server + Git tabs (consistent grain)

## Context

The single **Cop Analysis** view (Remediation page) with an `All sources / Server / Git`
dropdown produces inconsistent numbers because server and git cookbooks have different
natural grains:

- A **git repo is 1:1 with a cookbook** (one name, one HEAD, no versions/org).
- A **server cookbook** has real multiplicity (many immutable versions, per org).

Consequences seen in the UI:
- **Double-count in "All sources"**: the header "Cookbooks affected" counts distinct
  `{source, name}` (`handle_cookstyle_cops.go:135`), so a cookbook present in both server
  and git is counted twice → a single blocker cop shows **2026** while the Blocker card
  (all 7 blockers) shows **1945**.
- **Stale drill-down**: the expanded cookbook list isn't reset/refetched when the filter
  changes (`CopAnalysisTab.tsx` — `loadData` depends on `source`, but `drillItems` is only
  set in `openDrillDown`), so server rows persist under a Git filter.
- **Hidden pagination**: the drill-down requests `per_page: 20` and renders `resp.data` but
  drops `resp.pagination` (`CopAnalysisTab.tsx:118`), so the header says 1942 and the list
  shows 20 with no "of N" / pager.

**Decision (agreed):** replace the source dropdown with two tabs — **Cop Analysis (Server)**
and **Cop Analysis (Git)** — each using its natural grain. Server headline = **distinct
cookbook name**, drill-down **grouped by name**, expandable to the **version/org** detail.
Both tabs have the cop→cookbooks click-through with surfaced pagination.

Splitting per-source auto-resolves the double-count: with `source` fixed per tab, the existing
`{source, name}` count is already distinct-name-within-source. So no headline count-query
change is needed — the remaining work is the server drill-down grouping, pagination, and reset.

## Design

- Remediation page tabs become: `Priority | Cop Analysis (Server) | Cop Analysis (Git)`.
  Remove the `All sources / Server / Git` dropdown; each tab hardcodes its `source`.
- **Server tab**: headline "Cookbooks affected" = distinct cookbook name (already true when
  `source=server`). Drill-down returns **one row per cookbook name** (grouped), each
  **expandable** to its `{version, org}` rows (the current finest grain). Paginated by name.
- **Git tab**: cop → repos, 1:1. Drill-down is the flat repo list (current logic). Paginated.
- **Both**: surface `resp.pagination` — "showing X of N" + a pager (or load-more). Reset the
  open drill-down when the cop, tab, classification filter, sort, or target version changes.

## Chunks

### Chunk 1 — Backend: server drill-down grouped by name  (`handle_cookstyle_cops.go`)
- `handleCookstyleCopCookbooks` for `source=server`: aggregate the per-`{name,version,org}`
  rows into **one item per cookbook name**, carrying a nested `versions[]` (version, org,
  offences, would_pass) for the expand. Paginate by name; return an accurate total.
- `source=git` path unchanged (already name-grained / 1:1).
- Keep the current finest-grain shape available for the git list and the server expand detail.
- **Tests**: table/unit test the grouping (multiple versions/orgs of one name → one item with
  N nested versions; distinct-name total); functional test that the server drill-down total
  equals the header "cookbooks affected" for the same cop+target.

### Chunk 2 — Frontend: two tabs + reset + pagination  (`RemediationPage`, `CopAnalysisTab.tsx`)
- Split into `CopAnalysisServerTab` + `CopAnalysisGitTab` (or one component parameterised by a
  fixed `source` prop); remove the source dropdown; register both under the Remediation tab bar.
- Reset the drill-down (`drillCop`, `drillItems`) on change of `[copFocus, classFilter, sort,
  order, targetChefVersion, tab]` via an effect — fixes the stale panel (issue A).
- Surface pagination: render `resp.pagination` for the drill-down ("showing X of N" + pager or
  load-more) instead of discarding it (issue C).
- **Server drill-down UI**: one row per cookbook name, expandable to the `versions[]` detail.
- **Git drill-down UI**: flat repo list (unchanged), paginated.
- **Tests (vitest)**: changing the filter with a cop expanded resets the panel; the drill-down
  shows the total and pages; server rows expand to version/org detail; the git tab lists repos.

### Chunk 3 — Spec + nav
- Update the cop-analysis section (`journeys/cop-classification.md` or the remediation
  view spec) to document the two tabs, the per-source grain (server = distinct name with
  version/org detail; git = 1:1 repo), and the invariant: **within a tab, header count =
  drill-down total** (shared record selection).
- Update any nav/deep-links that pointed at `?source=` on the old single tab.

## Verification
- `go test ./internal/webapi/...`; `go test -tags functional ./internal/webapi/...` (drill-down
  total == header count per cop); `golangci-lint`.
- Frontend `tsc` + `vitest`: reset-on-filter, pagination surfaced, server expand, git list.
- Manual: open each tab, confirm the headline count equals what you page through; expand a
  server cop → grouped names → expand a name → version/org rows; switch tabs/filters and confirm
  the drill-down resets. No more "1942 vs 20".
