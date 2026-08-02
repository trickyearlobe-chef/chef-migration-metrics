# Ownership — ToDo

## Matching app users to owners (and thereby to git repos / cookbooks / nodes)

Context: a logged-in user's session carries only `username` (+ provider, role) —
NOT `saml_subject`. SAML identity is anchored on the stable `username`
(`username_attr`); `saml_subject` is a volatile login-match token (refreshed each
login, drifts with a transient NameID) and is used nowhere downstream. The hub
for ownership is the `owners` record; `owner_aliases` map identifiers
(`username`, `email`, `git_email`, `git_name`, `custom`) to one owner.

Join key between an app identity and git/commit data is **email**, not username
and not `saml_subject`.

- [ ] **Auto-create `username` alias on SAML JIT login.** Without it the session's
  `username` resolves to no owner. Tag `source='saml'`. (Spec mentions JIT
  auto-link; not yet implemented — see `auth.md` JIT Provisioning.)
- [ ] **Auto-create/ensure `email` alias from the email claim.** Email is the
  reliable bridge to git/commit identities. Auto-link to an existing owner when
  the email matches an existing alias (log the link); else seed it.
- [ ] **Do NOT fabricate `git_email = corp_email`.** Commit email ≠ corporate
  email in many shops (noreply, personal SCM, contractors). Instead:
  - Discover `git_email`/`git_name` from the repo/commit side.
  - Match to owners by **exact email equality first** (commit email == SAML email
    claim → safe auto-link), then fall back to the existing **pg_trgm fuzzy
    similarity** (`owner_aliases.go`) surfaced for **admin confirmation** — not
    silent creation.
- [ ] **Guardrails:** stamp `source` on every auto-created alias (auditable,
  reversible); never overwrite a `manual` alias.

## Owner ingest — what is left

The discovery-driven CSV intake is in (`specifications/ownership-intake.md`).
Remaining:

- [ ] **SQL source.** Journey 5 asks for a database as well as a file: choose a table
  and pick fields, or supply a query and pick fields from what it returns, with a
  preview. `RowSource` was designed for a streaming cursor, so this is a new source
  plus connection and credential handling. PostgreSQL needs no new dependency; MSSQL
  needs a driver and a supply-chain check before any code.
- [ ] **Reuse a saved mapping from the UI.** The mapping CRUD endpoints ship and the
  UI can save one, but cannot yet load one back — a repeat import still re-maps by hand.
- [ ] **Download the match report as CSV.** The report is on screen only.
- [ ] **`policy` entity existence is never confirmed.** CMM collects no policy objects,
  so a policy key always reports as not collected. Harmless — an uncollected entity
  never rejects a row — but the UI should say why rather than implying a miss.

## Blocking a scheduled ingest — merge two people into one

**Reassignment alone does not survive a re-ingest**, and that makes scheduled ownership
ingest unsafe today. `ReassignOwnership` moves assignments and leaves the source owner and
its aliases untouched, so the raw string still resolves to the owner the work was moved
off and the next run puts it back. Both behaviours are pinned by tests in
`internal/webapi/handle_ownership_intake_test.go`.

The durable correction is to move the **alias**, and doing that by hand today means
deleting the old alias first — `uq_owner_alias` is global — then recreating it against the
right owner, then clearing up the emptied owner. Three steps, none obvious, and getting
only the first two right leaves a duplicate person in the catalogue.

- [ ] **Merge action: fold owner A into owner B.** One operation that reassigns the work,
  moves A's aliases onto B (tolerating collisions), and removes A. This is what makes a
  correction stick across a scheduled re-ingest, and it is the natural home for the
  "who owns this? … I wonder if that's Thomas Smith" moment.
- [ ] **Show a person's aliases on their own page.** Owner detail has an *Identity Aliases*
  card that only links out to `/ownership/aliases?owner=<name>` (the link does work and
  auto-loads). But assignments and aliases answer two different questions — *what they own*
  versus *what they are called elsewhere* — and only the first is on the page. Observed
  2026-08-02: the assignments table was read as the alias list, and the conclusion was that
  the alias editor must be somewhere else entirely. Correction is an alias job, so the thing
  you correct with should not be the thing that is a page away.
- [ ] **Owner detail's alias blurb claims "SAML IDs".** The `alias_type` CHECK permits
  `email`, `git_name`, `git_email`, `username`, `custom` — never a SAML subject. This is the
  stale `saml_subject` claim surfacing in shipped UI copy, sending a reader to look for
  something that cannot exist. One string.
- [ ] **Surface it at the point of use**, per the journey-1 refinement in
  `plans/ownership-work-attribution.md`: wherever an owner is read, show the raw string
  and any candidate owners, one action from being merged.
- [ ] **Ingest is additive — decide what a disappeared row should mean.** A row dropped
  from the source never removes the assignment it created, so revoked ownership persists.
  Deleting on absence is dangerous (a truncated export would wipe the estate); a
  "not seen in the last N runs" report is probably the safe form. Needs a decision before
  ingest is put on a schedule.

Dependency on stable username: the whole graph assumes `username` stays stable
(driven by `username_attr` → a stable claim). If `username_attr` is unset,
username falls back to the (possibly transient) NameID and both login anchoring
and ownership matching break. Surface a config warning if `username_attr` is
empty for a SAML provider.
