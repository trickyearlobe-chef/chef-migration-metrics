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
## Where owners come from — generalise the committer flow

**Read the scoping decision first** — `plans/ownership-work-attribution.md` § Ownership only has
to be right where somebody has to act. Everything below is an optimisation on a list that may be
small enough to work through by hand. **Measure that list before building any of it.**

- [ ] **Blocking and unowned, ordered by affected nodes.** Needs no matching, no rules and no new
  collection — a join over cookstyle results, complexity and assignments. It is both the answer to
  journey 3 and the number that decides whether anything else here is worth building.

Raised by the product owner 2026-08-02. Assigning ownership from git committers is one
*provider* of candidates, wired directly to assignment. The general shape is: for an
entity, every provider offers candidates with their evidence and how far that source is
trusted; a human picks. Findings and evidence:
`specifications/ownership-identity.md` § Where ownership candidates come from.

Providers, most authoritative first:

- [ ] **CODEOWNERS** — a deliberate, machine-readable declaration, honoured by both GitHub
  and GitLab. Not collected. **Ask whether the customer uses it** before building for it.
- [ ] **`metadata.rb` maintainer — already collected**, and enough on its own to separate
  vendored from internal. Nothing in ownership reads it. `maintainer_email` is in the
  metadata and returned by the Chef API but is parsed and stored nowhere — a name matches
  by similarity, an address joins to an identity.
- [ ] **The ownership import** — deliberate, built.
- [ ] **git authors and committers** — inferred, noisy, rewritten by the hosting platform.
  The only one wired to assignment today.

- [ ] **Gate the committer flow on internal identities.** On a vendored cookbook it invents
  owners from external contributors, and since 2026-08-02 seeds their commit addresses as
  aliases too, where they can go on to match import rows.
- [ ] **"Vendored and unowned" is not a special case** — it is every provider returning no
  internal candidate. Report it rather than letting the row quietly fail to match: nobody
  has been made responsible, which is a finding a migration programme needs, not a matching
  failure to fix.
- [ ] **Internal email domains are the enabling primitive**, and are configured nowhere.
  One value powers vendored detection, service-account detection (internal domain, spread
  across hundreds of repos) and keeping external contributors out of the catalogue.
- [ ] **Finding the unowned is the gap, not assigning them.** Assignment without ingest
  already works — owner detail takes an array, so bulk is a UI question, not an API one.
  But `parseOwnerFilter` is wired into the platform dashboard and remediation only: not the
  git repo list, not the cookbook list, and the frontend never sends `unowned` anywhere. No
  screen answers "show me every repo with no owner".
- [ ] **Characterise the estate before writing any rules.** 2210 repos across Stash, GitLab
  and Jenkins will carry several patterns, not one. Per repo: how often author and committer
  differ, the distinct identities on each side, and which addresses appear across many repos
  — that last one is the service-account detector, since a person contributes to a handful
  and a pipeline account to hundreds. Customer access is VDI or file transfer, so it has to
  produce a small aggregated artefact that can be carried back, not a console dump.

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
