# Plan — Saved Filters

Spec: `specifications/saved-filters.md`.
Branch: `feature/saved-filters` (create from `main`).

Decisions (settled 2026-07-14, do not re-litigate): private with explicit share;
filters only (no sort/page); stale refs kept + warned on apply; global filters
(`target_chef_version`, `stale_tiers`) excluded.

No new query capability is needed — `NodeSnapshotFilter.Roles` already filters on
a role list. This is persistence + UI.

## Chunk 1 — Storage + API (backend, no UI)

Scope: `migrations/`, `internal/datastore/`, `internal/webapi/`
Dependencies: none

Steps (TDD):
1. Migration: saved-filter table — name, view, owner username, param selection,
   shared flag, timestamps. Unique (owner, view, name). FK owner → `users.username`.
2. Datastore CRUD + list-visible-to-user (own + shared). Functional tests against
   `cmm_test`.
3. Web API: list / create / rename / update selection / delete / share, owner-only
   mutations, session username as owner.
4. Reject payload params the target view's parser does not accept — the spec's
   "vocabulary is owned by the view" invariant. Test the rejection.

Acceptance: a saved filter round-trips; a non-owner sees a shared filter but
cannot mutate it; an unknown param is rejected at save.

## Chunk 2 — Nodes view UI

Scope: `frontend/src/pages/NodesPage.tsx`, `frontend/src/api/`, new component
Dependencies: Chunk 1

Steps (TDD):
1. Save-current-selection control + named list of own/shared filters.
2. Applying writes the view's URL params (keeps deep-link/back/bookmark working);
   does not touch sort, page, or global filters.
3. Stale-reference warning on apply ("3 of 20 roles no longer exist").

Acceptance: the 20-role "All Windows OS" cohort survives a reload and a new
session; applying it leaves sort/page/global lens untouched.

## Chunk 3 — Generalise to the other list views

Scope: Roles, Cookbooks, Git Repos pages
Dependencies: Chunk 2

Extract the Nodes control into a shared component once its shape is proven —
not before (the views hold filter state bespoke today; do not invent a generic
abstraction ahead of the second real caller).

Acceptance: same control on all four list views, no per-view special-casing left
in the component.
