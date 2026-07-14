# Plan — Saved Filters

Spec: `specifications/saved-filters.md`.

Decisions (settled 2026-07-14, do not re-litigate): private with explicit share;
filters only (no sort/page); stale refs kept + warned on apply; global filters
(`target_chef_version`, `stale_tiers`) excluded.

No new query capability is needed — `NodeSnapshotFilter.Roles` already filters on
a role list. This is persistence + UI.

## Backend contract (for the UI chunks)

- `GET/POST /api/v1/saved-filters`, `PATCH/DELETE /api/v1/saved-filters/{id}`.
  PATCH carries any of name / filters / shared — rename, re-select and share are
  one call. Applying needs no endpoint: the client sets the view's own filter
  state from the stored params.
- The per-view param allowlist lives in `internal/webapi/saved_filter_params.go`
  and must track each view's `<x>FilterFromValues` parser (and the `view_name`
  CHECK in `migrations/0051`). Adding a filter param to a view means adding it
  there too, or it cannot be saved.

## Chunk 3 — Generalise to the other list views

Scope: Roles, Cookbooks, Git Repos pages

Extract `components/SavedFilterBar.tsx` (proven on Nodes) into the other three
views. The per-view parts that must not be absorbed into the shared component:
the state↔param mapping (`pages/nodeSavedFilters.ts` is the Nodes one) and the
stale-reference check, which needs that view's entity catalogue.

Acceptance: same control on all four list views, no per-view special-casing left
in the component.
