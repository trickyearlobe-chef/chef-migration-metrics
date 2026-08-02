## Chunk 0 — Specifications (blocks A, C, D and E)

Run `wc -l specifications/ownership*.md specifications/web-api-ownership.md` yourself before deciding
what to touch; note that the `ownership*.md` glob both includes `ownership.md` (rewritten, not removed)
and misses `web-api-ownership.md`.

**Actually stale.** Re-derive every ownership PK, FK, cascade and column-type claim from the applied
migration set at HEAD before restating it in a spec — the pre-`0009_natural_keys` UUID-PK shapes are
still asserted in several specs, and every line citation in this plan predates the current tree.

**`ownership.enabled` exists nowhere in code** — `OwnershipConfig` has no `Enabled` field. This is the
premise of the whole rewrite: it invalidates `ownership-integration.md` wholesale, and the same claim is
asserted in several specs outside the ownership set. The real gate on auto-derivation is `len(rules) == 0`.

**What must survive** — every still-normative behaviour gets a named destination:

- **Auto-derivation:** read `internal/collector/ownership.go` end to end and document each rule type's
  normative behaviour from the code, noting that `cmdb_attribute` is rejected by `evaluateRule` and runs
  end-to-end on its own path. Cite `file:line @ <short-SHA>` as you go.
- **Resolution order:** `direct` → `git_repo_inherited` → `policy_inherited`; source precedence
  manual > import > auto_rule. Soft references, no FK — an assignment may legitimately name an entity
  that was never collected, so a `LEFT JOIN` NULL means "not collected", not "no owner".
- **Audit-log contract:** read the audit-log table definition and its CHECK constraints from the
  migrations, and the purge wiring from startup, then record the action enum and the details-by-action
  shape as the contract. Chunk C's new `failure_*` actions must satisfy the existing action/entity
  CHECK, so verify that constraint's current text rather than trusting any citation.
- **Committer collection:** trace the committer collection path from the git fetch cycle and record the
  replace-in-full semantics as an invariant, locating the functions yourself.
- **Web API:** enumerate the live ownership endpoints from the router and cover each one in the
  rewritten API spec; treat any list written earlier as incomplete. The spec set has **no** entry for the
  four alias endpoints (`/ownership/aliases`, `/aliases/`, `/aliases/import`, `/aliases/suggest`) —
  closed here, not widened.
- **Export:** determine from the export handlers which ownership filters and columns actually ship, and
  drop anything unshipped rather than restating it. Do not assume the filter/column split an earlier
  draft described still holds.
- **Management UI:** enumerate the shipped ownership routes from the frontend router and nav, and decide
  per visualisation section whether it ships — earlier drafts of this plan contradicted themselves on
  whether the auto-rule status view (§5.4) exists.

**Remove (5 files) — confirm against the tree first and stop if it differs.** Removal is its own commit,
listing each file, its destination and a one-line reason.

| File | Destination for normative content |
|---|---|
| `ownership-owner-model.md` | owner model, entity-key formats, resolution order, soft-reference invariant → `ownership.md` |
| `ownership-datastore.md` | audit-log action + `details`, committer replace-in-full, retention → `ownership-operations.md`; soft-reference rationale → `ownership.md` |
| `ownership-api-2.md` | §4.3 import, §4.4 audit-log endpoint → `ownership-api.md`; §4.5 owner filter → `ownership-reporting.md` |
| `ownership-integration.md` | retention key + env override → `configuration-schema-server.md`; committer/auto-derivation triggers → `ownership-operations.md`; export filters → `ownership-reporting.md`; export columns dropped |
| `ownership-visualisation.md` | §5.4 (as shipped) and §5.1-5.3 (as Chunk B/D intent) → `ownership-reporting.md` |

**Rewrite in place (5 files):**

- `ownership.md` — index plus owner model, entity-key formats, resolution order, alias model absorbed
  from `auth.md`, soft-reference and no-FK invariants; contracts referenced, never pasted.
- `ownership-api.md` — endpoint inventory, auth level and invariants per endpoint, response shapes
  referenced to the handler; **adds the four alias endpoints**; records that **the assignment id is an
  integer, not a UUID** (the delete handler parses it with `strconv.ParseInt`, so a spec — or a frontend
  built from it — that documents a UUID produces a 400 on every delete).
- `ownership-auto-derivation.md` — all **seven** rule types including `cmdb_attribute`; drops the
  `ownership.enabled` gate.
- `ownership-operations.md` — audit-log contract, retention and purge, owner-delete cascade, committer
  collection and replacement, the import row cap. Verify each cascade claim against the current schema
  before restating it, and confirm the import row cap from the import handler rather than from this plan;
  the "organisation deletion cascades" claim is against an FK that was dropped.
- `web-api-ownership.md` — **kept, not removed**: it is `web-api.md`'s split-index stub, so deleting it
  breaks the thin-index-plus-parts convention and forces an edit outside the ownership set for no gain.
  Rewritten as the endpoint index, aliases included.

**Write new (2):** `ownership-intake.md`, `ownership-reporting.md`. `specifications/failure-register.md`
already exists — Chunk 0 **extends** it, it does not author it (see the register decisions below).

Find the `git_repo` `entity_key` definition by text search across the ownership specs. That file is in
the removal set, so Chunk A's URL→name change lands consistently in `ownership.md`; if removal is
deferred the definition must still be corrected (CLAUDE.md forbids silent divergence).

**Configuration framing.** Confirm `ownership` is a config-store section in the assembly code, then
strike every "YAML configuration file" framing you find in the ownership specs: the rewrite states that
ownership config lives in the DB and is edited via the UI. (Auto-rule config is passed by value at
startup rather than through a live accessor — record in `plans/todo-tech-debt.md`; not spec work.)

**Spec-versus-algorithm constraint.** Read `.githooks/pre-commit` for the exact staged-spec checks
before writing; the behaviours matter, the section and line numbers will have moved. The two that shape
this chunk:

- A hard line cap on any staged `specifications/*.md`, enforced as a block.
- An intent-not-implementation check that **blocks** on `:=`, `^return <expr>`, `^if|for|switch|range`,
  `} else` inside a `go|ruby|rust|python` fence, **warns** on a long run of CONSTANT-style lines outside
  any fence, and **warns** on a `struct`/`interface` block inside a `go|typescript|ts|tsx` fence. **An
  unlabelled fence is exempt from all three.** So the scrub rule and hash canonicalisation go in as
  **invariants in prose plus a pattern table in a plain fence as reference data**, pinned by a **golden
  test vector in code**. Neither alone suffices.

**Decisions Chunk 0 must record — settled; specify, do not re-open.** The normative scrub rule is
**already written**, as `specifications/failure-register.md`; Chunk 0 extends that file with the rest of
the register spec rather than authoring the signature contract. Summary: tokens
`<path>`/`<host>`/`<user>`, applied to `failure_message`, `failed_resource`, `backtrace`, never to
`diagnosis`/`remediation_plan`; POSIX and Windows both in scope; idempotent, and that is a test. Order:
**scrub, then cap (512 bytes, UTF-8 rune boundary), then hash.** Projection
`sha256_hex(failure_class ␟ failed_recipe ␟ failed_resource ␟ scrubbed_capped_message)`, `\x1f`
delimiter, NULL and empty string indistinguishable by design; `git_repo_name` is **not** in the
projection — it sits beside the hash in the unique index. `has_commitment` means `state = 'committed'`.
Audit actions `failure_created`/`failure_updated`/`failure_deleted`, written with
`entity_type='git_repo'` and `entity_key=<repo name>`, satisfying the action/entity CHECK.

**Ruling on `saml_subject`: remove the claim from `auth.md`. Do not add a migration.** `auth.md` lists
`alias_type` values the CHECK has never permitted and omits three it does. Correct `auth.md`, because:

1. `saml_subject` is a **login-matching** key on `users`, deliberately refreshed on every login by the
   transient-NameID fallback; and since `uq_owner_alias UNIQUE (alias_type, alias_value)` is **global**,
   a transient-NameID IdP would mint a fresh alias per login and strand every prior one. Widening the
   CHECK is the tempting repair and is exactly wrong: it produces unbounded stranded alias rows in
   production, is invisible to any test with a stable NameID, and is expensive to unwind once aliases
   exist.
2. The stable anchor the code already uses is `username`, and `owner_aliases` carries `username` and
   `email` types, which is how design decision 5 resolves "mine".
3. Use LSP `findReferences` on the alias resolver before asserting in a spec that nothing resolves
   owners by alias today; grep alone will miss interface-dispatched callers.

`auth.md`'s **"Case-insensitive"** claim is struck: `0030` has no `citext` and no `lower()` index, so the
constraint is case-**sensitive**. Chunk B's "mine" resolution matches a session username and email
against `owner_aliases`; if the spec says case-insensitive, B is built assuming a match that will not
happen for differently-cased email — a silent wrong answer, not a failure. The IdP-prefix rule goes with
it. Locate each remaining `auth.md` claim by its text, not by line number (they have already drifted),
and verify the authorisation model is role-only from the handler middleware before striking the
Ownership-Scoped Permissions block and the ownership step in Permission Resolution.

**Cross-reference fix-ups.** Re-run the sweep yourself for each removed filename, `ownership.enabled`,
`CMM_OWNERSHIP_ENABLED` and every `§ N` link across `specifications/`, `plans/` and `README.md`. Earlier
drafts of this plan carried a table that was **not** exhaustive (it omitted `ownership-operations.md`'s
own `ownership.enabled` assertion) and whose line numbers have moved. `overview.md` and `README.md` both
carry the spec routing table and gain rows for the new specs. Replace every `§ N` reference with a
named-section link — section numbers do not survive the rewrite, and nothing in the tree validates a
`§ N` link, so the breakage is silent.

`plans/todo-ownership.md` is updated per CLAUDE.md § Specifications; it needs no link repair.

**Authorisation — GRANTED.** Editing `data-collection.md`, `configuration-env-overrides.md`, `configuration-schema-server.md` and `visualisation.md` to strip references to `ownership.enabled` is authorised; the flag exists in no Go code. This covers those four files and this edit only.

That grant, plus the removal set, is what CLAUDE.md's *"Do not modify specs without asking"* requires
here. `overview.md` and `README.md` are also touched, but only to add or repair routing rows for
files this chunk creates or removes — index maintenance, not a change of meaning — so proceed.
Leaving `ownership.enabled` asserted in any spec is the silent divergence CLAUDE.md forbids, so the
sweep is not optional.

**Acceptance.** No `specifications/*.md` references a removed filename or `ownership.enabled`. Every
behaviour under "What must survive" resolves to exactly one spec file. All seven auto-derivation rule
types and the four alias endpoints are specified. `auth.md`'s `alias_type` matches the CHECK or
references it. Every spec is under the line cap and passes both pre-commit spec checks.
