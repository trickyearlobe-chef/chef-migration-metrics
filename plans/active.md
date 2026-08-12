# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

**"What is next" starts with `make journey`.** The reds are the only backlog that is recomputed
rather than remembered, so where they and this file disagree, the reds are right.

## NOW — parameters, bodies and response shapes in the description (chunk 4)

**Start here; nothing from the branch's earlier work is needed.** The browsable page is built and
merged into the branch. What it exposes is that the description is thin: measured 2026-08-12 on
249 operations, **81 carry path parameters** (derived from `{name}` segments, cannot rot),
**0 carry a request body** though 106 are writes, **0 carry a query parameter**, and every single
one declares a bare `200: "The answer."`. A generated client from this has no inputs.

**The founding constraint.** The description is derived from what is served, never written beside
it — a hand-maintained table is the trap that killed the 128 specifications. So the answer is
reflection over real types, not a lookup map. `apiRoles` is the one exception and it survives only
because a test probes the running service and fails on disagreement.

**Scope, in order:**

1. **Lift the 22 anonymous `var body struct` declarations to named types**, then reflect them into
   `requestBody`. They live in ownership (4), ownership identity (3), failure register (3), admin
   users (3), ownership import (2), ownership aliases (2), credentials (2), git repos (1), auth
   tokens (1), auth (1). Use `sg` — `var body struct { $$$ }` is a shape, not a regex — and
   confirm call sites with LSP `findReferences`, because anonymous structs are invisible to text
   search.
2. **Query parameters from the shared machinery**, not per route: 24 routes call
   `ParsePagination`, and there are 5 filter helpers. Describe those once and attach them where
   the helper is used. The long tail is 69 direct `req.URL.Query()` reads — leave them until the
   shared sets are done, and count what is left.
3. **Response schemas** last, and the least mechanical: handlers write datastore types through
   `WriteJSON` with nothing declaring which.

**Never pull node-ingest structs into this.** Checked: none of the 22 is in the ingest path. The
Chef attribute data underneath ingest is genuinely flexible — 112 `map[string]any` /
`json.RawMessage` sites — and a named type there turns a real-world shape change into a decode
failure. A body is a candidate only if this service decides its shape.

**The lift pays three times**, which is why it is worth doing properly: the description, a usable
generated client, and shape-drift detection —
`TestJourney_TheShapeCannotChangeUnderACaller` in `internal/webapi/api_integration_journey_test.go`
is skipped today because nothing records what an answer looked like at a release.

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
