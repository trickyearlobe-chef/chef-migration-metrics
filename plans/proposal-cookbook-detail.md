# Proposal: Per-Version Cookbook Detail Page

**Status: DEFERRED / FOR DISCUSSION.** Not approved. Deferred 2026-07-14 — the change
touches eleven call sites and the implications need more consideration. The immediate
symptom (a redundant versions list) was addressed instead by grouping versions into
active/inactive collapsibles on the existing page.

Nothing below is implemented. Keep this file for the call-site analysis, which was the
expensive part; re-verify against the tree before acting on it.

## The problem

`/cookbooks` renders **one row per version** (name, version, organisation). Clicking a
row drops the version and navigates to `/cookbooks/:name`, which can then only re-list
every version — the list the operator just came from. They pick a version and are asked
to pick it again.

The cause is a dropped parameter, not a missing feature. Everything an operator wants
about a cookbook is keyed by version, not name: metadata is per
`(organisation, name, version)`; cookstyle results are per
`(organisation, name, version, target_chef_version)`; active/unused is a property of a
version. **A cookbook name is not a thing with properties; it is a collection.** That is
why the name-scoped page keeps turning into a list — it has nothing else it could
honestly be.

## The proposal

- **`/cookbooks/:name/:version`** — the real detail page. Header (name, version, org,
  badges), git repository card, metadata card (promoted out of the per-version
  collapsible `<dl>`), and cookstyle results **first class**: summary tiles, the
  blocker/review/noise classification, and the per-cop offense groups rendered inline
  rather than behind a "View Remediation Detail →" link. The auto-correct diff stays on
  `/cookbooks/:name/:version/remediation` — it is a working artefact for someone about
  to apply a fix, not a statement of the cookbook's state.
- **`/cookbooks/:name`** — demoted to a version chooser. It must never pick a version on
  the operator's behalf.

Frontend only. Both payloads already exist (`GET /cookbooks/:name` carries every version
plus its cookstyle summaries; `GET /cookbooks/:name/:version/remediation` carries the
offense groups). Requires extracting `OffenseGroupCard` and `OffenseRow`, currently
private in `CookbookRemediationPage.tsx`.

## Call-site analysis (verified 2026-07-14 @ 97f6757)

Eleven UI sites build a `/cookbooks/:name` path. Re-verify before relying on this.

**Drop a version they already hold — the actual defect, three instances:**

- `CookbooksPage.tsx:374` — the row *is* a version (`cb.version`).
- `RoleDetailPage.tsx:114` — `RoleBlockingCookbook.cookbook_version` (`types/roles.ts:43`).
- `NodeDetailPage.tsx:358` — `NodeGraphNode.version` (`types/nodes.ts:189`), which the
  dependency tree already *renders* as `@{version}`. (This tree is not a force graph —
  `ForceGraph` is used only by `RoleDetailPage`.)

**Genuinely hold no version — these are why a chooser is unavoidable:**

- `OwnerDetailPage.tsx:356` — `BlockingCookbookSummary` is
  `{cookbook_name, complexity_label, affected_node_count}` (`types/ownership.ts:50`).
- `RoleDetailPage.tsx:205` / `ForceGraph.tsx:870,913,933` — `RoleGraphNode` has no
  version (`types/roles.ts:92`), unlike `NodeGraphNode`. See `todo-tech-debt.md`.
- `RemediationPage.tsx:577` and `CopAnalysisTab.tsx:562` — aggregate rows over N versions
  (`version_count`); when it exceeds 1 there is no single version to link to. Both are
  **correct as written** — Cop Analysis already links per-version from its nested version
  rows (`:588`).
- `CookbookRemediationPage.tsx:114` — breadcrumb navigating *up* from a version.

**Wrong target, unrelated to versions:** `CookbookCommittersPage.tsx:206` breadcrumbs to
`/cookbooks/:name`, but committers are a git concept — it should link to
`/git-repos/:name`.

## Open questions before this is approved

- Is the chooser worth its own page, or should the name route redirect when only one
  version exists?
- Does absorbing the offense groups make `/cookbooks/:name/:version/remediation`
  redundant enough to retire, and what repoints the drill-throughs that target it?
- The remediation route ignores `organisation` though the cookstyle key includes it
  (`todo-tech-debt.md`) — fix that first, or carry it on the new route and leave the old
  one ambiguous?
