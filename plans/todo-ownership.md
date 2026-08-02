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

  **The file-import protections carry over free**, because they all sit above the
  source abstraction: the row cap and the value filter in `handleIntakeRun`, the
  distinct-value cap in `Profile(src RowSource)`, and the report truncation. A SQL
  source inherits every one without change.

  **One does not, and it is the dangerous one.** `Profile` reads to the end of the
  source. For a file that is bounded by the file, which somebody already has. For a
  query it is bounded by what the query returns — so a mistyped `WHERE` makes CMM scan
  the whole table before anyone sees a preview. Raised by the product owner 2026-08-02:
  a DB source *can* pre-filter on the identity-type column, but a filter that can be
  written can be written wrongly, so the bound must not depend on it being right.

  Shape to aim for: **profile a bounded sample** (`LIMIT`, or a cursor stopped after N)
  and report it as "profiled the first N rows" rather than as a row count, plus a
  statement timeout on the connection. A true count, if wanted, is a separate cheap
  `COUNT(*)` rather than a side effect of reading everything.
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

## Failure register — what is left

Behaviour is `specifications/failure-register.md`. Manual entry, the standup view and the
human verdict inside the readiness rollup are built.

- [ ] **A register entry only reaches readiness when its subject name matches a cookbook name.**
  The evaluator resolves a standing verdict by name. A repo whose name differs from the cookbook
  it holds is recorded and visible in the register but changes nobody's readiness — silently.
  Narrowed since the subject may now be the cookbook itself, which removes the common case; what
  is left is repos named differently from their cookbook. **Measure how often that happens before
  building matching for it.**
- [ ] **Versions are not modelled, deliberately.** A break may be in one version, several, or all,
  and today that nuance lives in the reason and evidence text. Keying entries on a version would
  fragment one problem into several and would need the evaluator to match versions too. Revisit
  only if the seeded entries show the free text is not enough.
- [ ] **Group the register by assignee, for taking standup in turns.** Raised by the product
  owner 2026-08-02, and agreed as MVP 2 rather than MVP. Each person reads their own items,
  so the list clusters by who is on it rather than by when it was raised.

  **The wrinkle to settle first: a holder is not necessarily a person.** It may be an owner,
  a user, or a ticket reference, and the record form defaults a bare reference to a ticket —
  so on today's data most holders would be ticket numbers, which cannot take a turn. Grouping
  is only as good as the proportion of holders that resolve to people, which makes this
  dependent on owner resolution rather than on grouping being hard to build.

  Cheapest useful form, if the dependency proves slow: sort or group on the holder as stored,
  and let a ticket-held item fall into an "elsewhere" group. Saved filters already exist for
  the git repo list (`SavedFilterBar`), so a saved per-person filter needs no new concept —
  and needs no team entity either, which is what journey 1 already concluded about
  multi-select over people.

- [ ] **Free-text references to things that should resolve may exist elsewhere.** Raised by the
  product owner 2026-08-02 after the register's owner reference turned out to be free text.
  The pattern to look for is any field naming an entity CMM holds — an owner, a repo, a
  cookbook, a node — that is typed rather than picked, because nothing reconciles it
  afterwards and the reference reads as valid while pointing at nothing. Deliberately not
  swept yet: **inventory it before fixing any of it**, since ingest paths take data as they
  find it on purpose and must not be "corrected" into rejecting rows.

- [ ] **The accuracy report is not defensible over time.** Every `broken` entry is counted as a
  failure the tools missed and every `not_broken` as a verdict they got wrong, but what the
  scans actually said at the moment the entry was raised is not snapshotted — so the numbers
  drift as scans are re-run. One read of the repo's materialised status at record time would
  fix it. Not built: it needs deciding what to record when a repo has never been scanned.
- [ ] **Repeat occurrences are not recognised as the same failure.** The spec's fingerprint
  design (hash a bounded, canonicalised projection) is deliberately unbuilt — it only matters
  once entries come from telemetry rather than by hand, which is out of scope.
- [ ] **Nothing surfaces the register on the repo or node it affects.** A node blocked by a
  human verdict carries the reason in `blocking_cookbooks`, and no view reads it yet.

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
- [ ] **Characterise the estate before writing any rules — extend the diagnostic bundle.**
  `specifications/diagnostic-bundle.md` is already the right vehicle: read-only, produces a
  file that can be carried back, and built around aggregate counts with identifiers as an
  explicit opt-in. **No new deployable, and nothing from an unmerged branch** — it can answer
  at scale on the version the customer already runs.

  Two measurements, both read-only, both needed on the same trip:

  **Identity landscape.** 2210 repos across several hosting platforms and CI systems will
  carry several patterns, not one. Per repo: how often author and committer differ, the
  distinct identities on each side, and — the service-account detector — which addresses
  appear across many repos, since a person contributes to a handful and a pipeline account to
  hundreds.

  **Blocking-set composition.** Not the size of the blocking set, its provenance: how much is
  Test Kitchen alone with CookStyle disagreeing, how much is CookStyle alone, how much is
  corroborated by both. Only the corroborated part is worth dispatching work from today.
  Within the Test Kitchen share, how much is environmental — `timed_out` is already counted
  as a cookbook failure, which gives a free lower bound. *(This half is about compatibility
  signals rather than ownership; it is here because it rides the same trip and the same
  bundle, and because it decides whether the ownership question is worth asking yet.)*

- [ ] **Rules for platform identities, bots and maintainers — write them against real data.**
  Findings and measurements: `specifications/ownership-identity.md` § Git identities are
  rewritten by the hosting platform. Deliberately deferred, and safe to defer because git
  history is re-derivable from the clones on disk.

  Cheaper and already collected: `commit_count`, `first_commit_at` and `last_commit_at` are
  on every committer row and nothing in ownership reads them. Journey 1 asks for how much
  and how recently somebody contributed; recency and concentration need no collector change
  at all.
- [ ] **Owner names are derived from an identifier and cannot be changed.** The committer path
  names an owner after an email localpart, so two addresses for one person make two owners, and
  `owners.name` is immutable — the referencing foreign keys have no `ON UPDATE CASCADE`, so
  merging is the only repair for what is a naming accident. Design:
  `specifications/ownership-identity.md` § The label is not an identity.
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
