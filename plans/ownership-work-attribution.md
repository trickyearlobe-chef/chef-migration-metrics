# Ownership, Work Attribution, and the Failure Register

## What this is for

CMM can say what blocks a migration. It cannot say whose job it is to fix, and it has nowhere to
record a failure that testing did not predict. Ownership exists today as an admin console — owners,
assignments, aliases, audit — that no list view, filter or export reads.

This plan carries **journeys, gotchas and features**. It deliberately does **not** carry call-site
inventories, line numbers or per-function failure modes: that detail is rediscoverable in minutes
with grep, LSP and the refactor MCP, it goes stale within days, and every defect found while
reviewing earlier drafts of this plan was in that layer. Derive it; do not inherit it.

---

## User journeys

Stated by the product owner, 2026-08-02. These are the requirement. Anything below that
does not serve one of these is scaffolding and can go.

**In scope for Monday: 1, 2, 4 and 5.** Complexity is not a reason to drop one.

**1. "What's mine."** A team lead sees only their own estate — the nodes and git repos they
own — and works from that list. Several people can be selected at once: working groups are
often a handful of individuals rather than a named team.

For this to answer anything, a signed-in person has to resolve to an owner. Known sources,
still being refined with the product owner:

- **SSO**, which brings email, name and user id.
- **Git contribution history** — who has contributed code, how much, and how recently.
- **Added by hand.** Not only by administrators: end users should be able to contribute
  their own aliases alongside the sources we derive.

Where a match is uncertain it is **suggested for a person to confirm, never applied
silently**. Fuzzy matching already exists in CMM and may serve this.

**Refined by the product owner, 2026-08-02 — this supersedes "a wrong owner is worse than no
owner".** An imperfect owner is acceptable *provided the clue survives*, because people are good at
getting to the right person from a recognisable one: they reach "Thomas Smith" from "Tommy Smith",
"Smithy" or "Fat Tommy". That makes it correctable — as an alias, or at source. What is not
acceptable is a silent guess or a discarded string, because both destroy the clue.

**Correction is deferred to the point of use, not done at ingest.** *"Fixing a 20,000 user import at
ingest is time consuming. Correction can be deferred and fixed later when we are looking at who
needs to fix something — who owns this repo… Fat Tommy… I wonder if that's Thomas Smith."* So ingest
takes the data as it finds it, and the correction moment belongs to journeys 2 and 6, where somebody
has the context. Consequence for those journeys: wherever an owner is *read*, the raw string and any
candidate owners must be visible and one action away from being corrected.

**Teams do not exist as data, and owners can be several people.** There is no source for who
is in a team, so teams would have to be constructed by hand. **For MVP that is avoided:**
filtering on a multi-select of users covers the working-group case without a team entity —
which is why journey 1 says people rather than teams.

The source data already carries **multiple owners for a single cookbook or node**, quite
possibly the customer's own workaround for the missing team data. That leaves an **open
question: when several people own something, who is specifically on the hook to fix it?**
Unresolved, and it lands on journey 2, which promises "the person accountable".

**Deferred idea — infer teams from co-ownership.** Where the same people repeatedly own the
same things together, that grouping is probably a team, and could be proposed at a stated
level of confidence rather than asserted. **Not in scope for Monday** unless it turns out to
be the cheapest way to hit it.

**2. "Who can unblock this?"** A node is blocked. Its owner carries the outcome but usually
cannot do the work, because the fix lives in a git repo someone else owns. From the blocked
node, reach the repo holding the fix and the person accountable for it.

**3. "What is blocking migration, and where is the work tracked?"** A programme manager needs
to see what is blocking migration, what work has an owner, what is progressing, and **where to
find the actual work tracking** — which may be comments in CMM, or may be tracked elsewhere.
CMM points at externally-tracked work by **URL or ticket number**, which may address a Jira
issue, a ServiceNow task, or anything else. CMM does not integrate with those systems; it
holds the reference.

**4. "Something broke that testing didn't catch."** An engineer records a failure nobody
predicted: what broke, the diagnosis, what they plan to do about it, and a target date if one
has been given. Recorded by hand, with no telemetry configured. Read back daily — see journey 6.

**6. "What's broken right now?" — the standup view.** Every morning the team reads the
register together and needs, in one place: **which cookbooks are currently broken** — which
ultimately map to git repos — **why** each one is broken, **what is being done about it**, and
**whether the list is getting too large**. The size and direction of the list matter as much
as its contents: a register that is growing is a different message from one that is shrinking.

This is the consumer for journey 4. Without it, recording a failure is data entry nobody
reads, and people stop doing it.

**5. "Get our ownership data in."** An admin brings existing ownership data in from a CSV
**and/or a database** — probably MSSQL, possibly PostgreSQL. **We will not have the schema, so
discovery happens in the UI:** choose a table and pick fields from it, **or** supply a SQL query
and pick fields from what it returns, with a **data preview** to make that choice possible. It
must succeed while the source data is still inconsistent, and report what matched, what did not,
and why.

---

## Vocabulary — cookbook and git repo are not interchangeable

Affects journeys 2, 4 and 6. Getting this wrong builds the right feature against the wrong
thing.

- **The unit of work is the git repo.** That is where a fix is made and re-released, so the
  diagnosis, the plan, the target date and the ticket reference all hang off the repo.
- **It is called a cookbook.** To a human the cookbook *lives* in the repo and also *gets
  deployed*, via the Chef Server, onto nodes — one thing seen in two places. Standup says
  "cookbook" while looking at repo-level work.
- **Never a specific version.** Several versions are usually in use at once, so failures are
  discussed version-agnostically.

So the work is **keyed on the repo** and **labelled with the cookbook**. This is one list,
not two.

**This customer has exactly one distinct cookbook per repo**, holding multiple versions in
the normal manner — so the label is unambiguous. Repos containing several distinct cookbooks
are **mono-repos**; some customers use them, this one does not. **Deliberately not considered
now** — assume one cookbook per repo, and treat mono-repo support as a later question rather
than designing around it.

## Gotchas

Hard-won and expensive to rediscover. Everything else in this plan is cheaper to derive than to read.

**Git repo URLs are discovered and volatile; repo names are not.** `git_base_urls` is an ordered
preference list and the collector re-points a clone at a higher-preference base as soon as the repo
appears there — built for incremental re-hosting across several bases, with no cutover. Re-hosting
writes a new row and deletes the old one. Ownership assignments have no foreign key and nothing
cleans them, so a re-hosted repo's owner silently stops resolving. **Key ownership on the repo
name.** This is the single most valuable thing on this page.

**Nothing populates owner aliases automatically.** The committer-assign path — the main way owners
get created — sets a contact email but writes no alias row. So any "who am I?" lookup that goes only
through aliases returns nothing on a real deployment, and *Mine* is dead on arrival. Seeding is part
of the work, not an optimisation.

**A commit address is not a corporate address.** Noreply addresses, personal SCM accounts,
contractors. Treating them as the same thing silently attaches work to the wrong person. Where they
genuinely differ, resolution must **not** quietly bridge them: the candidate is surfaced for a human
to confirm. It does not follow that the row should be dropped — see the refinement under journey 1.
Note the localpart signal: the same username under several domains is a strong *lead* and a bad
*match*, because the committer-assign path already collapses owner names to the localpart.

**The SAML subject is a login token, not an identity.** It is refreshed every login, drifts with a
transient NameID, and appears in no other system, so it can never join to anything. Identity anchors
on username and email. A spec still claims otherwise; it is wrong.

**This corner of the codebase has never been exercised against a real database.** The ownership
datastore SQL has no functional test coverage at all, and *every* query examined there was broken —
silent row caps, joins on dropped columns, queries against tables that have never existed. Errors
are swallowed, so all of it looks like an empty or unremarkable result rather than a failure.
**Assume anything here is broken until a test proves otherwise, and verify each function's actual
failure mode before writing its red-first test** — some error, some return confidently wrong numbers,
and a test asserting an error will never go red for the latter.

**A blind backfill will stop the service from starting.** A unique index over ownership assignments
turns a colliding rewrite into a duplicate-key error, which rolls back the migration, which exits
the process at boot. Rewrite only non-conflicting rows; leave the rest as they are and log them.
Never delete an ownership row.

**Converge run data is ephemeral.** Range-partitioned with a short default retention, no foreign
keys, and switched off by default. Snapshot the evidence you need at capture time; never reference it.

**Comparing every owner with every other owner does not finish.** Owner names cluster hard —
people share surnames, and committer-derived names are email localparts — so a trigram sweep of
the catalogue is quadratic in that density, not in the row count. Measured: a ten-thousand-owner
catalogue of that shape had not returned after two minutes; the same data scanned in 24 seconds
once each row was limited to its nearest few candidates (a GiST `<->` nearest-neighbour scan,
which is what `gist_trgm_ops` is for; the GIN `%` operator cannot do it). Anything that pairs
records by similarity must be bounded per row and computed away from the request that reads it.

**Unbounded text in a btree index has already caused a production outage here.** Hash a bounded,
canonicalised projection instead. **Narrowed 2026-08-02:** storing unbounded text is not the risk
— a btree entry is capped at roughly a third of a page, so the failure is a hard write error and
it only occurs if the text reaches an *index or unique constraint*. The rule and the
canonicalisation are normative in `specifications/failure-register.md` § Storing the evidence.

**Migration numbers are assigned centrally, in this plan.** A duplicate version hard-errors at
startup before anything applies, and the runner silently skips any version at or below what a
database has already seen — so a shared test database quietly applies nothing.

**Assigned so far:** `0056` — `ownership_import_mappings` (owner ingest); `0057` —
`idx_owners_name_trgm`, `0058` — `owner_merged` audit action and `0059` — owner display-name
index plus contact-email aliases (identity and alias management); `0060` —
`failure_register_entries` and `0060`/`0061` — a verdict subject that may be a repo or a
cookbook, and `0062` — holder kinds narrowed to owner and ticket (the failure register).
The next free number is `0063`; take numbers in the order chunks actually land,
never in the order they were planned. Ingest landed first and therefore took the lowest free number: shipping a higher one would
have made every lower-numbered migration unappliable on any database that had seen the release.
This is not hypothetical — the shared `cmm_test` database already carried a version `0057` from an
earlier, discarded attempt, and had to be recreated before its own tests could run.

**Specs may not contain algorithms**, and a pre-commit hook enforces it by scanning
language-labelled fences. Reference data belongs in unlabelled ones.

---

## Work order

Stated by the product owner, 2026-08-02. This supersedes any ordering in the part files.

**Why this order:** matching faults and data-quality gotchas cannot be discovered until
there are real owners to match against. Ingest first is what makes the rest testable
against reality rather than against fixtures we invented.

1. **Get owner ingest solid.** Owners come in before anything can be matched to them.
   CSV is in; the SQL source (MSSQL/PostgreSQL table-or-query discovery) is deliberately
   deferred — the `RowSource` contract was built for a streaming cursor, so it is a new source
   plus connection handling, not a rewrite. MSSQL needs a driver dependency and its supply-chain
   check first.
2. **Identity and alias management.** **Moved from last to second, 2026-08-02**, after testing
   the import. It is not tidy-up: it is the tool you debug matching failures *with*, and it
   *"will pay off once we start exploring why stuff is not matching in node and git views."*
   Doing it after the matching chunks means diagnosing every miss with no quick way to fix
   one, and the deferred-correction model from journey 1 only works if correcting is easy at
   the moment somebody recognises the person. Scope is in `plans/todo-ownership.md`: put a
   person's aliases on their own page with add and remove inline, and add the merge action
   that folds one owner into another — which is also what makes a correction survive a
   re-ingest.
3. **The failure register.** **Moved ahead of both matching chunks, 2026-08-02.** Journeys 4
   and 6 were declared in scope and had no place in this order at all; they do now, because
   **both automated blocker signals are currently untrustworthy**. CookStyle marks cookbooks
   blocked that demonstrably run fine in production. Test Kitchen is not running for reasons
   that are not technical, and the batches that did run failed on DHCP exhaustion — which the
   rollup counts as cookbook failures, so they are durable false blockers.

   A person recording *"this actually broke"* outranks both, and is the one signal an
   environment problem cannot fake. Real examples exist to seed it, and they double as the
   only available measure of what the automated signals **miss**. It needs no matching: a
   handful of entries assigned by hand is the whole of the ownership requirement.

   Behaviour is `specifications/failure-register.md`. **Two-sided, confirmed by the product
   owner 2026-08-02:** it records a failure nothing detected *and* overrules a wrong automated
   verdict, both with a mandatory reason. It is a third verdict source in the existing readiness
   rollup, not a parallel list.
4. **Node matching.** Nodes resolve to their owners. Found to be the low-hanging one on the
   previous attempt. **Demoted, 2026-08-02** — see § Ownership only has to be right where
   somebody has to act. Scope it against the blocking set rather than the estate, and
   establish that set first: it may be small enough to assign by hand, in which case matching
   is a convenience rather than a requirement.
5. **Git repo matching.** Repos resolve to their owners. The previous attempt treated this
   as the hard one because it assumed a re-keying job. Diagnosing that failure suggested the
   repo name is **already carried alongside the URL and already used elsewhere**, so the
   re-key may not be needed here at all. **Verify before assuming either way** — the repo
   URL gotcha below is why the assumption was made, not evidence that it holds. **Demoted on
   the same grounds as node matching**, and may never be built.

**The incoming ownership data is not clean.** It identifies people inconsistently: sometimes
an email, sometimes a username, sometimes a team. Matching has to cope with all three, and
alias management is what makes it correctable when it gets one wrong. Anything built on the
assumption of a single clean identifier will be wrong on real data.

Chunk part files carry per-feature detail from an earlier draft. They are **subordinate to the
journeys and this work order** — where they disagree, these win.

---

## How to work

**Per chunk:** failing tests first, implement, green, then an independent adversarial review of the
diff against that chunk's acceptance criteria whose job is to *refute* that it is done. Fix what the
review confirms, re-run.

There are **two separate gates**, and they are not the same conversation.

**Gate 1 — the product owner reviews every chunk before the next one starts.** A finished chunk is a
full stop, not a handover to the next item. Report what was built and wait. The purpose is to catch
drift early, while it is one chunk's worth of work to unpick rather than the whole MVP's. Do not
begin the next chunk unprompted, and do not treat a green test suite as the signal to continue —
"tests pass" and "this is what I asked for" are different claims, and only the product owner can
make the second.

**Gate 2 — nothing merges to `main` until the whole MVP is complete and the product owner has tested
it.** Never ask for merge permission at the end of a chunk. Stated 2026-08-02, after an earlier
attempt had to be backed out.

**All chunks share this one branch**, rather than a branch each. That is what makes gate 2 cheap: if
the MVP is abandoned, it costs one branch deletion and nothing on `main` has to be unpicked.
Incrementally merging green chunks is precisely the thing that would need unpicking. The branch
sitting unmerged for the length of the MVP is the intended state, not drift — the session-start
branch sweep must not report it as stale or propose merging it.

**Verify before you rely.** Any citation here was true when written and the tree moves. Open the file.
A mismatch means the surrounding reasoning may also be stale — report it rather than adjusting quietly.

**Stop rather than widen.** A chunk that cannot reach green inside its own scope reports; it does not
reach into another chunk's files.

**Repo hygiene.** This repository is public. Code, specs, comments, commit messages, fixtures and UI
copy describe capability only — no customer identifiers, no characterisation of any organisation or
its people, no scheduling detail. Use `example-corp` / `acme` placeholders.

---

## Ownership only has to be right where somebody has to act

Stated by the product owner, 2026-08-02. **This bounds the whole problem and reorders the work.**

An owner needs to be accurate when a cookbook or repo has a problem somebody must fix. A repo with
no detected problem can wait. So the set needing accurate ownership is not the estate — it is
*blocking and unowned*, which is a join over data that already exists, and is smaller than the
estate by orders of magnitude.

This is the journey-1 deferral generalised: correction was already deferred to the point of use;
so is establishment. Not a new principle, the same one applied a level up.

What follows:

- **Manual assignment is probably sufficient.** If the blocking set is hundreds rather than
  thousands, working through it by hand in the UI that already exists is practical — which makes
  automated matching, candidate-provider ranking and identity re-modelling optimisations on a
  tractable list rather than prerequisites. **Establish the size of that list before building any
  of them.**
- **Ordering is already derivable** — unowned and blocking, sorted by affected nodes, is blast
  radius crossed with the ownership gap.
- **It is rolling, not a cleanup.** "No detected problem *yet*" depends on detection coverage,
  which is known incomplete: static coverage of Ruby removals has gaps, and only cookbooks
  reachable from node objects are tested at all. The deferred set will keep producing work as
  coverage improves, so this is a demand-driven activity with no finish line — which is the right
  shape, and should not be planned as a one-time exercise.

## Blockers are discovered serially and predicted in parallel

Stated by the product owner, 2026-08-02, describing how the customer works today: a daily
speculative run against the new Chef version, installed side by side with the current one,
surfaces the cookbooks blocking each runlist. A run stops at the first failure, so fixing it
reveals the next — a runlist with three blocking cookbooks takes three cycles to enumerate.

**CMM already holds the whole set.** Static readiness records every blocking cookbook per node,
not just the first, so the second and third are known on the day the run is still failing on the
first. Fixing them together collapses three cycles into one, and — the point for this plan —
identifies all three owners at once so the work dispatches in parallel instead of one owner per
day. That is journey 2 with the sequencing removed.

Bounds on that, all of which matter before anyone dispatches work from the prediction:

- **Static and runtime see different things.** The scan reads what it can; the run finds what
  actually breaks. Neither is a superset, so the prediction is a head start, not a replacement.
- **Membership is predicted, order is not.** Which failure comes first depends on runlist order
  and compile-versus-converge phase. Presenting the set as a sequence would be a false claim.
- **The blind spot inverts here.** Cookbooks invoked through an override runlist or a special job
  are absent from static readiness, and a real run is the only thing that finds them. For that
  class the speculative run is the better detector, not the slower one.

**The prediction stands without run data.** CMM does not receive the customer's converge history
and may not for a while, for reasons that are not technical. It does not need to: naming all the
blockers in a runlist rather than the one somebody is failing on today comes from static readiness
alone. Whether that prediction can be *graded* against real failures — and what that would take —
is an event-ingest question, recorded in `plans/todo-event-ingest.md` § Why this matters at the
customer.

## Decisions already taken

- Accountability attaches to git repos, keyed on name.
- A commitment holder may be an owner, a user, or a ticket reference.
- The owner filter matches direct assignments only, and says so in the UI.
- Fuzzy identity matches are suggested for confirmation, never applied automatically.
- Unmatched owner strings on import are recorded, not rejected.
- Counting grain is `(organisation, node)`, so the unowned remainder reconciles to the fleet.
- Editing the four specs outside the ownership set to remove a config flag that exists in no code is
  authorised.
- Superseded ownership specs are removed in this branch, as their own itemised commit.
