# Saved Filters

## TL;DR

A user can name and persist a set of filter selections on a list view, then
re-apply it later. Driving use case: on the Nodes view, save a selection of ~20
roles as "All Windows OS", so an operator stops rebuilding it by hand every
session. Generic across list views, not Nodes-only.

## Intent

Filter selections that take real effort to build (a 20-role multi-select) are
today ephemeral — they live only in the page's URL params. The operator's mental
model is a *named cohort* of the fleet ("All Windows OS", "All RHEL OS", "all
base roles"), which the tool has no way to express. Saved filters give that
cohort a name and a lifetime.

This is persistence and UI only. It adds **no new query capability**: the Nodes
view already filters on a role *list* — `NodeSnapshotFilter.Roles`
(`internal/datastore/node_snapshot_filter.go`), populated from the comma-split
`role` query param (`internal/webapi/handle_nodes.go`). The 20-role filter
already works; it just cannot be kept.

## Model

A saved filter is a **named, owned, view-scoped set of query parameters**.

The stored payload is the view's *existing request contract* — the same query
params the view's URL and its `<x>FilterFromValues` parser already speak — not a
second, parallel schema per view. This is what makes one table serve every list
view: the source of truth for a view's filter vocabulary stays in the code that
owns it (`handle_nodes.go`, `handle_roles.go`, `handle_cookbooks.go`,
`handle_git_repos.go` and their `datastore` filter structs). A saved filter
records a *selection* in that vocabulary; it never redefines it.

Fields (normative shape lives in the Go type and migration once written):

- **name** — operator-supplied, unique per (owner, view).
- **view** — which list view the filter belongs to (nodes, roles, cookbooks,
  git-repos). A saved filter is not portable across views.
- **filters** — the query-param selection (multi-value params carry lists).
- **owner** — the creating user, anchored on `username` (see Ownership).
- **shared** — visible to other authenticated users, read-only.

## Invariants

- **Filters only.** Sort field, sort order, and pagination are *not* part of a
  saved filter. "Which nodes" and "how I'm reading them" are different concerns:
  applying a saved filter must never reorder or re-page the user's current view.
- **Vocabulary is owned by the view, not the saved filter.** A payload may
  contain only params the target view's parser already accepts; unknown params
  are rejected at save time, not silently stored. A saved filter cannot smuggle
  in a filter the view does not support.
- **Global filters are excluded.** The cross-cutting `target_chef_version` and
  `stale_tiers` (`GlobalFilterContext`) are a lens the operator sets
  deliberately, not part of a named cohort. Applying a saved filter must not
  move them. (Target version is single-valued today — see
  `cop-classification.md`.)
- **Ownership by username.** `auth.SessionInfo` carries `Username`, not a user
  id, and username is the established ownership anchor. Renaming or deleting a
  saved filter is the owner's right alone.
- **Shared is read-only.** A non-owner may apply a shared filter and may copy it
  to their own, but may not rename, edit, or delete it.
- **Applying is non-destructive.** Applying a saved filter sets the view's
  filter params and nothing else; it never writes.
- **Names are stable identifiers to humans.** A saved filter's meaning may drift
  as the fleet changes (see below) — the name must therefore never be treated as
  a guarantee about content.

## Stale references

A saved filter names roles (and other entities) that can disappear — during a
migration, that is the *normal* case, and it is signal.

- Stored selections are kept verbatim. A role that vanishes is **not** silently
  dropped from the saved filter.
- On apply, the view filters on what currently exists, and the UI must surface
  the shortfall explicitly: *"3 of 20 roles in this filter no longer exist."*
- Rationale: silently ignoring a missing role changes what the filter *means*
  while keeping its name — the cohort quietly shrinks and the operator is never
  told. Warning turns a rot into a report. (This is the same
  quiet-drift failure class the status/selection work keeps surfacing.)
- Saving is **not** blocked on validation: a filter can rot after it is saved, so
  validating only at save time moves the problem rather than solving it.

## API surface

Authenticated CRUD, owner-scoped, under the existing web API (see `web-api.md`
for conventions — auth, error envelope, pagination):

- list saved filters visible to the caller (own + shared), filterable by view
- create, rename, update selection, delete (owner only)
- share / unshare (owner only)

Applying a saved filter needs **no endpoint**: the client holds the params and
issues the view's existing list request. This keeps the read path unchanged and
means a saved filter cannot diverge from what the view natively supports.

## UI

- On each list view: save the current filter selection under a name; pick a saved
  filter to apply; manage (rename, delete, share) own filters.
- Applying sets the view's filter selection and nothing else — it must not move
  the user's sort, page, or the global lens (see Invariants).
- *How* a view holds that selection is the view's own business, and a saved
  filter must go through the view's existing filter-setting path rather than
  around it. The Nodes view keeps its filters in component state and reads URL
  params only as inbound seeding on mount, then strips them (`NodesPage.tsx` —
  the strip is deliberate: `GlobalFilterContext` shares those params). So a
  saved filter is applied by setting the view's filter state, not by rewriting
  the URL. Saved filters are addressed by id, not by a filter-bearing URL.
- Shared filters are visually distinguished from own filters, and show their
  owner.
- The stale-reference warning appears on apply, not buried in a management
  screen.

## Non-goals

- Not dashboards, charts, or exports — list views only.
- No org scoping, groups, or per-filter ACLs beyond private/shared.
- No scheduling, alerting, or "watch this cohort" behaviour.
- Not a saved *view* (columns/sort/page) — see Invariants.

## Related

- `web-api-filters.md` — the filter query-param vocabulary a saved filter records
  a selection in. **This is the contract; saved filters never redefine it.**
- `web-api-common-patterns.md` — API conventions this feature follows.
- `node-list-enhancements.md`, `node-tags.md` — the Nodes view filter vocabulary.
- `auth.md` — sessions and the username ownership anchor.
