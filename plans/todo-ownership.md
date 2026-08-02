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
- [ ] **Ingest is additive — decide what a disappeared row should mean.** A row dropped
  from the source never removes the assignment it created, so revoked ownership persists.
  Deleting on absence is dangerous (a truncated export would wipe the estate); a
  "not seen in the last N runs" report is probably the safe form. Needs a decision before
  ingest is put on a schedule.

## Identity and alias management — what is left

- [ ] **Surface it at the point of use**, per the journey-1 refinement in
  `plans/ownership-work-attribution.md`: wherever an owner is read, show the raw string
  and any candidate owners, one action from being merged.
- [ ] **Rules for platform identities, bots and maintainers — write them against real data.**
  Findings and measurements: `specifications/ownership-identity.md` § Git identities are
  rewritten by the hosting platform. Deliberately deferred, and safe to defer because git
  history is re-derivable from the clones on disk.

  Cheaper and already collected: `commit_count`, `first_commit_at` and `last_commit_at` are
  on every committer row and nothing in ownership reads them. Journey 1 asks for how much
  and how recently somebody contributed; recency and concentration need no collector change
  at all.
- [ ] **Re-model aliases: shape is not source.** Design and evidence:
  `specifications/ownership-identity.md` § Proposed: shape is not source. Do not restate it
  here.

  **Timing is the open decision.** Node and repo matching consume the resolution chain this
  changes, so doing it after them means revisiting them. Doing it now adds a chunk to the
  MVP before matching starts. Row volume will never be smaller than it is today.

  The symptom that raised it — one address belonging to two owners under two types, the
  second unreachable — is a consequence of the model, not a separate bug, and is fixed by
  keying uniqueness on the value.

Dependency on stable username: the whole graph assumes `username` stays stable
(driven by `username_attr` → a stable claim). If `username_attr` is unset,
username falls back to the (possibly transient) NameID and both login anchoring
and ownership matching break. Surface a config warning if `username_attr` is
empty for a SAML provider.
