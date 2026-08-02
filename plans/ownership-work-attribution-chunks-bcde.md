## Chunk B — "What's mine" and "what's theirs" (depends on Chunk 0 and A; last item depends on C)

### The identity gap — fix this first, or the chunk delivers nothing

**"Mine" does not work on a real deployment as the code stands, and this is a design hole, not a
detail.** The committer-derived bulk assign is the primary way owners get created here, and it creates an
owner with a contact email but **no alias row**. Find every production caller of `InsertOwnerAlias` and
confirm none is automatic before designing the seeding. Nothing seeds an alias automatically today, so an
`/me/owners` that resolves only through `owner_aliases` returns an empty list for every committer-created
owner, and the headline capability is dead on arrival.

Read `plans/todo-ownership.md` in full before starting this chunk and update it as items land; treat its
identity-resolution conclusions as settled input. Three changes, all required:

1. **Committer-assign seeds a `git_email` alias on every assign, not only when it creates the owner.**
   Read the committer-assign handler end to end and seed on **both** branches — the existing-owner branch
   currently falls straight through to the assignment and never touches `contact_email` or aliases, so
   assigning committers to an owner that already exists leaves it unresolvable. Seed
   `alias_type='git_email'`, `alias_value=<author email>`, `source='git'`. It must **not** be `email`:
   commit email ≠ corporate email in many organisations (noreply addresses, personal SCM accounts,
   contractors), and seeding `email` silently mis-attributes work to the wrong person. Tolerate
   `ErrAlreadyExists` — `uq_owner_alias UNIQUE (alias_type, alias_value)` is **global, not per-owner** —
   by logging and continuing, or the second owner sharing an address fails the assign outright.
2. **SAML JIT login seeds `username` and `email` aliases**, `source='saml'`, **but only against an owner
   that already resolves by one of the other identities**. It never invents an owner:
   `owner_aliases.owner_name` is `NOT NULL REFERENCES owners(name)`, and a JIT-provisioned user
   frequently has no owner record at all. If nothing resolves, seed nothing and let the user link
   themselves — silently minting an owner per login would fill the table with duplicates of real people
   and there is no cheap cleanup. Use LSP `findReferences` on the JIT provisioner's store interface and
   its constructor to enumerate every implementor and construction site before widening it.
3. **`/me/owners` resolves `owners.contact_email`** as a fallback. Confirm no existing datastore method
   queries owners by `contact_email`, then add `GetOwnersByContactEmail(ctx, email string)
   ([]string, error)`; do not assume a listing method can be reused.

### Resolution order, and where it legitimately fails

`username` alias → `email` alias → **`git_email` alias** → `owners.contact_email`. The `git_email` tier
**is** the exact-email bridge: a commit address that equals the corporate address resolves, and one that
does not simply falls through. There is no separate auto-link mechanism to build — including `git_email`
in the lookup is the whole of it, and it is testable as one chain.

**Where a commit identity genuinely differs from a corporate one, resolution fails, and that is
correct** — the alternative is attaching work to the wrong person. The recovery path is the existing
pg_trgm alias suggestion surfaced for **admin confirmation, never silent creation**, reachable from the
empty state of "Mine". Two known populations land here and are not defects: users whose commit address
differs from their SSO address, and admin-created local users, who get no JIT seeding at all.

**Known limitation, mitigated not solved:** the committer UI forces the owner name to the email
localpart, so `a@x.example` and `a@y.example` both become owner `a` and the second person inherits the
first's identity. The arbitrary owner picker added below is the mitigation; **do not** add a
`username`-to-owner-name heuristic to compensate, because it would claim ownership on exactly this
collision.

**Guardrails:** `owner_aliases.source` already exists, so no migration is needed — stamp every
auto-created alias with its origin, and **never overwrite one whose `source` is `manual`**.

**Lowercase both sides before comparing:** `owner_aliases` has no `citext` and no `lower()` index, so
matching is byte-exact today while SSO email casing varies. A read-side lowercase alone passes against a
mock and fails in production.

**Surface a config warning when `username_attr` is empty for a SAML provider.** Without it, username
falls back to a possibly transient NameID and both login anchoring and ownership matching break.

### Endpoints

- **`GET /api/v1/filters/owners`** — follow the existing filter-facet handler and its router registration
  as the template, matching its response shape, `?q=` prefix behaviour and result cap exactly. Source is
  the `owners` catalogue, **not** distinct assignment values — an owner with nothing assigned must still
  be selectable, or you can never filter to "this team has nothing yet". Not organisation-scoped:
  `owners` has no organisation column. Needs a new `ListOwnerNames(ctx, prefix string, limit int)` on
  `DataStore` plus its mock entry.
- **`GET /api/v1/me/owners`** — resolves the session user to owner names. The session carries `Username`
  only; email comes from the same user lookup `/me` already does. Resolution order is the four-tier chain
  above. **Multiple matches are legitimate and are all returned** — the `owner` filter is a union, so
  "Mine" over several owners is meaningful. De-duplicate, sort by name for a stable response.
  **Empty list with 200, never an error**, when nothing matches — the UI says so in words. Enumerate
  every way the email side of the chain can be absent (nil auth store, nullable user email, IdP not
  emitting the configured email attribute) and prove each falls through to the next tier rather than
  erroring.
- **RBAC:** verify against the router that owner names are already readable by any authenticated session
  before settling on no extra RBAC for the two new endpoints.

### Filters, columns, saved filters

- **Tri-state control is a new component.** Check the existing filter components for a mode
  discriminant; if none can express tri-state, build a new component rather than overloading them.
  `owner` and `unowned` are already mutually exclusive server-side (400), and the control makes that
  unreachable rather than relying on the 400.
- **The tri-state is a UI concern only; it must not reach the saved-filter model.** Read the saved-filter
  mapping module to confirm the set of supported param kinds before designing page state; do not add a
  new kind. A `{mode, owners}` object fits neither existing kind and would force a third into a mapping
  engine shared by every list view. Keep page state flat: `owners: string[]`, `mine: boolean`,
  `unowned: boolean`, with the component enforcing mutual exclusion. That serialises through the existing
  model unchanged: `owner` as a list, `mine` and `unowned` as scalars.
- **Saved-filter encoding.** `mine` persists as its own boolean param and is resolved at *apply* time,
  not as owner names frozen at save time — a saved filter should follow the user's identity, and
  resolving at save time freezes a shared filter to whoever saved it. This also avoids inventing a
  sentinel owner name: any literal token stored in the `owner` list would have to be guaranteed never to
  collide with a real owner, and `ownerNameRe` permits almost any lowercase string — there is no reserved
  namespace. `mine` and `unowned` encode as `["true"]`. Locate the saved-filter vocabulary and its
  validator, establish empirically whether values are validated server-side, and put the `true`-only
  check wherever the evidence says it belongs. Add all three params to the vocabulary.
- Check the `saved_filters` `view_name` CHECK for `nodes` and `git-repos` before assuming no migration is
  required. `owner`/`unowned` are params, not views.
- **Owner column resolves direct assignments only**, matching the filter. Using the full
  `LookupOwnership` chain would show an owner the filter cannot match — a visible contradiction. Multiple
  owners render as all of them. Find the existing bulk readiness loader on the nodes list and copy its
  failure-tolerance and per-organisation query grain: a lookup failure blanks the cell and logs, and
  never fails the list.
- **The bulk lookup does not exist and must be built.** Every ownership assignment method is
  single-entity. Add `BulkLookupOwnership(ctx, entityType string, entityKeys []string,
  organisationName string) (map[string][]string, error)` — it **must** return an error, or "a lookup
  failure blanks the cell" has nothing to test against — plus `DataStore` and mock entries. The grain is
  **one query per `(entity_type, organisation)` group**, not one per page; verify the precedent's grain
  in the code rather than from this plan.
- **The Go↔TS lockstep change is much wider than it looks.** Trace the shipped `tags` facet end to end
  (grep **and** LSP, Go and TS) and change every site it touches. Earlier drafts under-counted the file
  list twice, so treat any enumeration as a floor, not a checklist.
- Locate the committer-assign UI block and add an arbitrary owner picker alongside it, without removing
  the existing flow.

### Who fixes this node — **the register leg depends on Chunk C**

On node detail, find the existing `blocking_cookbooks` type and its parser and reuse them; the node
handler already builds the map you need. Take the cookbook name, hop to `git_repos.name` by the
name-equality convention, and resolve that repo's owner. After Chunk A the assignment `entity_key` **is**
the repo name, so no `git_repos` lookup is needed. A blocking cookbook with no matching repo renders as
"no known owner" and is **never dropped silently** — omitting it makes the blocker counts quietly wrong.

This is **two** bulk queries, not one: the blocking set, then the owner set. The **target date leg
requires `observed_failures`, created by Chunk C's migration `0058`** — build the owner attribution first
and add the date when C lands, or sequence B after C.

### Acceptance

A team lead selects Mine on the node list and the git repos list, gets a non-empty result on a deployment
whose owners came from the committers tab **and whose commit address matches their SSO address**, saves
the filter, reloads, and exports it. Where those addresses differ, Mine returns an empty state that
offers to link them to an owner — **that is a pass, not a failure.** Opening a blocked node shows, per
blocking cookbook, the repo holding the fix and its owner.

### Specification

Chunk B adds two public endpoints and changes filter behaviour on two views, so it needs its spec written
by Chunk 0 — `ownership-reporting.md` covers the B/D surface — and it edits
`specifications/web-api-filters.md` and `specifications/saved-filters.md`. **Chunk 0's authorisation flag
covers those two edits; confirm before touching them.**

## Chunk C — The failure register (depends on Chunk 0 and on Chunk A)

**Dependency on A is hard.** `GET /api/v1/failures?owner=…` resolves an owner to `git_repos.name` values
and matches them against `observed_failures.git_repo_name`. Before `0057`,
`ownership_assignments.entity_key` for `git_repo` holds the **URL**, so a name-keyed comparison matches
nothing and the filter returns an empty list with **no error** — the same silent-empty mode A removes. C
must not ship the `owner` filter before `0057` lands.

**A and C both modify `internal/datastore/git_repo_filter.go`** (A: `OwnerNames`/`Unowned` plus the
`DISTINCT ON` restructuring of `buildGitRepoFilterQuery`; C: the `has_*` `EXISTS` pushdown). Land A first
and rebase C; C's emitted-SQL tests are written against A's two-level shape, not the flat query.

### Migration `0058` — `observed_failures`

The full column, type and index listing belongs in `specifications/failure-register.md` (extended by
Chunk 0), derived from the existing migration conventions. What is decided here and must not drift:

- **`UNIQUE (git_repo_name, signature_hash)`** — the register's grain. `signature_hash` is the projection
  defined in `failure-register.md`.
- Three CHECK value sets: `state` (`open|diagnosed|planned|committed|fixed|wont_fix`), `holder_kind`
  (`owner|user|ticket`), `detected_by` (`manual|run_telemetry`).
- **`example_run_end_time` is carried** because `converge_runs`'s PK is `(run_id, end_time)` — `run_id`
  alone cannot locate the row. Evidence is snapshotted, never referenced.

**`organisation_name` and `cookbook_name` are deliberately outside the unique key** — state this in
`failure-register.md` and the assumption log. The grain is one entry per
`(git_repo_name, signature_hash)`: one distinct failure per repo, fleet-wide; the same failure in two
organisations collapses to one row. The operator-authored payload attaches to the *fix*, which lives in
the repo and deploys from git; keying per organisation would fragment one fix into N entries each needing
diagnosis and each kept in step, breaking the "one register entry, one commitment" model.
`occurrence_count` already carries the "seen more than once" signal. Consequence: these two columns record
**first-observation provenance**, not identity, and the UI labels them *"first observed in"*. So a first
observation lacking an organisation is not pinned to nothing forever, the upsert updates them **only from
NULL** — `organisation_name = COALESCE(observed_failures.organisation_name,
EXCLUDED.organisation_name)`, likewise `cookbook_name`. A plain-overwrite upsert silently rewrites
provenance on every repeat observation. Widening the key later is a migration, not a code change.

**Upsert on repeat signature:** increment `occurrence_count`; update `observed_at`, `example_run_id`,
`example_run_end_time`, `example_node_name`, `chef_version`, `cookbook_version` and `backtrace` to the
newest observation; **never touch** `diagnosis`, `remediation_plan`, `target_date`, `state`,
`holder_kind`, `holder_ref`, `detected_by` or `created_by`. A `run_telemetry` observation does not flip a
`manual` row's `detected_by`. This field partition is the whole safety property of the register:
automated ingest must never destroy operator-authored text, and the loss is invisible until someone looks
for their notes.

### Endpoints

`GET /api/v1/failures` — list, filterable by `git_repo`, `owner`, `state`, `has_diagnosis`,
`has_target_date`, `has_commitment`; `POST /api/v1/failures`; `GET|PUT|DELETE /api/v1/failures/{id}`.
Register the collection and item patterns per the existing aliases precedent (ServeMux patterns are
exact — both forms must be registered) and reuse the package's pagination and operator-or-admin helpers;
find them rather than working from citations. Every mutation calls the audit path. The three `has_*`
filters also become `EXISTS` pushdown on the git repos list and on the remediation priority endpoint.

Add the new datastore methods to the `DataStore` interface and its test mock in the same change,
following the package's existing `<Method>Fn` naming and nil-field default-return pattern, and use LSP
`goToImplementation` to confirm every implementor.

### Widen the `ownership_audit_log.action` CHECK

It is an inline, **anonymous** column constraint, so it must be dropped by the name Postgres generated.
Postgres names an unnamed column-level CHECK `<table>_<column>_check`, suffixing only on collision;
`action` carries exactly one unnamed CHECK, so the name is **`ownership_audit_log_action_check`**.
**Verify it in `pg_constraint` on a freshly migrated database before writing the file** and record the
verification in the commit body.

`0058.up.sql` drops it by that exact name — **plain `DROP CONSTRAINT`, not `IF EXISTS`**, which would
silently no-op on a name mismatch and turn the first register write into a runtime constraint violation
instead of a migration failure — then re-adds it under the same name with nine values (`owner_created`,
`owner_updated`, `owner_deleted`, `assignment_created`, `assignment_deleted`, `assignment_reassigned`,
`failure_created`, `failure_updated`, `failure_deleted`). The separate named action/entity constraint
needs **no change**.

**`.down.sql`:** a blanket restore of the narrow CHECK fails against any row already written with a new
action, and `ownership_audit_log` is append-only — **down must never delete or rewrite rows to fit a
constraint.** So `0058.down.sql` drops the constraint and re-adds the six-value form **`NOT VALID`**:
existing rows preserved and readable, new inserts constrained to the six original actions. The asymmetry
is deliberate and is stated in a comment at the top of the file.

Confirm how the audit writer handles an absent `owner_name` against the `NOT NULL` column, and follow
whatever the existing shipped callers do.

### UI and intake

Add a register tab to the git repo detail page following its existing tab enum/bar/dispatch pattern —
note tab state is local, so a deep link to a repo's failures needs the tab lifted into the route. A
register list at `/failures` with columns repo, cookbook, class, state, target date, owner, occurrences.
On the run events page, a "record from this run" action on rows that carry a failed-resource cookbook
name; every pre-filled field stays editable, because the captured failed resource is only the first one.

**`git_repo_name` is `NOT NULL`, filled from the failed resource's cookbook name by the name-equality
convention. When no `git_repos` row has that name, the entry is still recorded.** The cookbook name is
written verbatim; it is a soft key with no FK, matching `git_repos.name` throughout Chunk A. **No 404, no
rejection** — refusing would lose exactly the observation the register exists to capture. The pre-fill
response carries `git_repo_known: false` and the dialog shows *"No matching git repo — no owner will
resolve for this entry"* beside the still-editable field. `owner` filtering and per-owner rollups simply
do not match such an entry; the count of entries with `git_repo_known = false` is itself the useful
figure and is surfaced on the register list. **The only rejection is an empty or whitespace-only
`git_repo_name` → 400 `missing_required_field`**; the UI cannot reach it because of the cookbook-name
guard, but the API enforces it.

## Chunk D — Progress by owner (depends on A; degrades without C)

Read the existing per-owner summary query in full and confirm which duplicate-suppressing properties
already hold before changing it. The defect is **not** duplicate readiness rows — the CTE fixes the
target version and the primary key forbids duplicates within an organisation. It is the
**organisation-blind join** (`r.entity_key = nk.entity_key`), a **join fan-out** under `COUNT(*)`, fixed
by the three bullets in Chunk A's grain decision. The fix is scoping the join by organisation, not
deduplicating rows.

Grain: one row per `(owner, target_chef_version)` plus a synthetic **unowned** row, so the columns
reconcile to the fleet total — that is the property a user will check on the page. Columns: nodes
total/ready/blocked, % ready, repos owned/blocked, and from Chunk C open/diagnosed/dated/past-target
counts. **Write the register CTE so it lands empty and harmless if C is absent.**

Two columns carry the join: **blocked nodes attributed here** (nodes whose `blocking_cookbooks` resolve
to a repo this owner owns) and **dependent node owners** (distinct owners of those nodes). Both are
counts derived from stored relationships and are sortable. **They are outbound figures and will not
reconcile against the owner's own node counts — label them accordingly**, or someone "fixes" correct
numbers.

## Chunk E — Discovery-driven intake (depends on Chunk 0)

Read the existing fixed-header import handler to see what the new source-agnostic path must preserve; it
stays in service unchanged, and that is covered by a regression test. New `internal/ownershipimport/`:
`RowSource`, mapping document, mapper, report classifier — pure, no DB. The **matcher is DB-backed and
lives in `internal/webapi`**, taking lookup results as inputs, so the pure package stays testable without
a database.

**`RowSource` contract:** `Columns() []string` (ordered — a `map` does not preserve order and profiling
needs it), `Next() bool`, `Row() map[string]string`, `Err() error`, `Close() error`. **Single-pass and
not re-readable**, because a future SQL source is a streaming cursor. `/profile` and `/preview` each open
their own source.

**Mapping document** — JSON, persisted in a new `ownership_import_mappings` table created by migration
**`0059`**. It is an API contract stored in a table and outlives the code. Each of six target fields —
`owner` (required), `entity_type`, `entity_key` (required), `organisation`, `notes`, `display_name` — is
specified as exactly two things: **one source**, then **an ordered chain of transforms**. Source and
transform are disjoint; nothing appears in both.

**Source** is a tagged union with three variants, evaluated once to produce the initial string:
`{"kind":"column","column":"<header>"}` (a header absent from `Columns()` is a mapping-validation error,
not a per-row error); `{"kind":"constant","value":"<literal>"}` (row cells not read);
`{"kind":"concat","columns":[…],"separator":"<sep>"}` — the only N-column source, joined in the order
given, an empty cell contributing an empty segment (separators are not collapsed), so the result is
deterministic and testable.

**Transforms** are a `[]Transform`, each strictly `string → string`, applied left to right; none reads a
column. The normative catalogue lives in the Chunk E spec. Two semantics an implementer would otherwise
get wrong: `strip_domain` leaves IP literals **unchanged**, and `regex_extract` yields the **empty
string** on no match or no capture group. Note the `default` transform is deliberately named that rather
than `constant`, to remove the collision with the `constant` **source**.

**`entity_type` is constrained further:** its source must be `{"kind":"constant"}` — it is an enum,
per-row variation from a column is not supported — and the constant must be a value of the live
`entity_type` CHECK; read that constraint in the schema for the authoritative set. Anything else is a
**mapping-validation error at save/preview time**, not a per-row `rejected` with reason
`invalid_entity_type`; that per-row reason survives only for the fixed-header path, where the value does
come from the row.

**Owner-name normalisation — decided: no accent stripping, and therefore no new dependency.** Unicode
decomposition would need `golang.org/x/text`, in neither `go.mod` nor `go.sum` today, for one cosmetic
transformation, and CLAUDE.md requires a full supply-chain check for any new dependency. Slugify is
**not** a transform: it is applied implicitly and unconditionally as the final step of the `owner` field
only. Stdlib only, in this order: (1) lowercase; (2) replace **every** rune outside `[a-z0-9._-]` with a
single `-`, non-ASCII runes not decomposed; (3) collapse runs of `-`; (4) trim leading/trailing `-`, then
any remaining leading `.` or `_`; (5) reject if empty or still failing the owner-name regex. Read that
regex in the handler and the mirroring DB constraint, and assert the slugifier's output against both — a
property test over arbitrary input is stronger than fixed cases.

**Accepted trade-off:** an accented value folds to `-`, not to its ASCII base (`Renée` → `ren-e`). This
costs nothing in matching, because the **raw** string is kept as `display_name` and seeded as a `custom`
alias, so the original is what fuzzy matching and future imports compare against; the slug is only a
stable, constraint-legal handle. `display_name` receives the value *before* slugification — which is why,
absent an explicit mapping, it defaults to the `owner` field's pre-slugify output, not to a column.
Rejection carries a new `rejected_reason` value **`invalid_owner_name`**: a raw string slugifying to
empty (`"???"`, `"---"`) is neither a missing field nor a malformed row, and conflating it with
`missing_required_field` hides the most actionable class of import miss.

### Endpoints

`POST /api/v1/ownership/import/profile` → `{columns:[{name, sample_values, non_empty_pct,
distinct_count}], row_count, delimiter, warnings}`, persists nothing, 10 deduped sample values per
column. `POST …/preview` → full pipeline, **no writes**, returns the report. `POST …/commit` → same
payload, writes. Confirm the existing fixed-header import route stays registered and behaviourally
unchanged, and cover that with a regression test.

**Delimiter detection is advisory only.** The algorithm belongs in the Chunk E spec and is driven from
tests. What is decided: detection is **never binding** — `/profile`, `/preview` and `/commit` all accept
an optional explicit `delimiter`, used verbatim with no detection when supplied, so a misdetection costs
one field edit and never a failed import; **consistency across lines, not raw frequency, is the
discriminator**; and because `/preview` and `/commit` open their own single-pass `RowSource`, each
carries its own `delimiter` — it is not remembered from a prior `/profile`. A persisted mapping stores
the delimiter alongside `field_map`.

**Mapping CRUD** — the table exists so a repeat import needs no re-mapping. Register both the collection
and item patterns per the aliases exact-pattern precedent.

- `POST …/mappings` — create. Body `{name, source_kind:"csv", delimiter, field_map}`. `name` unique;
  collision is 409. Validates the whole document (unknown target field, non-constant `entity_type`
  source, uncompilable `regex_extract`, unknown transform) → 400 with the offending field path. 201.
- `GET …/mappings` — list `id`, `name`, `source_kind`, `created_by`, `created_at`, `updated_at` —
  **not** `field_map`. Paginated with the package's helpers.
- `GET …/mappings/{id}` — full document including `field_map` and `delimiter`.
  `PUT …/mappings/{id}` — replace `name`, `delimiter`, `field_map`; same validation as create; editing
  never re-runs a past import. `DELETE …/mappings/{id}` — 204; referenced by nothing, no cascade.
- `/preview` and `/commit` accept **either** an inline `field_map` **or** a `mapping_id`, never both
  (400, in the spirit of the `owner`/`unowned` exclusion); an unknown id is 404.
- **Writes (`POST`, `PUT`, `DELETE`, `/commit`) require operator-or-admin; `/profile`, `/preview` and the
  two `GET`s need only standard protected auth.** `0059` creates `id`, `name` (UNIQUE), `source_kind`
  CHECK(`csv`), `delimiter TEXT NOT NULL DEFAULT ','`, `field_map JSONB NOT NULL`, `created_by TEXT`,
  `created_at`/`updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`.

### Match report

Keyed by 1-based `source_row`, carrying both raw and mapped values: `owner_match`
(`exact|alias|fuzzy_suggestion|unknown`), `entity_match` (`found|not_found`), `outcome`
(`would_create|duplicate_exists|owned_by_other|rejected`), `rejected_reason`
(`unknown_owner|invalid_entity_type|missing_required_field|malformed_row|invalid_owner_name`).
**`entity_match = not_found` does not reject** — soft references make an uncollected entity legitimate,
and rejecting would break the primary use case: assigning ownership before collection has run. Aggregate
counts per outcome, plus the top-20 unmatched owner strings, downloadable as CSV. Unmatched owner strings
are recorded, not dropped, so an import completes and reports its misses.

**`already_other_owner` is split**, because it carried two unrelated facts a caller could not tell apart:

- **`outcome = "owned_by_other"`** — an assignment exists for this
  `(entity_type, entity_key[, organisation_name])` under a *different* owner. The new assignment is
  **still created**: `ownership_assignments` is many-to-many and overlapping records are legitimate by
  design. The outcome exists so the operator sees the overlap, **not to signal failure** — skipping the
  write silently drops assignments the operator asked for. `existing_owners` carries the names already
  holding the entity.
- **`outcome = "duplicate_exists"`** — an identical assignment for the *same* owner already exists.
  No-op. Find the live uniqueness index on `ownership_assignments` to determine exactly which tuple
  constitutes a duplicate before implementing this.
- **The alias collision is not an outcome at all.** `uq_owner_alias UNIQUE (alias_type, alias_value)` is
  **global, not per-owner**, so the same raw string cannot be seeded as a `custom` alias under a second
  owner. That is a fact about the *alias seed*, not the assignment, and must not suppress or recolour the
  assignment result. Report it as two per-row fields carried alongside whatever the outcome is:
  `alias_conflict` (bool) and `alias_conflict_owner`. When true the assignment is created normally and
  only the alias seed is skipped; the import never fails on it.

A row may therefore legitimately report `outcome = "would_create"` **and** `alias_conflict = true` — the
case a single `already_other_owner` made unrepresentable. **Aggregates are reported per `outcome` and
separately for `alias_conflict`; the two are never summed.**

**`owner_match = "fuzzy_suggestion"` always carries `outcome = "rejected"` with
`rejected_reason = "unknown_owner"`.** A fuzzy match is a suggestion, **never auto-applied**, so no owner
name is resolved and no assignment can be created — the row must not be silently counted as "would
create". It is `unknown_owner` rather than a new reason because the owner genuinely was not resolved;
`owner_match` is what distinguishes "no idea" from "close matches exist", and the two are always read
together. The row carries `owner_suggestions`: at most 3 `{owner_name, score}` entries — reuse the
existing pg_trgm alias-suggestion endpoint and datastore method rather than reimplementing the scoring.
The remedy is to create the alias — or the owner — and re-run; because unmatched strings are recorded
rather than dropped and `/preview` writes nothing, this costs one extra preview cycle and never a partial
import. `fuzzy_suggestion` rows count under `rejected` in the aggregates and appear in the top-20
unmatched strings; `alias_conflict` is not evaluated for them — there is no owner to seed an alias under.
