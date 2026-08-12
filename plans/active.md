# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

**"What is next" starts with `make journey`.** The reds are the only backlog that is recomputed
rather than remembered, so where they and this file disagree, the reds are right.

## NOW — what a call answers with, and what it takes (chunk 4)

**The mechanism is built and partly applied. What is left is applying it to the rest.**
An address declares the type it writes back with `answers()` / `answersPage()` beside `takes()`,
and the shape is reflected off that type. `takesQuery()` declares the filters; `takesForm()`
declares the fields of the six form submissions. The `/api-docs` panel shows the answer's fields,
descending into a page's rows.

**Two counts are the backlog, and both are recomputed rather than remembered.** A ratchet in
`openapi_responses_test.go` holds the operations that answer undescribed, and one in
`openapi_filters_test.go` holds the filters described nowhere. Both fail in either direction, so
finishing work means striking the number down. Run them for today's figures rather than quoting
any written here.

**Declare only what has been measured.** `tools/api-probe/probe.py` reads every GET on a running
instance and reports any address sending a field the description does not name. It caught one
wrong declaration the moment it was written. Re-run it after declaring anything, against
`https://127.0.0.1` with a token in `CMM_API_TOKEN`.

**The unit is the (method, address), never the handler.** One handler serves many addresses and
answers a different shape at each, and two are registered at several exact patterns and dispatch
on the path inside. Reachability from the handler is an upper bound and nothing more — that is
what made three attempts at deriving pagination wrong. The addresses left undescribed are mostly
these: each needs its own dispatch read before it can be declared.

**Where the recording lives, decided:** `internal/webapi/testdata/response_shapes.json`, derived
from the Go types and compared on every build, re-recorded deliberately with `-update`. The live
probe's own output is deliberately *not* kept in git: it is read off a running service, and an
object's keys there can be data — a map keyed by organisation or version would put customer names
into the repository.

**Carry forward:**

- **Requiredness of a body's fields is not derivable and is not claimed.** Handlers enforce it by
  hand. A query parameter is different: four addresses refuse a plain GET, the probe measures
  which, and those are declared required.
- **Three decode idioms exist, not one**: JSON into a named type; `decodeAdminConfigBody` for the
  16 settings sections (read as YAML — the **yaml tag** is the wire name); `io.ReadAll` +
  `Unmarshal`. Any derivation over handlers must know all three or it silently covers two thirds.
- **An anonymous map cannot be described.** Roughly half the write sites assemble one inline, and
  each has to be lifted to a named type before its address can declare anything. That lift is the
  bulk of the remaining work, and it is mechanical.
- **Never pull node-ingest structs into this.** `POST /api/v1/ingest` sits in `undescribedBodies`
  with its reason served alongside it.

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
