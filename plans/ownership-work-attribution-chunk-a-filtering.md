## Chunk A — Owner filtering in SQL, and a stable git repo key (blocks B, C and D)

**Gated on Chunk 0's file.** The ownership spec set normatively defines `git_repo` `entity_key` as the
repo URL. Two acceptable orders: **0 lands first** (the owner-model file is removed, the name-keyed
definition written into `ownership.md`; A then has no spec work), or **0 has not landed** (A carries a
one-line spec-correction commit against that definition). Either way the `git_repo_url_pattern` rule
keeps **matching** on the URL — only the `entity_key` it *emits* changes.

### Give git repo ownership a stable key

Enumerate for yourself every site that reads or writes an `entity_key` for `entity_type='git_repo'` —
grep `EntityKey`/`entity_key` across `internal/`, then confirm with LSP references — and change all of
them in **one commit**. Successive drafts said three, then five, and the five-row list still omitted
`internal/datastore/ownership_audit_log.go` and `internal/webapi/handle_remediation.go`. **Assume any
fixed list is short; the count is what you must derive, not inherit.** Known at drafting, as a floor:
the `git_repo_url_pattern` auto-rule, the committer-assign path, the cookbook→git-repo inheritance
lookup (and the `InheritedFrom` block in its response), the fixed-header CSV import, and the freeform
assignment API. Treat unvalidated key shapes on the CSV-import and freeform-assignment paths as
**normalise-or-reject**. Flip readers with writers: a wrong key format returns **empty rather than
erroring**, so a reader left behind is never detected at runtime.

**Why the URL is not a stable key.** All surfaces agree on the URL today, so URL-keying is
self-consistent as long as the URL is stable. It is not. `git_base_urls` is an ordered preference list
and `reconcileOrigin` re-points a clone at a higher-preference base once the repo appears there.
Re-hosting writes a new row, and `DeleteStaleGitRepos` removes rows for that name at other URLs,
cascading the FK'd tables and explicitly cleaning committers because they have no FK.
`ownership_assignments` has no FK and **nothing cleans it**, so its rows survive pointing at a URL no
longer present. The owner stops resolving, with no error. Confirm both functions and their call sites
are still live; the count of cascading tables is not load-bearing, the absence of cleanup is.

`name` is a **soft key**: `0009_natural_keys` dropped the unique constraint on `git_repos.name` and made
the PK `(name, git_repo_url)`, and one-row-per-name is maintained only by the `DeleteStaleGitRepos` call
site, which does not run on the clone-failure path — so a moved-then-unreachable repo can retain both
rows. Confirm the PK and the absence of a unique constraint from the migration yourself. A de-dup-free
query, or a migration CTE assuming uniqueness, can produce two rows per name.

### Migration `0057`

- Rewrites `entity_key` to the repo name for `entity_type='git_repo'`, resolving via `git_repos` on URL.
  Where a URL matches more than one row, de-duplicate on the **lookup key — the URL** — with the
  `DISTINCT ON` expressions leading the `ORDER BY`, as a CTE the `UPDATE … FROM` joins on
  `ownership_assignments.entity_key = m.git_repo_url`:

  ```
  SELECT DISTINCT ON (gr.git_repo_url) gr.git_repo_url, gr.name
    FROM git_repos gr
   ORDER BY gr.git_repo_url, gr.last_fetched_at DESC NULLS LAST
  ```

- **Never `UPDATE` blind.** `idx_ownership_assignments_unique` covers
  `(owner_name, entity_type, entity_key, COALESCE(organisation_name,'__none__'))`. Two URLs for one owner
  resolving to one name raise a duplicate-key error, roll back and **exit the process — the service will
  not start.** Rewrite only non-conflicting rows; leave conflicting rows URL-keyed and log the count.
  **Never delete an ownership row.** Repos already re-hosted have no row at their old URL, cannot be
  resolved, are left and logged; no substring or fuzzy guessing.
- Add `legacy_entity_key TEXT`, populated only for rewritten `git_repo` rows. `.down.sql` restores
  `entity_key` from it **only where non-NULL**, leaving post-migration rows untouched — `entity_key` is
  `NOT NULL`, so a blanket restore fails. `.down.sql` drops the column last.
- **Observable for tests:** the migration runner returns no per-row detail, so `0057` must leave
  conflicting rows identifiable by a URL-shaped `entity_key`. Derive the runner's signature and the
  URL-shape regex yourself.

**Land the Go changes in the same commit as `0057`**, or a deploy between them lets the next collection
run re-create URL-keyed rows. Two second-order consequences inside the same functions: the auto-rule's
**dedup set changes from the URL to the name** (or a repo with two URL rows emits duplicate matches,
which then trip the unique index); and `getOwnerGitRepoSummary`'s query is broken against dropped id
columns — establish **how** it fails before writing its test (see the owner-summary section below),
then rewrite it against the composite natural key with a `name` predicate.

### One row per repo name in the git repos query

`buildGitRepoFilterQuery` emits a plain `SELECT` with no `DISTINCT`, so the transient two-rows-per-name
case returns duplicates. **The de-duplication is unconditional, not conditional on an owner filter** —
the same builder serves the list handler and the export, so this changes results for callers that never
asked for an owner filter, including every existing export. Deliberate: two rows for one name is a
pre-existing display defect, and a row count that changes depending on whether an unrelated filter is set
is worse than either behaviour consistently. The surviving row is the one with the most recent
`last_fetched_at`.

**`DISTINCT ON` alone does not fix `total_count`:** Postgres evaluates window functions *before*
`DISTINCT`/`DISTINCT ON`, so `COUNT(*) OVER () AS total_count` would still count pre-de-duplication rows;
and `DISTINCT ON (name)` forces `name` to lead the `ORDER BY`, colliding with the user-selectable
`Sort`/`SortOrder`. Structure it in two levels: an inner CTE that de-duplicates
(`SELECT DISTINCT ON (name) … ORDER BY name, last_fetched_at DESC NULLS LAST`), and an outer query
carrying the `WHERE` clauses, the ownership `EXISTS`/`NOT EXISTS` predicate, `COUNT(*) OVER ()`, the user
`ORDER BY`, `LIMIT` and `OFFSET`.

### Owner filtering on the git repos list is new public API surface, and A introduces it

A owns it (not B) because A already owns the `DISTINCT` and the `EXISTS` predicate on this query; B is
then purely the facet endpoint, the tri-state control and the owner column.

- **Parameters:** reuse the vocabulary verbatim — `owner` (comma-separated names, union) and
  `unowned=true`.
- **400 mutual exclusion inherited, not reimplemented.** Reuse the existing `parseOwnerFilter` /
  `validateOwnerFilter` pair and copy the call ordering from a handler that already does it correctly;
  assert the message in a test rather than restating it here.
- `gitRepoFilterFromValues` populates new `OwnerNames []string` / `Unowned bool` on `GitRepoFilter`
  straight from `url.Values`. No pre-resolution to entity keys — the predicate is `EXISTS`/`NOT EXISTS`.
- **The export follows in the same change.** The git repo export source calls the same
  `gitRepoFilterFromValues`, so it silently inherits the filter *and* the invalid combination, and
  returns 200 on `owner` + `unowned` unless the validation pair is added to the exports handler too. Both
  surfaces must return the same 400. Correct the now-false export comment and the handler doc-comment.

### Push the owner filter into SQL

An owner-filtered node request abandons SQL pagination and loads the whole matching set into Go. Both
existing resolvers carry hard **10,000-row** limits and issue one query per owner, so large owners are
silently truncated — the per-owner resolver caps the key set, and the all-owners resolver lists owners at
the same cap. Locate both, confirm the limits still stand, and rewrite both over the new single-query
helper, or `unowned=true` stays truncated everywhere.

New `ListOwnedEntityKeys(ctx, entityType string, ownerNames []string) ([]string, error)` — one query, no
limit. **Contract:** empty or nil `ownerNames` means *all owners*, returning every owned key for that
entity type; it **never** means "no filtering". Deliberately opposite to the nil convention in
`resolveOwnershipFilter`, so the helper is never passed a nil that could read as "match everything
downstream" — callers translate explicitly. Backwards, this inverts the unowned filter and returns
everything instead of nothing.

Add it to the **`DataStore`** interface (there is no type named `Store` in this package) and to the test
mock in the same commit; LSP `goToImplementation` for every implementer, since Go satisfaction is
implicit. Chunks C and E edit the same file.

Add `OwnerNames []string` / `Unowned bool` to the node-snapshot, cookbook and git-repo filter structs;
converted endpoints emit an `EXISTS`/`NOT EXISTS` predicate from these, residual sites keep resolving key
sets through the helper. Run `refactor_rename_audit` before deleting `handleNodesWithOwnerFilter`, and
remove every inline owner branch on the endpoints you convert — find them, don't trust a list.

**Preserve organisation semantics exactly.** There is none in the filter today — both resolvers leave it
empty, and an empty `OrganisationNames` already means all orgs. Do **not** add organisation scoping while
you are in here, however tempting once you are building an `EXISTS` over a table carrying
`organisation_name`.

### The three broken owner-summary functions — and the table that does not exist

**Three** owner-summary functions in this file are broken, and they do not all fail the same way.
Some error; some swallow the error internally and return confidently wrong numbers. **Establish each
function's actual failure mode from the code before writing its red-first test** — a test asserting
an error can never go red against a function that returns wrong values instead, and "fixing" the test
to make it pass ships the defect unpinned. Earlier drafts of this plan miscounted these functions
twice and misattributed their causes once; do not trust any enumeration, including this one, without
opening the file.

**`cookbook_complexity` and `cookbooks` are not tables and never have been.** The tree has
`server_cookbooks`, `server_cookbook_complexity`, `cookbook_usage_analysis`,
`cookbook_usage_detail`, `cookbook_platform_coverage` and `git_repo_complexity`. Two functions in
`internal/datastore/owners.go` query the phantom pair, join on a `cookbooks.id` that was dropped,
and swallow the resulting error — so in production one has always returned a blank
`complexity_label` and the other has counted **every owned cookbook as Untested**. Neither surfaces
an error; both look like empty results.

**Decide the replacement source by reading the code, and record why.** This is the one place in this
plan where the natural implementation is plausibly wrong and a green test would not catch it, so do
not guess. Establish: what `node_readiness.blocking_cookbooks` entries actually carry (name only, or
name plus version, and whether they are organisation-scoped); which complexity table has keys that
match that grain; what the remediation priority list already uses for the same concept; and what
`frontend/src/types/ownership.ts` consumes, since dropping the field silently narrows a response the
UI already reads. Pick the source whose key matches the grain of the thing being labelled and say so
in the commit message. Dropping the field is available but is the option most likely to be wrong.

### The failure modes, which differ

**`getOwnerReadinessSummary` — errors and is swallowed into a WARN.** Against a dropped
`node_readiness` id column, for every owner holding at least one `node` assignment (an owner with
none returns early). Owner detail silently shows no readiness — it looks like an empty result, not a
bug. It also binds one parameter per owned node and breaks past the 65,535 wire limit. Rewrite
set-based against the current `node_readiness` PK, fold in the per-cookbook loop, and surface the
error as a **500**. Its per-cookbook label step is the phantom-table query above.

**`getOwnerGitRepoSummary` — does NOT error.** It references dropped id columns, but the error is
swallowed *inside the function*, which returns `nil` and a summary counting **every owned repo as
Incompatible**. The handler emits a 200 with no WARN. So a red-first test asserting an error will
never go red — assert instead that every repo comes back Incompatible regardless of the fixture.
Decide, and record, whether this one should also become a 500: that changes owner detail from
silently-wrong-200 to a visible error.

**`getOwnerCookbookSummary` — does NOT error either**, and the earlier draft missed it entirely. Same
phantom tables, error swallowed, **every owned cookbook counted as Untested**. It sits in the same
file and is called from the same handler, so you will hit it while building the fixture. Fix it in
this chunk: leaving one of three broken while repairing the other two is worse than either.

**Acceptance criterion — parity does not apply to functions that are broken today.** For all three
the criterion is a **specified-result functional test**: seed a fixture with known counts and assert
exact values, plus a red-first test matching each function's actual failure mode — an error for the
first, wrong-but-successful values for the other two. Parity is the criterion only for functions that
return a usable result today.

### Dashboard readiness

Collapse the owner-filtered branch onto the same counting function the unfiltered fast path already
calls, extending its signature with `ownerNames []string, unowned bool` and emitting the same
`EXISTS`/`NOT EXISTS` predicate; delete the `if ownedKeys != nil` fork and fix any stale comments you
pass. Read the current branch rather than working from a walkthrough — it is the worst path in the set.
**Preserve the existing `latestReadinessForOrg("$1")` scoping**, without which every historical
collection cycle's rows are counted: inflated counts, no error, no test failure unless a fixture has
multiple cycles.

### Residual in-memory sites

Enumerate every remaining in-memory ownership filter yourself — grep for calls to the two resolvers and
to `ownershipInclude`/`filterByOwnershipKey` — and cover each in the parity suite. An earlier draft
presented its list as exhaustive and was not; note some sites (remediation) call the truncating resolvers
**directly**, bypassing `resolveOwnershipFilter`.

`handleDashboardReadinessTrend` **stays in memory by design** — it filters a node-name array inside a
stored `readiness_summary` metric-snapshot JSON payload, not live `node_readiness` rows, so there is no
predicate to push down. Explicit carve-out from "push the owner filter into SQL". Its parity test must
also cover the `payload.NodesOmitted || payload.Nodes == nil` skip, which drops a trend point entirely
when the filter is active. Treat any further site found during implementation as a defect in the list and
add it, rather than converting it opportunistically.

### Ownership counting grain — decided: `(organisation_name, node_name)`

Applied identically to `getOwnerReadinessSummary` and `listOwnersWithSummary`. A node *name* in two
organisations is **two nodes**, because the fleet is keyed that way and the synthetic unowned row must
reconcile to the fleet total; distinct-name counting under-counts the fleet and breaks the
reconciliation. The owner-rollup defect is therefore **not** "counts twice" — it is a **join fan-out**
under `COUNT(*)`:

- `node_keys` carries `oa.organisation_name` alongside `oa.entity_key`; `readiness` carries
  `nr.organisation_name` alongside `nr.node_name`.
- The predicate becomes `r.node_name = nk.entity_key AND (nk.organisation_name IS NULL OR
  r.organisation_name = nk.organisation_name)` — an org-scoped assignment matches only that
  organisation's node; an org-agnostic assignment (`organisation_name IS NULL`, permitted by the
  `COALESCE(…,'__none__')` unique index) legitimately covers the node in every organisation it appears in.
- Aggregates become `COUNT(DISTINCT (r.organisation_name, r.node_name))`, not `COUNT(*)`, because
  `ownership_assignments` is many-to-many and one owner may hold both an org-scoped and an org-agnostic
  assignment for the same node.

Do **not** "fix" this by adding a `DISTINCT` on node name — that is the opposite of the decision.
