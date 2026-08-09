# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

## Branch map (2026-08-03)

`main` — **v2.18.13** tagged and pushed. Collection is hourly. A few commits sit unpushed on
top of the tag: the git repo owner filter, and the snagging fixes made after it was cut.

`feature/owner-ingest-discovery` — merged into `main` 2026-08-02. **Gate 2 was overridden
deliberately by the product owner**, ahead of the MVP being complete, to get a build deployable
at customer scale and take the measurements below. Later chunks branch fresh from `main`.

`feature/ownership-list-filters` — **merged into `main` 2026-08-03** after sign-off. The
ownership filter on the git repo and cookbook lists (savable as a named cohort, enforced on the
export path), the snagging fixes and migration 0063 found while using it, and the switch that
stops Test Kitchen feeding blocking. 0063 is applied to the dev DB.

**Turn `tk_blocks_readiness` off at the customer site** while vSphere access is gone. It ships
on, so nothing changes until somebody does.

**If a rollback is ever needed:** there is **no down-migration runner** — only `MigrateUp` — so a
schema rollback is `psql` by hand or a restore. Take a `pg_dump` before any deploy. Two
migrations leave a residue the old binary reads:

- **0059** — `owner_aliases` back-filled from `owners.contact_email`; its down script removes
  exactly those rows.
- **0063** — git repo assignments re-keyed from the git URL to the repo name. An older binary
  reads three of those paths by URL, so it would show those repos as unowned again. The down
  script rewrites them back, but it is **not a true inverse**: it cannot tell a row it rewrote
  from one the import always held by name, and the redundant duplicates it removed are gone.

## WHERE THIS WAS LEFT — end of 2026-08-03

`feature/ownership-sql-ingest`, 16 commits, unmerged, tree clean, every gate green. The app on
:443 runs this build; SQL Server and PostgreSQL containers are up and seeded.

**Working and demonstrable:** ownership filtering on nodes, git repos and cookbooks; savable as
a named cohort; enforced on the export path. Import from a file or from a database — browse the
tables a connection can see, pick one or write a query, map the columns, preview, import. The
connection lives in a stored credential, so no password is typed on the import screen.

**Timeline:** screenshots for the change control form on Monday 2026-08-03; the customer wires
it up in **production on Tuesday 2026-08-04**. So Monday is available to close the two below,
and they are worth closing before Tuesday rather than after.

**Not done, and the two that get met in production rather than in a demo:**

1. **Entity type comes from a dropdown, not from a column.** A source table holding several
   kinds of asset must be imported once per kind, using the row filter. Nothing on screen says
   so, and getting it wrong writes assignments of the wrong type — as happened here on
   2026-08-03.
2. **Nobody has watched a commit from a database source write into a real database.** Profile
   and preview are proven end to end against SQL Server through the HTTP layer. Commit uses the
   same seam and the same writer as a file import, and has never been observed.

**Not done, cosmetic or deferrable:** "my stuff" (needs the SAML alias work first — see
`plans/todo-ownership.md`), and the placeholder-ownership question, which is the one that
decides whether unknown ownership matters at all.

## DEADLINE — the ownership MVP was due Monday 2026-08-03

Set by the product owner. **Scope is ownership only.** Deploy access is bureaucratic to
arrange, so the work is batched into one release rather than shipped piecemeal.

Two things are named as not done:

- **Database ingest (SQL Server *and* PostgreSQL) — half done, and the next session picks it
  up from here. Both are first-class sources an administrator chooses between; PostgreSQL is
  not merely the one used for testing.**

  **Done:** `internal/ownershipsql` reads ownership rows from a database as an
  `ownershipimport.RowSource`, so everything above the source abstraction — the row cap, the
  value filter, the distinct-value cap, report truncation — applies to a query result with no
  change. It supports `sqlserver` and `postgres` equally, registers both drivers itself, verifies
  the connection before running the query (an unreadable source must not read as an empty
  one), and renders NULL as empty. The functional tests run against PostgreSQL because it is
  already to hand; the SQL Server path differs only in driver name and connection string, and
  step 4 below covers testing it for real.

  **The dependency is settled — do not re-litigate it.** `github.com/microsoft/go-mssqldb`
  v1.10.0, pinned. It adds **4 modules** to `go.mod` (the driver plus `golang-sql/civil`,
  `golang-sql/sqlexp`, `shopspring/decimal`) and **compiles no Azure or Kerberos code** —
  `go list -deps` shows none. `go.sum` does gain 12 Azure/Kerberos checksum lines, so expect
  scanner advisories about code that never runs; that is the same false-positive pattern as
  the settled Dependabot entries. `govulncheck` is clean. The archived `denisenkom` driver is
  lighter but unmaintained, so it was rejected.

  **Credentials, the endpoints, the UI and table browsing are all built.** The connection
  string is read from a stored credential by name and never accepted from a request, so there
  is no password field on the import screen and it never reaches a browser. Profile, preview
  and commit all take a database because the two sources meet at one function.

  **Table browsing exists because whoever sets this up usually cannot inspect the database.**
  `INFORMATION_SCHEMA` is the same in both, so it is one query; system schemas are excluded,
  views are offered, and choosing one writes a quoted `SELECT` that is then editable — owners
  normally need a join.

  **What is left:**
  1. **Watch a commit write, from a database source, into a real CMM database.** Profile and
     preview are proven end to end against SQL Server through the HTTP layer; commit shares
     the same seam and the same writer a file import uses, but has not been watched.
  2. **Screenshots for the change control form** — not takeable from here.
- **Node ownership — DONE.** The list carries the ownership control, the API already resolved
  it, and the import always accepted `node`. Local data seeded in the dev DB: 8 nodes across 3
  owners, 5 unowned, so both questions show something.

## AWAITING REVIEW — ownership admin move + scheduled database import

Branch `fix/ownership-import-browse-tables`, uncommitted. Asked for by the product owner
2026-08-06, on top of the browse-tables fix already on the branch. Built; the owner has not
seen it yet.

**Decisions taken that outlive this plan, and the reasons, because nothing else records them:**

- **Import and duplicates are admin-only, which reverses two earlier decisions on the owner's
  instruction.** Preview was open to viewers so "the people who own the data can check it"; a
  preview shows the contents of a system of record, and writing nothing is not the same as
  showing nothing. Dismissing a duplicate was operator rather than admin because it removes a
  suggestion, not a person. Both are now admin, in the nav, the routes and the API.
- **A schedule belongs to a saved import, not to a global setting.** Taken as an assumption
  rather than asked. A saved mapping held only the field map, so an unattended run had no
  connection to run against; widening it into a saved *import* (driver, credential name, query,
  row filter, create-owners, cron) is what makes scheduling possible at all, and lets the
  several systems of record this estate has each carry their own cadence.
- **A scheduled run commits.** It writes the same assignments a manual import would and audits
  them under "scheduled import: <name>". Staging for review was not built: a schedule that
  needs somebody to approve it is a reminder, not a schedule.
- **No global on/off switch for the scheduler.** It polls and does nothing when no import is
  scheduled. A schedule the screen shows and a flag silently suppresses is the failure this
  feature exists to avoid.

**No history beyond the last run per import** (the per-row detail is in the ownership audit log).

### Asked for 2026-08-06, while exploring source data quality

Three things, all in service of the same loop: try a source, judge what came back, throw it
away, try again — and hand the source's owner a list of what is wrong with their data.

1. **Run now.** A saved database import can be run on demand. Synchronous, returning the same
   summary a scheduled run records, because the point of it is watching what happens.
2. **Clear out what an import brought in.** Removes assignments whose source is an import, and
   owners those imports created that nothing references any more. **Chosen by the owner over
   the wider and narrower options:** hand-made owners, hand-made assignments, aliases,
   dismissals and the failure register all survive — those cost real effort and are not what a
   trial import dirtied. Owner provenance comes from the audit log (`owner_created` with
   `source: import`), because `owners` carries no source column.
3. **Two exports, for two different readers.** `ownership_corrections` is a fix-list for
   whoever maintains the source system — duplicate people we merged, owners we reassigned,
   details we corrected. `ownership` is the full current state: the shape the source should be
   corrected to match.

**Known gap in the corrections export:** rejected import rows are not in it, because they are
not persisted anywhere — the per-run report is built and discarded. They are the most direct
statement of source data quality there is, so this is worth revisiting.

**The audit log's detail keys are inconsistent between writers, and the corrections export
depends on them.** A merge records `into_owner`; a reassignment records `to_owner`; a dismissal
records `owner_a`/`owner_b`; a deleted assignment records neither and names the owner on the
entry itself. The first version of the export assumed `to_owner` throughout and produced an
empty "should be" column on every real merge — a report telling somebody to fix their data with
the fix missing. A test now asserts the keys against `datastore.MergeOwnersResult` itself rather
than a literal, so a rename in the writer fails a test instead of silently emptying a column.

**Live verification still owed:** no scheduled import has been watched firing against a real
database. The path is covered by tests at every layer and the app runs with the scheduler
started, but nobody has seen one fire.

## NOW — the ownership MVP (`plans/ownership-work-attribution.md`)

Work order and journeys live in that plan; per-chunk scope lives in
`plans/todo-ownership.md`. Do not re-plan either.

**Chunks 1–3 are partly built.** Identity and alias management and the failure register are
shipped. **Owner ingest is not: it reads a file, and the SQL source has never been built.**
Recording it as shipped is what hid the gap — the open item was in `todo-ownership.md` all
along. Behaviour lives in `journeys/ownership-intake.md`, `ownership-identity.md` and
`failure-register.md`. Three decisions from those chunks still bind:

- Ingest **creates** unresolved people rather than rejecting the row, and a fuzzy candidate does
  not reject it either. Correction is deferred to the point of use, by design.
- **Read `journeys/ownership-identity.md` § Proposed before extending aliases.**
  `alias_type` conflates what shape an identifier is with where it came from, and uniqueness
  includes the provenance, so one address can belong to two owners. Recorded, not fixed.
- A human verdict in the failure register **outranks CookStyle and Test Kitchen** and joins the
  existing per-source verdicts on `node_readiness.blocking_cookbooks` rather than sitting beside
  them.

**Node and git repo matching are probably dead — but "matching" means two different things, and
only one of them is dead.** What the measurement retired is *entity* matching: guessing which
repo or node belongs to whom when nobody recorded it. What it says nothing about is *identity*
matching — resolving the several identifiers one person has (SAML email, username, display
name, git email) onto one owner record. That is what aliasing exists for, it is what "my stuff"
needs, and it is open: `plans/todo-ownership.md` § Matching app users to owners. Do not let the
92% figure be read as retiring it.

The measurement itself: **92% of repos carry an owner, and 126 are blocking and unowned** —
but **that 92% is inflated**: the product owner reports about half the repos are assigned to one
person in the Chef team as a stand-in for unknown ownership. Genuine coverage is nearer 45% and
the 126 undercounts, because a repo with a placeholder owner is not unowned. See
`plans/todo-ownership.md`; do not plan against the 92% until it is re-measured.
Both chunks were scoped assuming ownership was largely absent. 126 is a hand-workable list. Full
numbers and the reasoning are in `plans/todo-ownership.md`; do not start either chunk without
revisiting them.

**But the blocking signal itself is the real problem, and it is worse than "untrustworthy".**
Nothing distinguishes a cookbook that fails to converge from a lab that could not authenticate
or hand out an IP. A Test Kitchen failure rolls up to `tk_status = failed`, and readiness treats
any TK failure as incompatible — **overriding a CookStyle pass**. On this estate, where most TK
runs fail on auth or DHCP, lab failures are blocking real nodes. CookStyle offences are
separately reported as badly curated for this work.

**Measured on the estate 2026-08-03: 89% of the Test Kitchen failure signal was never about a
cookbook.** The fix is a config switch that stops Test Kitchen feeding blocking while vSphere
access at the customer site is gone, not a smarter signal. A classifier was built and reverted
the same day as more machinery than the situation needs; it is in git history if Test Kitchen
comes back. Detail: `plans/todo-bulk-kitchen-scanning.md`.

So the 126 **bounds** the unowned work rather than describing it, and the top open question is
no longer ownership — it is whether the blocking list is true at all. Detail and the shape of a
fix: `plans/todo-bulk-kitchen-scanning.md`. The failure register is the only instrument that can
correct it today, one repo at a time, and every entry is also a measurement of how wrong the
signal is.

## NEXT — ownership filtering in the list views

Scope and the decisions that bind: `plans/todo-ownership.md` § Ownership filtering in the list
views.

The git repo and cookbook lists carry the control. Ownership is savable as a named cohort, and
the export path enforces the same rule as the list views. **The node list is in scope for
today** — see the deadline section.

## Snagging (`plans/todo-snagging.md`)

Defects found by the product owner using the shipped app. Faults in what is built, so they come
before new work. Seven found and fixed on 2026-08-02, six of them while importing real data —
none would have come from a code review. Reproduce, write the failing test, then fix.

**Next free migration number: 0065.**

## QUEUED behind the ownership MVP

CC19 target-version preset (`plans/todo-event-ingest.md`) and the general audit log
(`plans/todo-audit.md`). Scope lives there; do not restate it here.

## Dependabot — settled 2026-08-02, do not re-triage

All 12 open alerts were dismissed as **inaccurate**: every one named a version outside its own
advisory's vulnerable range. **The local gates were right and GitHub was wrong**, which is the
reverse of the usual assumption — Trivy scans the lockfile that ships, Dependabot was resolving
the manifest. Do not "fix" this by unpinning `overrides` in `frontend/package.json`; the parent
ranges already permit the patched versions, and several pins exist because the Harness registry
quarantines recent versions.

## Release preconditions (the bump target runs no tests)

1. `make ci` and `make vuln-go` must pass first — `bump-patch-push` does not depend on `ci`.
2. Local `make ci` needs `TRIVY_SKIP_DB_UPDATE=true`: the 1.2GB Trivy DB pull from
   `mirror.gcr.io` stalls mid-transfer (registry handshake is fine; the blob does not
   move). GitHub CI pulls it fresh, so the gap closes on push.
3. The push is a human step — CLAUDE.md forbids remotes. An assistant may bump and tag
   locally, and may push only with explicit, per-action authorisation.

## Parked — do not propose picking these up

- **Collector streaming** — shared collection path, silent-corruption failure modes,
  conflicts with runtime-observability Chunk 4.
- **Collection history** (`plans/collection-history.md`) and the early `completed_at`
  stamp that rides with it — complex and risky for a small gain; the duration question is
  answerable from logs today.
- **Spec/Plan Drift Control** (`plans/spec-drift-control.md`).
- **SAML config follow-ups** — empty `username_attr` warning; local-user username
  collision returning an opaque 500.
