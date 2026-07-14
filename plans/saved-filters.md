# Plan — Saved Filters

Spec: `specifications/saved-filters.md`.
Branch: `feature/saved-filters` (create from `main`).

Decisions (settled 2026-07-14, do not re-litigate): private with explicit share;
filters only (no sort/page); stale refs kept + warned on apply; global filters
(`target_chef_version`, `stale_tiers`) excluded.

No new query capability is needed — `NodeSnapshotFilter.Roles` already filters on
a role list. This is persistence + UI.

## Backend contract (for the UI chunks)

- `GET/POST /api/v1/saved-filters`, `PATCH/DELETE /api/v1/saved-filters/{id}`.
  PATCH carries any of name / filters / shared — rename, re-select and share are
  one call. Applying needs no endpoint: the client replays the params into the
  view's existing list request.
- The per-view param allowlist lives in `internal/webapi/saved_filter_params.go`
  and must track each view's `<x>FilterFromValues` parser (and the `view_name`
  CHECK in `migrations/0051`). Adding a filter param to a view means adding it
  there too, or it cannot be saved.

## Chunk 2 — Nodes view UI

Scope: `frontend/src/pages/NodesPage.tsx`, `frontend/src/api/`, new component
Dependencies: backend (done)

Steps (TDD):
1. Save-current-selection control + named list of own/shared filters.
2. Applying writes the view's URL params (keeps deep-link/back/bookmark working);
   does not touch sort, page, or global filters.
3. Stale-reference warning on apply ("3 of 20 roles no longer exist").

Acceptance: the 20-role "All Windows OS" cohort survives a reload and a new
session; applying it leaves sort/page/global lens untouched.

## Chunk 3 — Generalise to the other list views

Scope: Roles, Cookbooks, Git Repos pages
Dependencies: Chunk 2 (the backend already serves all four views)

Extract the Nodes control into a shared component once its shape is proven —
not before (the views hold filter state bespoke today; do not invent a generic
abstraction ahead of the second real caller).

Acceptance: same control on all four list views, no per-view special-casing left
in the component.
