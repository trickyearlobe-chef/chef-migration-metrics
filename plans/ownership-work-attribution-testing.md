## Testing

**Standing up a database is your problem to solve, not a blocker.** Functional tests need Postgres;
local development already runs one in a container, and the Makefile has the targets. If you need a
throwaway instance — a clean database at a specific migration version, or a second one so parallel
work does not share state — start your own container. A Proxmox MCP server is also available if you
ever need a full VM rather than a container. Do not stop and ask how to get a database.

TDD per CLAUDE.md. Find the existing test harnesses in this repo before writing any test and follow the
closest precedent — mock-store handler tests, query-builder tests asserting emitted SQL, and functional
DB tests behind a build tag. The answer must name a real file in the current tree, not a remembered one.
Check the current functional coverage of the ownership datastore SQL yourself; the point that survives is
that a 10,000-row truncation, two queries against dropped columns and two against tables that have
never existed all reached shipped code because
nothing exercised that SQL against a real database.

### Parity suite, two tiers

Enumerate every call site of the owned-key resolvers and of the in-memory key filter yourself
(`refactor_rename_audit` plus LSP `findReferences`, then grep for string and TS-side uses) and cover each
one. Earlier drafts of this list were incomplete and must not be treated as exhaustive.

**Package placement is a ruling, not discovery.** Confirm for yourself which packages have a
functional-test harness, then: Tier 1 (SQL predicate vs in-memory result) is asserted in
`internal/datastore` against the query builders, where the functional harness already exists. Tier 2
(>10,000, no truncation) is asserted in `internal/webapi` with `mockStore` returning N keys — **no
database, no new harness, no functional build tag in `internal/webapi`.** Standing up a functional
harness in `webapi` for a test that needs no database is days of wasted work.

- **≤ 10,000 owned entities — parity:** the SQL-predicate result set equals the in-memory result set
  element for element, including ordering and `total_count`.
- **> 10,000 — completeness, not parity:** seed N > 10,000 assignments for one owner and assert the
  endpoint returns **all N** and a `total_count` of N, computed from the fixture, never from the
  in-memory path. Above the cap, "parity with the in-memory result" and "returns everything" are
  contradictory, because the in-memory result is *defined* by the truncation.
- The >10k test is **required to fail on `main`** (it returns exactly 10,000); that failure is the
  evidence the truncation was real and is recorded as the red step, not treated as a regression.
- A third case with > 10,000 **owners** pins the all-owners resolver, whose owner listing is separately
  capped, asserting `unowned=true` is complete.

Both tiers required before the SQL predicate replaces the in-memory filter. Locate both resolvers by
symbol name and read their current limits yourself — the line numbers in earlier drafts have drifted.

### Migration test

`0056` does not exist on disk, so do not migrate "to `0056`" — migrate to a **version ceiling** of `56`,
which applies `0001`–`0055` today and keeps working unchanged if the reserved `0056` later lands. Add
`migrateTo(t *testing.T, db *DB, ceiling int)` beside `testDB`. **Contract:** apply every `NNNN_*.up.sql`
whose `NNNN <= ceiling`; the ceiling need not name an existing file.

Read the current migration entry points and pick the simplest ceiling mechanism that uses **exported API
only and changes no production code** — a temp-dir copy of the qualifying `.up.sql` files is the expected
shape. **Do not add a ceiling parameter to production code for a test's benefit**: those entry points are
the application's startup migration path, and a test-driven signature change there has blast radius far
beyond this branch.

**The test must not call `testDB(t)`** — it migrates to HEAD unconditionally, which destroys the test's
whole premise.

Derive the test's steps from `0057`'s own up/down contract in the Chunk A part file. The two properties
that must be asserted: conflicting rows survive the backfill **URL-keyed**, and the down migration
restores **only** rows carrying a non-NULL `legacy_entity_key`.

**Two hard preconditions.** The migration runner skips any migration whose version is `<= MAX(version)`
with **no error**, so a database already at HEAD can never be stepped back to a lower ceiling. The test
therefore requires a **freshly created, empty** database and **must not share one with the package's
other functional tests** — sharing produces a test that silently applies nothing and passes vacuously,
and poisons the shared test database for every other functional package. Give it its own
`CMM_TEST_MIGRATION_DATABASE_URL`, skipping when unset.

### Chunk A

- **Readiness dashboard call count.** With N nodes across O organisations and V target versions the
  invocation count of the counting method is `O × V` and **independent of N**. Red-first: the per-node
  readiness method is invoked **zero** times on the owner-filtered path. Correctness tests pass with an
  N+1 in place; only a call-count invariant catches it. Find the mock's counting methods and their
  fallback behaviour when you write the test — one of them falls back to a sibling, so the test must set
  the specific `…Fn` explicitly or it counts the wrong field.
- Enumerate the early-return and skip branches in the readiness **trend** handler yourself and cover
  each; do not assume any recorded branch set is current or complete.
- **Specified-result tests** for the **three** owner-summary functions, red-first per **each one's
  actual failure mode** — an error for `getOwnerReadinessSummary`, and wrong-but-successful values
  for `getOwnerGitRepoSummary` (every repo `Incompatible`) and `getOwnerCookbookSummary` (every
  cookbook `Untested`). **A red-first test asserting an error will never go red for the latter two**;
  assert the wrong values instead. See the Chunk A part file, which is authoritative here. Then, per the
  normal TDD rule. The one property worth stating: assertions come from a seeded fixture, never from the
  function under test.
- After the git-repo de-duplication restructuring, assert **both the row set and `total_count`** — a
  count computed before de-duplication is the specific failure to catch. De-duplication by name is
  unconditional, so it changes results for callers setting no owner filter, and the tests must pin that.

### Chunk B

All against `mockStore` unless stated.

- `/me/owners` resolves at each tier of the chain in turn: `username` alias, `email` alias, `git_email`
  alias, `owners.contact_email`. **The `git_email` tier is the regression test for the identity gap** — a
  deployment whose owners came from the committers tab has only that alias, so before it was added to the
  chain this case returned empty. Do not collapse it into the `email` tier in a later cleanup.
- A commit address that differs from the session email resolves to **nothing**, and the response is an
  empty list rather than a wrong owner. **This is the deliberate behaviour, not a gap.**
- Cover the nil-dependency and NULL-column paths on the identity endpoint; the invariant is that every
  one of them returns **200 with an empty list** rather than an error.
- Assert the `/me/owners` response shape against the contract in the ownership API spec, not against this
  plan; multiple matches are de-duplicated and sorted, each carrying its `matched_on`.
- Email matching is case-insensitive: a mixed-case session email matches a lowercase alias.
- Committer-assign seeds a **`git_email`** alias (never `email` — a commit address is not a corporate
  one) with `source='git'`, **on the existing-owner branch as well as on create** — the existing-owner
  path is the one that silently produced unresolvable owners. It tolerates `ErrAlreadyExists` from the
  global `uq_owner_alias` by logging and continuing rather than failing the assign.
- Alias values are lowercased **on write as well as on read**: the resolver is byte-exact SQL with no
  `lower()` and no `citext`, so a read-side lowercase alone would pass against the mock and fail in
  production. Assert the manual create path normalises too.
- SAML JIT login seeds `username` and `email` aliases with `source='saml'` **only when the user already
  resolves to an owner by another identity**, and seeds nothing otherwise — **it never creates an owner.**
  Insert is the only write path; there is no update or upsert, so "never overwrite a `manual` alias" is
  satisfied by insert-and-tolerate rather than by a source check.
- Keep the decision that an empty `username_attr` on a SAML provider is a **warning, not an error**; find
  the auth validator and its call sites yourself and follow the warning-capable signature its sibling
  validators already have.
- Check which store methods you need are still hardcoded stubs rather than `<Method>Fn` hooks and convert
  them first; build whatever auth-store test double the identity cases require.
- Test the owner filter-values endpoint against the contract Chunk 0 writes into the ownership API spec —
  including the result cap and the query parameter — and treat the spec, not this plan, as the source of
  those numbers. An owner with zero assignments must be included.
- `savedFilterVocabulary` accepts `owner`/`unowned` for `nodes` and `git-repos` and **still rejects them
  for `roles` and `cookbooks`** — widening the vocabulary uniformly is the natural implementation and
  produces saved filters for entity types with no owner filter behind them.
- `stateToParams`/`paramsToState` round-trip the tri-state, with **"Mine" persisting as the token rather
  than resolved names** — persisting resolved names freezes a shared saved filter to whoever saved it.
- Owner column: a multi-owner entity renders all owners; **a lookup failure blanks the cell and leaves
  the list 200**; the bulk method is invoked **once per `(entity_type, organisation)` group**, asserted by
  call count on the mock — the analogue of the readiness call-count test above.
- Node detail: **a blocking cookbook with no matching repo renders "no known owner" rather than being
  omitted** — omitting it makes the blocker counts quietly wrong.

### Chunk C

Take the scrub and signature vectors and their idempotence property from
`specifications/failure-register.md`, which already holds the contract; this plan must not restate them.
Beyond those:

- Repeat signature increments `occurrence_count` and leaves `diagnosis` untouched; the upsert fills
  `organisation_name`/`cookbook_name` **only from NULL**.
- **An entry survives its `converge_runs` row being purged.** `converge_runs` is range-partitioned with
  short retention enforced by dropping partitions, so a reference-based implementation passes every test
  written inside the retention window and loses data in production days later.
- A `git_repo_name` with no matching repo is accepted and reports `git_repo_known: false`.
- `0058.down.sql` (exec'd directly) **preserves** a `failure_created` row **and** rejects a fresh one.
  Both halves are needed; testing only one lets a destructive down land.

### Chunk D

Org-**scoped** assignment to `org-a`, node name present in `org-a` and `org-b` → `total_nodes == 1`;
org-**agnostic** assignment (`organisation_name` NULL), same two organisations → `total_nodes == 2`. Both
assert **owned + unowned reconciles to the fleet count**. An earlier draft asserted the opposite grain,
so this has already been got wrong once.

### Chunk E

Table-drive slugify, the transforms and delimiter detection from the rules in the import spec, including
a rejection case (`invalid_owner_name`) and an undetectable-delimiter case; enumerate the cases from the
spec's rule set rather than from a list written earlier. Plus `would_create` + `alias_conflict = true` in
one row; a golden match report; **preview writes nothing**. Golden CSVs under
`internal/ownershipimport/testdata/`.

## Verification

1. Run the repo's standard CI and vulnerability targets as defined in the Makefile.
2. Recreate the test database and run the functional suite against it, **plus
   `CMM_TEST_MIGRATION_DATABASE_URL` against a second, empty database for the `0057` test** — without it
   the migration test skips silently and the gate passes with it never having run.
3. Exercise every surface your chunk adds through the running app before declaring it done; derive the
   walkthrough from the chunk's own acceptance criteria. Four checks worth naming explicitly:
   - a git repo assigned via the committers tab is matched by the owner filter;
   - manual register entry works with telemetry disabled, and is accepted (and flagged) for a cookbook
     name with no `git_repos` row;
   - import writes nothing before commit;
   - **the git repos export returns the same 400 as the list for `owner` + `unowned`** — list and export
     are separate handlers, so this consistency only holds if someone asserts it.
4. Verify the ingest-linked "record from this run" pre-fill end to end in a deployment with ingest
   enabled.
