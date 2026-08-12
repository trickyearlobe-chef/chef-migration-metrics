# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

**"What is next" starts with `make journey`.** The reds are the only backlog that is recomputed
rather than remembered, so where they and this file disagree, the reds are right.

## NOW — the assistant surface, MVP (chunk 5)

**Shipped in v2.22.0: the description now carries what a call answers with, the filters it takes,
and the fields of a form.** That was the prerequisite. This chunk is the thing it was for: an
assistant in somebody's editor that can read what this service knows.

**Read `journeys/agent-access.md` first — it is the requirement, and it is not the integrator's.**
An integrator wants all of it in a shape that has not moved; an assistant wants small answers it
can think about, has to narrow before it fetches, and has to be able to tell what it can ask for
without being told. Building the assistant surface by exposing the whole API is the obvious move
and the wrong one.

**Built into the service, not beside it.** `TestJourney_TheAssistantSurfaceIsBuiltIn` is the red
that starts this, and it already names the addresses it will accept: `/api/v1/mcp`, `/api/mcp`,
`/api/v1/mcp/sse`. Deploying a second process next to this cannot happen inside the customer
estate, so the endpoint is served by the same binary.

**A client is already pointed at it** — user-scoped, outside this repository, at
`/api/v1/mcp` over Streamable HTTP with a bearer token read from `CMM_API_TOKEN`. It answers
"MCP endpoint not found" today, which is the correct state: build the endpoint at that path and
it connects. The token is a credential from an account's own record (`/api/v1/auth/me/tokens`),
carries that account's level and no more, and can be read-only or writing — that machinery is
built and tested, see `internal/webapi/credential_scope_test.go`.

**Open, and not ours to settle alone:**

- Which tools to expose, and whether they are generated from the description or chosen. The
  journey argues for chosen: an assistant picking from 245 operations picks wrong, and the
  journey says so from field reports of that exact failure on other tools built here.
- Whether an assistant may write. The journey says the credential decides, most are read-only,
  and a finding it wrote must never appear under a person's name unmarked — the register already
  records how an entry got in.
- Streamable HTTP against SSE. The client entry assumes the former.

**Breaking the API to fix one of these is allowed.** Nobody holds our description yet, so there
is no external contract to break, and the owner has said so. The gate is the interface: its tests
must stay green.

**But "the UI tests are green" is not evidence that the UI still works.** Measured: 31 of the 45
page test files mock the API module outright, and nothing anywhere drives a real request body
into a real handler. So a change that makes handlers stricter about what they accept cannot be
cleared by that suite — it would pass while the running application broke. What would be
evidence is comparing what the interface actually sends against the types the handlers decode
into, which is a piece of work in its own right and the first step of the unknown-field fix
rather than an afterthought to it.

**A red against an API this surface uses is fixed on the way past, not noted.** Exposing a call
to an assistant is relying on it, and relying on something with a known defect against it while
leaving the defect is how the defect becomes permanent. The reds most likely to be in scope, all
of them measured rather than suspected:

- `TestJourney_SomethingItCannotUnderstandIsRefused` — a body with a field the service does not
  understand is accepted and silently dropped. An assistant sending a nearly-right body is the
  case this was written for: it is told the call worked, and neither side can say what was acted
  on. Expect this one first.
- `TestJourney_NothingIsAdvertisedThatIsNeverRead` — a described body can list fields nothing
  reads. An assistant builds its call from the description, so this is the journey's own "cannot
  tell when it has used one wrongly", arriving by a different route.
- The two ratchets below — an address whose answer is undescribed gives an assistant no model to
  decode into, so exposing one means describing it first.
- `TestDebt_EverySettingsSectionAnswersTheSameShape` (`make debt`) — only if the settings are
  exposed at all.

**Carry forward from the description work, because the same rules apply:**

- **The unit is the (method, address), never the handler.** One handler serves many addresses and
  answers a different shape at each. Reachability from the handler is an upper bound and nothing
  more — three attempts at deriving pagination from it were all wrong.
- **Measure against a running instance.** `tools/api-probe/probe.py` reads every GET and reports
  any address sending a field the description does not name. Re-run it after declaring anything.
- **Two ratchets are the API backlog**, recomputed rather than remembered:
  `TestResponses_TheUndescribedAnswersOnlyGetFewer` and
  `TestFilters_TheUndescribedFiltersOnlyGetFewer`. Run them for today's figures; do not quote a
  number written here. The bulk of what is left is lifting anonymous maps to named types, which
  is mechanical: an anonymous map cannot be described at all.
- **Never pull node-ingest structs into this.** `POST /api/v1/ingest` sits in `undescribedBodies`
  with its reason served alongside it.

Renderer research, if the hand-rolled page is ever revisited: `plans/todo-documentation.md`.

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

**The local instance is running detached, not from a terminal.** It was restarted as
`nohup ./build/chef-migration-metrics --config deploy/pkg/config.yml &` after a `make build`, so
it survives a closed session; `make run` in a terminal does the same thing and blocks. It binds
`*:443` as the user and takes fifteen to twenty seconds before it answers — a check straight
after starting it will fail and mean nothing. Its configuration comes from the database, not
from the file: the file supplies only what unlocks the database. To rebuild: `make build`, kill
the running process, start it again.

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

**Dependabot reported 6 alerts on the default branch at the v2.22.0 push (2 high, 4 moderate) —
not triaged.** The local gates were clean: `make vuln-go` finds nothing this code calls. Treat as
the same class as the settled 12 below until somebody checks.

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
