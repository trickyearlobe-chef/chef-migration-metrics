# Active Plan

**"What is next" is `make journey` and `make debt`.** Those lists are recomputed rather
than remembered, so where they and any prose disagree, they are right.

## Outstanding

- [ ] **Set up a database connection on a running instance and import from it.** Every layer
  has tests and the screen has its own suite, but the whole path has never been exercised end
  to end by a person. Two things cannot be answered by testing: whether a real directory
  account works, and whether a proposed connection is a good enough starting point to be worth
  offering.
- [ ] **Telling CMM where the Chef tools are cannot rescue the case it exists for.** The tools
  are found on PATH, and the setting is there for when PATH is not right. But whether the
  subsystem is built at all is decided once at startup, from whether a tool was found then —
  and "not found at startup" is exactly the situation the setting addresses. So an operator
  whose PATH is wrong sets the directory, the setting saves, the lookup would now succeed, and
  nothing runs, because what would have used it was never wired. Only a restart brings it back.
  **Fix:** decide what to wire from the same live lookup the runs use, not from a boot-time
  snapshot.

## Standing action for a deployment

**Turn `tk_blocks_readiness` off at any site without hypervisor access.** It ships on, and
nothing distinguishes a cookbook that fails to converge from a lab that could not authenticate
or hand out an address — so without a working lab, converge failures wrongly block machines.

## Operational constraints

**Next free migration number: 0070.**

**There is no down-migration runner** — only `MigrateUp` — so a schema rollback is `psql` by
hand or a restore. Take a `pg_dump` before any deploy. Two migrations leave a residue an older
binary reads: **0059** (`owner_aliases` back-filled from `owners.contact_email`) and **0063**
(git repo assignments re-keyed from URL to repo name). 0063's down script is **not a true
inverse** — it cannot tell a row it rewrote from one always held by name.

**Release preconditions — the bump target runs no tests.** `make ci` and `make vuln-go` must
pass first; `bump-patch-push` does not depend on `ci`. **Do not set `TRIVY_SKIP_DB_UPDATE=true`
for `make ci`** — Trivy rejects that flag alongside `--download-db-only`, so the DB refresh dies
before any scan runs.

**Dependabot alerts were triaged and dismissed as inaccurate** — every one named a version
outside its own advisory's vulnerable range. Do not "fix" this by unpinning `overrides` in
`frontend/package.json`: the parent ranges already permit the patched versions, and several
pins exist because the package registry in use quarantines recent versions.

**Re-run `make frontend-fields` after changing any request body.** It re-records what the
interface sends and holds it against the served description. It is deliberately not part of
`make ci`, because regenerating there would make the check agree with whatever the interface
currently does.

## Parked — do not propose these

Collector streaming; collection history; SAML config follow-ups (empty `username_attr` warning,
local-user username collision returning an opaque 500).
