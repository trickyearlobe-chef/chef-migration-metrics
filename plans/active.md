# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

**"What is next" starts with `make journey`.** The reds are the only backlog that is recomputed
rather than remembered, so where they and this file disagree, the reds are right.

## NOW — parameters, bodies and response shapes in the description (chunk 4)

**Request bodies and query parameters are done. Response schemas are what is left.**
Measured on the served document: 245 operations, 81 with path parameters, **61 of the 86
POST/PUT/PATCH writes carry a request body** (60 named types under `components/schemas`), and
**23 reads describe their query parameters** — 21 taking `page`+`per_page`, 2 taking `per_page`
alone. The remaining 25 writes read nothing from the body and say so. The `/api-docs` panel shows
body fields, query parameters with their bounds, and a curl built from them. Every operation still
declares a bare `200: "The answer."`.

**The founding constraint.** The description is derived from what is served, never written beside
it — a hand-maintained table is the trap that killed the 128 specifications. So the answer is
reflection over real types, not a lookup map. `apiRoles` is the one exception and it survives only
because a test probes the running service and fails on disagreement.

**Reuse the body machinery rather than reinventing it.** `takes()` / `subTakes()` name a type at
the registration site next to `methods()` — one token, no field table — and `openapi_schema.go`
reflects the shape off it. Tests in `openapi_bodies_test.go` read the handlers and hold the
described and decoded sets in step both ways, so an undeclared body is a red build.

**Carry forward:**

- **Requiredness is not derivable and is deliberately not claimed** — handlers enforce it by hand,
  which reflection cannot see, so the schemas stay silent rather than guess.
- **Three decode idioms exist, not one**: JSON into a named type; `decodeAdminConfigBody` for the
  16 settings sections (read as YAML — the **yaml tag** is the wire name); `io.ReadAll` +
  `Unmarshal`. Any derivation over handlers must know all three or it silently covers two thirds.

**Remaining scope, in order:**

1. **Response schemas** — the last and least mechanical: handlers write datastore types through
   `WriteJSON` with nothing declaring which. **Do not repeat the pagination mistake of deriving
   from the handler**; the unit is the (method, address), and a live probe recording of what each
   address actually answered is available (see below).
2. **The long tail of query parameters.** Pagination is described; filters are not. 69 direct
   `req.URL.Query()` reads remain, commonest keys `q` (7), `repo` (3), `entity_type` (3).
3. **Multipart form fields.** Six addresses take a form, not JSON (`uploadWrites`). Described as
   uploads, but the field names are `req.FormValue` string keys, so nothing reflects them.

**How pagination was settled, because the same method applies to what is left.** The plan's idea —
describe `ParsePagination` once, attach it where the helper is used — does not work. Three
derivations were tried and all over-report: reachability from the registered handler gives 36
patterns against 22; restricting to non-subtree routes still over-reports by seven, because
`handleOwnershipIntake` and `handleOwnershipEndpoints` are each registered at several exact
patterns and dispatch on the path inside; and looking for a `pagination` object in the answer
misses one address that pages without any metadata. So it is declared per address with
`paginated()` / `cappedNotPaged()`, **measured against a running instance**, and held by two
static checks — nothing may claim pagination its handler cannot reach, and no handler that pages
may go unclaimed.

**A read-only probe of a running instance is the tool that settled it.** It walks every GET in the
served description, fills path parameters from the nearest ancestor collection, and tests
behaviour rather than guessing — asking twice, once with `per_page=1`. It records field names and
types only, never values, so its output carries no data. That recording is also what step 3 and
`TestJourney_TheShapeCannotChangeUnderACaller` need, and it does not exist in the repo yet —
deciding where it lives is part of step 3.

**Never pull node-ingest structs into this.** `POST /api/v1/ingest` sits in `undescribedBodies`
with its reason served alongside it, so a reader sees why rather than assuming it was forgotten.

`TestJourney_TheShapeCannotChangeUnderACaller` is **still skipped**: named types unblock it, but it
is about *responses* and nothing yet records what an answer looked like at a release — it needs
step 3.

Renderer research, if the hand-rolled page is ever revisited: `plans/todo-documentation.md`.

## AFTER THAT — the assistant surface

The last red in the agent-access suite: `TheAssistantSurfaceIsBuiltIn`. The service hosts nothing
an editor assistant can connect to, so using it would mean deploying a second thing beside it,
which cannot happen inside the customer estate. Agreed to run on its own branch and thread.

## The top open question is not ownership — it is whether the blocking list is true

Nothing distinguishes a cookbook that fails to converge from a lab that could not authenticate or
hand out an IP, and readiness treats any Test Kitchen failure as incompatible, **overriding a
CookStyle pass**. Measured on the estate 2026-08-03: **89% of the Test Kitchen failure signal was
never about a cookbook.**

**Standing action: turn `tk_blocks_readiness` off at the customer site** while vSphere access is
gone. It ships on, so nothing changes until somebody does it.

The failure register is the only instrument that corrects this today, one repo at a time, and
every entry is also a measurement of how wrong the signal is. Detail and the shape of a fix:
`plans/todo-bulk-kitchen-scanning.md`.

## QUEUED

- Ownership: `plans/todo-ownership.md` — identity matching for "my stuff" is the live one. The
  92% repo-ownership figure is **inflated** (about half are assigned to one person as a stand-in
  for unknown); genuine coverage is nearer 45%. Do not plan against 92% until re-measured.
- Snagging: `plans/todo-snagging.md` — defects in the shipped app, ahead of new work. Reproduce,
  write the failing test, then fix.
- CC19 target-version preset: `plans/todo-event-ingest.md`.
- General audit log: `plans/todo-audit.md`. Note this now overlaps chunk 2 — recording *how*
  something got in is decided; recording *what it did* is not.

## Operational facts that bite

**Next free migration number: 0069.**

**There is no down-migration runner** — only `MigrateUp` — so a schema rollback is `psql` by hand
or a restore. Take a `pg_dump` before any deploy. Two migrations leave a residue an older binary
reads: **0059** (`owner_aliases` back-filled from `owners.contact_email`) and **0063** (git repo
assignments re-keyed from URL to repo name). 0063's down script is **not a true inverse** — it
cannot tell a row it rewrote from one always held by name.

**Release preconditions — the bump target runs no tests.** `make ci` and `make vuln-go` must pass
first; `bump-patch-push` does not depend on `ci`. **Do not set `TRIVY_SKIP_DB_UPDATE=true` for
`make ci`** — it makes the run fail. Trivy rejects that flag alongside `--download-db-only`, which
is what the DB-refresh step uses, so the refresh dies before any scan happens. Plain `make ci`
works; the refresh is already bounded and retried. The push is a human step —
CLAUDE.md forbids remotes, so an assistant may bump and tag locally and push only with explicit
per-action authorisation.

**Dependabot — settled 2026-08-02, do not re-triage.** All 12 alerts were dismissed as inaccurate:
every one named a version outside its own advisory's vulnerable range. The local gates were right
and GitHub was wrong. Do not "fix" it by unpinning `overrides` in `frontend/package.json` — the
parent ranges already permit the patched versions, and several pins exist because the Harness
registry quarantines recent versions.

## Parked — do not propose picking these up

- **Collector streaming** — shared collection path, silent-corruption failure modes, conflicts
  with runtime-observability Chunk 4.
- **Collection history** (`plans/collection-history.md`) and the early `completed_at` stamp that
  rides with it — the duration question is answerable from logs today.
- **Spec/Plan Drift Control** (`plans/spec-drift-control.md`).
- **SAML config follow-ups** — empty `username_attr` warning; local-user username collision
  returning an opaque 500.
