# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

**"What is next" starts with `make journey`.** The reds are the only backlog that is recomputed
rather than remembered, so where they and this file disagree, the reds are right.

## NOW — pick from the reds

`make journey` has two, both in `journeys/own-password.md`: nobody can change their own
password, and nothing tells them the rules before telling them they got it wrong. `make debt`
has one, the settings-shape item that was already there. Nothing is left from the assistant
surface or the strict-body work.

**Where the Chef tools are must apply without a restart.** Saving that directory reports
`restart_required`, because the path is resolved once at startup and the resolved string is
handed to the scanner and the kitchen executor. Configuration is supposed to apply live.

- Scope: `internal/embedded` (the directory read through an accessor, not a field),
  `internal/analysis` and `internal/nodekitchen` (each executor resolves its binary when it
  runs, not when it is built), `cmd/chef-migration-metrics/main.go` (wiring), and the
  analysis-tools handler, which stops claiming a restart.
- Acceptance: changing the directory changes which binary the next scan runs, with no restart;
  the handler reports no restart for it; `make ci` green.
- **The limit worth knowing before starting:** a tool missing at boot means its subsystem is
  never wired at all, so a directory fixed afterwards still needs a restart. Making that live
  too is a separate change to startup gating, not this one.

**What the interface sends is measured, not read.** `make frontend-fields` re-records it with
the TypeScript compiler; `TestFrontend_EverythingTheInterfaceSendsIsAFieldWeRead` holds it
against the served description. That is how three live defects were found before they broke
anything, and how the settings wipe was found at all. Re-run it after changing any request
body — deliberately not part of `make ci`, because regenerating there would make the check
agree with whatever the interface currently does.

## Settled here, kept because the reasoning still binds

**Which tools an assistant is offered is chosen, not generated.** Eight, matching what
`journeys/agent-access.md` says is needed to diagnose a failing cookbook. Generating one per
operation is the obvious move and the wrong one — 249 of them, read flat. Held by
`TestMCP_TheListIsShortEnoughToRead`, which fails if somebody starts generating them.

**No tool queries anything.** Each names a request, dispatched through the same mux a browser
reaches, so access, scope and bounds are inherited. A tool answering from anywhere else would
show an assistant a different estate from the one on screen.

**"The UI tests are green" still cannot clear a change that makes handlers stricter.** 31 of
the 45 page test files mock the API module and nothing drives a real body into a real handler.
The evidence is the comparison above, not the suite.

**A new red can pass while the thing it names is still true.** Assert the baseline first. Two
tests here caught their own fixtures doing nothing — one had never stored the value it then
checked was lost.

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
