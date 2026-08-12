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

**The customer's database import is blocked on network access, not on this code.**
Their ownership database sits behind one name resolving to several addresses across
subnets — an availability group, which is why their connection carries
`MultiSubnetFailover=True`. From the host running this tool, one address refuses and
another times out, so nothing there is reachable. Firewall rules are slow to arrange,
so the first import will be a CSV extract instead, which is the proven path anyway.

What was settled on the way, so it need not be worked out again:

- **The connection string itself now works.** It was refused as "invalid URL format"
  because the credential contained a character no URL can carry. That is repaired on
  the way to the driver, and the failure has moved to the network layer.
- **Their credential very likely lacks its domain prefix.** Their own script connects
  as `DOMAIN\` + username; the connection stored here reported no backslash in it. That
  will surface as an authentication failure once the network path opens, and it will
  look nothing like the errors seen so far.
- **Nothing has watched a commit from a database source.** So even with the firewall
  open there are two unknowns, not one.

The discovery-driven CSV intake is in (`journeys/ownership-intake.md`).
Remaining:

- [ ] **SQL source — the reading half is built** (`internal/ownershipsql`), for SQL Server and
  PostgreSQL equally. What remains is credentials in the encrypted config store, the
  profile/preview/run endpoints taking a database as an alternative source, and the UI. See
  `plans/active.md` for the ordered list.

- [ ] **The connection is visible; only the password is secret.** Requirement and reasoning:
  `journeys/ownership-connection.md`, suite `internal/webapi/ownership_connection_journey_test.go`
  (all skips — nothing is built). Settled with the product owner 2026-08-12: one connection
  string the administrator can read and edit, with **only the password substituted into it**,
  because the password is the only value they can never see and therefore never check. Nothing
  visible is rewritten behind them. A starting example is proposed per kind of database and is
  freely overridable. The connection can be tested before browsing tables, and both the string
  reader and the server report in their own words.

  **Measured 2026-08-12, and it contradicts the obvious assumption.** `sql.Open` was probed
  directly for both drivers. SQL Server given a URL-form DSN parses eagerly and returns
  `unable to parse connection string: invalid URL format` — the exact error the customer hit,
  and it is the driver's, not ours. But the same driver accepts literal nonsense when the string
  is not URL-shaped, because it falls back to keyword parsing; and lib/pq validates **nothing**
  at Open, accepting `"not a url at all"` without complaint. So: pass driver errors through, but
  they cannot be the validation, and the "must name a database" rule stays ours.

  The escaping rule differs by form — percent-encoding for URL-shaped, brace-quoting for
  keyword-shaped — so applying one to the other produces a string that reads correctly and is
  refused. **Measure both against the SQL Server container** (`make mssql-up`, `seed-mssql`,
  `test-mssql`); do not settle it from documentation, which is how it was got wrong before.

  **Errors must have the password taken out of them.** Both the string reader and the server
  routinely quote what they were handed, and neither knows which part was secret; this estate
  ships its logs to a Splunk many people can read, and screenshots and support bundles carry the
  same text. Redact wherever and however it appears, **including escaped** — the escaped form is
  the case that will be missed, because the password in the message no longer looks like the
  password that was stored.

  Highest value first: **showing the composed string with the password masked**. It answers in
  one glance the question that has cost days, and is cheap next to the rest.

- [ ] **SUPERSEDED — hold a database connection as its parts rather than as a string to be parsed** —
  replaced by the item above, which keeps one visible string and injects only the password.
  `plans/database-connection-as-parts.md` is kept for its reasoning, not as work to do. Original:
  host, database, user, password and a list of vendor options, with the connection string
  constructed when connecting. See `plans/database-connection-as-parts.md`. This absorbs the
  question of deriving the driver from the connection string: it stops being a question.
  The load-bearing consequence is that only the password is a secret, which removes the
  redactor and the shape describer rather than improving them.

- [ ] **A saved import does not carry the TLS override.** The interactive import takes one
  per run, but `ImportMapping` has no column for it, so a schedule built from a connection
  needing an override connects without one and fails. Not silent — the failure now reports
  that no `sslmode` was set — but wrong. Needs a column and a migration.

- [ ] **`strict` TLS for SQL Server is offered but unproven against a server that supports
  it.** The local container cannot do TDS 8.0, so the only thing measured is that it
  correctly refuses — which is what the functional test asserts. Whether it connects to a
  server that does support it has not been seen.

  **Testing it needs no customer database.** `make mssql-up`, `make seed-mssql`,
  `make test-mssql` stand up SQL Server 2022 in a container, seed a sample system of record
  and run the functional tests against it. The seed is deliberately awkward — a join across
  two tables, a person who has left, an asset with no owner, an owner with no email, and date,
  NVARCHAR and BIT columns — because those are what a PostgreSQL test cannot tell you about.

  **MVP2: a permanent Linux VM running SQL Server in the Proxmox lab.** No arm64 image exists,
  so the container runs under emulation on Apple Silicon; it works and is fine for
  development, but a real VM is the better home for demos and anything long-lived.

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

  **A commit is one synchronous request, and that is the scale ceiling.** Measured
  2026-08-02 on real data: 18,821 assignments plus 1,862 new owners took 37 seconds,
  about 500 rows a second on a per-row insert path. The same rate puts a 267,000-row
  import near eight minutes of a single held-open request — past most default proxy
  timeouts. Nothing is transactional, so a timeout mid-commit leaves a partial import
  **and no report**, which is worse than a refusal: there is no record of where it
  stopped. Batching the inserts would buy most of it back; making the commit
  resumable or asynchronous is the real fix, and neither is needed while imports are
  filtered to the tens of thousands.

  Shape to aim for: **profile a bounded sample** (`LIMIT`, or a cursor stopped after N)
  and report it as "profiled the first N rows" rather than as a row count, plus a
  statement timeout on the connection. A true count, if wanted, is a separate cheap
  `COUNT(*)` rather than a side effect of reading everything.
- [ ] **Raise the import cap to 250,000 rows.** Requirement: `journeys/ownership-intake.md`
  ("To load the whole thing in one go"), held red by
  `TestJourney_TheWholeSourceLoadsInOneGo`. The cap is 100,000 today and the customer's source
  holds about 130,000 in one list; splitting it needs filters that between them must cover
  everything exactly once, with no way to check they did. **The cap itself is one constant. The
  problem is the commit path** — see the synchronous-request note below: ~500 rows/second means
  130k is around four minutes of a held-open request and 250k around eight, past most default
  proxy timeouts, and a timeout mid-commit leaves a partial import and no report. Raising the
  number without batching or a resumable commit ships a cap that fails later instead of sooner.

- [ ] **Reuse a saved mapping from the UI.** The mapping CRUD endpoints ship and the
  UI can save one, but cannot yet load one back — a repeat import still re-maps by hand.
- [ ] **Download the match report as CSV.** The report is on screen only.
- [ ] **`policy` entity existence is never confirmed.** CMM collects no policy objects,
  so a policy key always reports as not collected. Harmless — an uncollected entity
  never rejects a row — but the UI should say why rather than implying a miss.
- [ ] **Scheduled imports, run one at a time. MVP2.** Raised by the product owner 2026-08-03:
  several imports on a schedule, executed **serially** so a large source cannot be read
  alongside another and blow the memory budget.

  **Most of the parts already exist.** A saved mapping says how to read a source; a stored
  credential holds the connection; the database source is a streaming cursor that reads a
  query. A scheduled import is those three plus a trigger. `kitchenqueue.Manager` is the
  precedent for serial execution with a DB-backed queue that survives a restart, rather than
  goroutines and a timer.

  **What has to be decided, not assumed:** what happens when a run is still going and the next
  is due (skip, queue, or overlap — skip is the memory-safe answer); whether a failed run
  retries or waits for the next slot; and how a run reports what it did, given a person is not
  watching it. A scheduled import that quietly imports nothing is worse than one that fails.

  **It makes the item below load-bearing rather than theoretical.** A nightly import against a
  source that has changed will accrete ownership unless reconciliation is settled first: do
  not schedule anything until it is.

- [ ] **Ingest is additive — reconciling a refreshed source. MVP2.** A row dropped from
  the source never removes the assignment it created, so revoked ownership persists. With
  a refresh job on the source database this stops being theoretical: repeated imports make
  ownership accrete, and a person who handed something over years ago stays attached to it.

  **The operating model, agreed with the product owner 2026-08-02:** treat the source
  database as the truth for what it covers, add anything missing by hand, and remove dead
  rows when a fresh import arrives. `ownership_assignments.assignment_source` already
  scopes this exactly — only `import` rows are ever in scope for removal, so a `manual`
  addition cannot vanish underneath somebody.

  **The one missing piece** is that a repeat import leaves no footprint: `insertAssignment`
  is a plain INSERT, the repeat is caught on the unique index, and `updated_at` is never
  touched — so a reaffirmed row is indistinguishable from an abandoned one. An upsert that
  touches `updated_at`, or an import-run stamp, makes "import-sourced and not reaffirmed
  by the latest run" a query.

  **Keep it a report with a delete beside each row, never an automatic purge.** A truncated
  or half-failed export would otherwise wipe live ownership, and it would do it silently.
  The delete already exists on the owner's page (`OwnerDetailPage.tsx`), so the missing
  half is the list, not the action.

## Failure register — what is left

Behaviour is `journeys/failure-register.md`. Manual entry, the standup view and the
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

## Ownership filtering in the list views

Journey 1 ("what's mine") and the unowned question both need the list views to filter on
ownership. The git repo and cookbook lists now carry the control; three decisions bind
anything that extends it:

- **Both questions live in one control** (`OwnerFilter`). The API rejects `owner` and
  `unowned` together with a 400, so ticking either withdraws the other and the pair cannot
  leave the page. Split across two controls that rule could only be enforced by catching the
  error after somebody had already asked for it.
- **Owners are searched on the server**, not filtered in the browser. The estate carries
  thousands, so a page of them held locally would answer "no such person" for anybody who did
  not make the first page. The control calls `GET /owners?search=`, which runs the owners
  list's readiness enrichment on every search — measure it against the real estate before
  assuming it is cheap enough.
- **The export carries the ownership filter too.** Adding the control made an export taken
  from a filtered list reachable, and the git-repo export was returning the whole estate with
  nothing in the file to say so.

- **The chips sit in a row of their own, not in the bar** (`OwnerFilterChips`). The control is
  a fixed-width button carrying the count; the selection wraps full-width beneath the bar, so
  no number of owners can displace the other filters. The count alone was rejected: it cannot
  say who is selected or take one person off.
- **Node list carries the ownership control**, the same as git repos and cookbooks: both
  questions, chips in their own row, savable as a named cohort. The deferral was wrong — it
  said there was no node ownership dataset, but the import has always accepted `node` as an
  entity type and verifies node keys against `node_snapshots`. Nothing was missing but data,
  and data can be made locally.
- **Ownership and the team verdict are part of a saved filter.** A saved owner selection
  names people, so it is the fixed cohort "alice.brown's repos" — which is what a *shared*
  one means to everyone who opens it. "What's mine" is that cohort saved unshared; there is
  deliberately no marker that resolves against whoever is looking. `owner` and `unowned` ask
  opposite questions, so the pair is rejected at save time as well as on the list request —
  otherwise a cohort could hold a contradiction that only failed when somebody opened it.
  `human_verdict` is savable on git repos, and not on cookbooks, whose parser ignores it.
- **The export path enforces the same ownership rule as the list views.** Checked once in
  `handleExports`, before any source runs, rather than in each export source: a source can
  only fail as a 500, so the caller would have been told the export broke rather than that
  they asked two contradictory questions. It therefore covers every export type, including
  roles, which ignores ownership entirely — a contradictory request is nonsense whether or
  not the endpoint would have acted on it.

## "My stuff" — the logged-in person's own estate

Journey 1's actual question. **Not built.** What exists is "find yourself in the owner list and
tick your own name", and save that as a private cohort. There is no control that says "mine",
because nothing connects a session to an owner record.

**How they link, confirmed by the product owner 2026-08-03:** the SAML email, or the user id,
or the username / display name. All are already on a session — `/api/v1/auth/me` returns
username, display name, email and provider — and `owner_aliases` already stores `email` and
`custom` aliases.

**This is the point of § Matching app users to owners at the top of this file** — the alias
auto-creation on SAML login is what makes the resolution below possible, and neither is
retired by the 92%-owned measurement, which was about entity matching. Build that section
first; this one is its user-visible half.

- [ ] **Resolve a session to an owner**, trying the identifiers above against `owner_aliases`
  and the owner name. **The dangerous case is not "no match", it is "two matches":**
  `alias_type` conflates the shape of an identifier with where it came from, and uniqueness
  includes the provenance, so one address can legitimately belong to two owners (recorded in
  `journeys/ownership-identity.md` § Proposed). Showing somebody else's estate under the
  heading "mine" is worse than showing nothing, so an ambiguous match must refuse and say why.
- [ ] **A "My stuff" control** on the node, git repo and cookbook lists, applying that owner.
- [ ] **An honest answer for the majority who own nothing.** Most users will not resolve to an
  owner. It must say so, not quietly show the whole estate — this branch has produced that
  failure three times already (an owner catalogue that failed to load, a search parameter the
  server ignored, a list silently cut at fifty).
- [ ] **Display-name matching is the one to be careful with.** Two people share a name far more
  often than they share an email. Prefer email, then username; treat a display-name match as a
  suggestion rather than an answer.

## Identity and alias management — what is left

- [ ] **Surface it at the point of use**, per the journey-1 refinement in
  `plans/ownership-work-attribution.md`: wherever an owner is read, show the raw string
  and any candidate owners, one action from being merged.
## Where owners come from — generalise the committer flow

**Read the scoping decision first** — `plans/ownership-work-attribution.md` § Ownership only has
to be right where somebody has to act. Everything below is an optimisation on a list that may be
small enough to work through by hand. **Measure that list before building any of it.**

**Blocking and unowned — MEASURED 2026-08-02 against the real estate. This answers the
question the rest of this section was waiting on.**

  A real ownership export was imported: 18,821 assignments, 1,862 owners created, 37 seconds.

  | | |
  |---|---|
  | repos CMM knows | 2,126 |
  | repos with an owner | 1,963 (**92%**) |
  | blocking **and** unowned | **126** |

  **2026-08-03: THIS NUMBER IS INFLATED AND THE CONCLUSION BELOW NO LONGER STANDS.** The
  product owner reports that **roughly half of the repos are assigned to one person in the
  Chef team, because the real owner is unknown**. That is a placeholder, not an owner. So
  "92% carry an owner" is nearer 45% genuinely owned, and the 126 blocking-and-unowned is a
  serious undercount — a repo assigned to the stand-in *has* an owner, so the unowned filter
  cannot see it, and neither could the measurement.

  **What follows:**

  - Entity matching was retired on the strength of this number. It should be reopened, or at
    least not treated as settled, until the real figure is known.
  - "My stuff" for that one person would return around a thousand repos, which is useless to
    them and is itself the tell.
  - **The primitive already exists and has never been used.**
    `ownership_assignments.confidence` takes `definitive` or `inferred`, and every write path
    in the codebase hardcodes `definitive` — four call sites, all literals. Nothing sets it,
    filters on it or displays it. A placeholder assignment is exactly "not definitive".
  - Marking the *owner* as a stand-in is probably simpler than marking every assignment:
    `owners.owner_type` already has a `custom` value, and one record is doing this job.
    Whichever it is, **the unowned question has to be able to mean "nobody real"**, or it goes
    on undercounting.

  **The decision rule, and it has not changed:** ownership only has to be right where somebody
  has to act. Twenty or thirty repos needing an owner is an afternoon of asking around; a
  thousand is a project. So the number to get is not "how many repos are unowned" but **"how
  many *blocking* repos have no owner once the stand-in is discounted"** — and two corrections
  pull it in opposite directions. Placeholder ownership pushes it up; CookStyle and Test
  Kitchen over-blocking push it down. Neither has been applied, which is why 126 means little.

  ```sql
  WITH standin AS (SELECT 'THE-STANDIN-OWNER-NAME'::text AS name),
  real_owner AS (
      SELECT DISTINCT oa.entity_key
      FROM ownership_assignments oa, standin s
      WHERE oa.entity_type = 'git_repo' AND oa.owner_name <> s.name
  )
  SELECT gr.cookstyle_status, gr.tk_status,
         CASE WHEN ro.entity_key IS NULL THEN 'needs an owner' ELSE 'genuinely owned' END,
         count(*)
  FROM git_repos gr
  LEFT JOIN real_owner ro ON ro.entity_key = gr.name
  WHERE gr.cookstyle_status = 'blocked'
  GROUP BY 1, 2, 3 ORDER BY 4 DESC;
  ```

  Joins verified against the dev database. Drop the `WHERE` to sanity-check it returns rows
  before trusting a zero — a query that finds nothing and a query that is wrong look identical.

  **The other measurement, and it is one query:** assignments grouped by owner, descending.
  The stand-in will be at the top by a wide margin, and the size of that first row is how
  wrong the 92% is.

  **The original conclusion, now in doubt:** Both chunks were
  scoped on the assumption that ownership was largely absent. It is 92% present, and 126 repos is
  a list somebody works through by hand in a couple of days — cheaper than an engine that would
  still need a human to confirm every uncertain match. Re-scope or drop them; do not start either
  without revisiting this number.

  **The 126 are mostly CookStyle-blocked with Test Kitchen untested** — the least trustworthy
  state the product has, and the one the failure register exists to correct. So they are not 126
  repos needing owners; they are **126 unverified claims**. Verify the top few by affected nodes
  on a real converge and record the verdicts, before anybody spends a day hunting owners for
  cookbooks that may run fine. That also seeds the register with real entries chosen by impact.

  The impact-weighted list (the `ORDER BY affected nodes` half) was not run. It is a
  `jsonb_array_elements` over `node_readiness.blocking_cookbooks` joined to the unowned set, and
  it is what turns 126 into the twenty that matter.

  **Note:** `2,126`, not the `2,210` quoted elsewhere in these plans. Correct the older figure
  where it is load-bearing.

Raised by the product owner 2026-08-02. Assigning ownership from git committers is one
*provider* of candidates, wired directly to assignment. The general shape is: for an
entity, every provider offers candidates with their evidence and how far that source is
trusted; a human picks. Findings and evidence:
`journeys/ownership-identity.md` § Where ownership candidates come from.

Providers, most authoritative first:

- [ ] **The source repository platform's own ownership data — Stash today, GitLab soon.**
  Raised by the product owner 2026-08-02: a system account may be granted to read repo
  ownership directly from the Stash and/or GitLab APIs. The consolidated database the
  spreadsheets came from is believed — not confirmed — to be fed from Stash, so this would be
  the same data at its source rather than an export of unknown age.

  **What it fixes:** staleness. It removes the whole reconciliation problem below — no
  generations, no age-out, no wondering when a file was pulled.

  **What it does not fix:** coverage, which is already 92%. That weakens the case for building
  a collector, credential handling and rate limiting, so decide on freshness value alone.

  **The caution that matters:** repository *permissions* are not accountability. "Owner" in
  Stash or GitLab usually means whoever administers the repo, and on a large estate that is
  often a platform admin holding it across hundreds. **Check the cardinality before trusting
  it** — one name against four hundred repos is an access-control artefact, not an owner.

  Worth requesting the system account now regardless: approval is slow and costs nothing to
  start while the decision is open.

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
  `journeys/diagnostic-bundle.md` is already the right vehicle: read-only, produces a
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
  Findings and measurements: `journeys/ownership-identity.md` § Git identities are
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
  `journeys/ownership-identity.md` § The label is not an identity.
- [ ] **Re-model aliases: shape is not source.** Design and evidence:
  `journeys/ownership-identity.md` § Proposed: shape is not source. Do not restate it
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

## Decisions that bind, moved out of the active plan

Kept because they are the reasoning, not the status: the work they describe is in `main` and
its done-ness is git history and passing tests. Do not re-litigate these by finding them
inconvenient.

- **Ingest creates unresolved people rather than rejecting the row**, and a fuzzy candidate does
  not reject it either. Correction is deferred to the point of use, by design.
- **A human verdict in the failure register outranks CookStyle and Test Kitchen**, and joins the
  existing per-source verdicts rather than sitting beside them.
- **Import and duplicate handling are admin-only**, reversing two earlier decisions on the
  owner's instruction. A preview shows the contents of a system of record, and writing nothing
  is not the same as showing nothing.
- **A schedule belongs to a saved import, not to a global setting.** A saved mapping held only
  the field map, so an unattended run had no connection to run against; widening it into a saved
  import is what makes scheduling possible, and lets each system of record carry its own cadence.
- **A scheduled run commits.** Staging for review was not built: a schedule that needs somebody
  to approve it is a reminder, not a schedule.
- **No global on/off switch for the scheduler.** It polls and does nothing when nothing is
  scheduled. A schedule the screen shows and a flag silently suppresses is the failure the
  feature exists to avoid.
- **Entity matching is retired; identity matching is not.** What the measurement retired is
  guessing which repo or node belongs to whom when nobody recorded it. Resolving the several
  identifiers one person has onto one owner record is what aliasing exists for, it is what "my
  stuff" needs, and it is open above. Do not let the 92% be read as retiring it.

**The audit log's detail keys are inconsistent between writers, and the corrections export
depends on them.** A merge records `into_owner`, a reassignment `to_owner`, a dismissal
`owner_a`/`owner_b`, and a deleted assignment names the owner on the entry itself. The first
export assumed `to_owner` throughout and produced an empty "should be" column on every real
merge — a report telling somebody to fix their data with the fix missing. A test now asserts the
keys against the writer's own type rather than a literal.

**Known gap:** rejected import rows are not in the corrections export, because the per-run report
is built and discarded. They are the most direct statement of source data quality there is.

**Verification owed:** no scheduled import has been watched firing against a real database. Every
layer is covered by tests and the app runs with the scheduler started, but nobody has seen one
fire.
