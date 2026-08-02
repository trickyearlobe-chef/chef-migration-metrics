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
silently** — a wrong owner is worse than no owner. Fuzzy matching already exists in CMM and
may serve this.

**Teams do not exist as data, and owners can be several people.** There is no source for who
is in a team, so teams would have to be constructed by hand. **For MVP that is avoided:**
filtering on a multi-select of users covers the working-group case without a team entity —
which is why journey 1 says people rather than teams.

The source data already carries **multiple owners for a single cookbook or node**, quite
possibly the customer's own workaround for the missing team data. That leaves an **open
question: when several people own something, who is specifically on the hook to fix it?**
Unresolved, and it lands on journey 2, which promises "the person accountable".

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
has been given. Recorded by hand, with no telemetry configured.

**5. "Get our ownership data in."** An admin brings existing ownership data in from a CSV
**and/or a database** — probably MSSQL, possibly PostgreSQL. **We will not have the schema, so
discovery happens in the UI:** choose a table and pick fields from it, **or** supply a SQL query
and pick fields from what it returns, with a **data preview** to make that choice possible. It
must succeed while the source data is still inconsistent, and report what matched, what did not,
and why.

---

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
genuinely differ, resolution should **fail** and route to a human-confirmed suggestion — an empty
result is correct, a wrong owner is not.

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

**Unbounded text in a btree index has already caused a production outage here.** Hash a bounded,
canonicalised projection instead. The rule and the canonicalisation are written and normative in
`specifications/failure-register.md`.

**Migration numbers are assigned centrally, in this plan.** A duplicate version hard-errors at
startup before anything applies, and the runner silently skips any version at or below what a
database has already seen — so a shared test database quietly applies nothing.

**Specs may not contain algorithms**, and a pre-commit hook enforces it by scanning
language-labelled fences. Reference data belongs in unlabelled ones.

---

## Work order

Stated by the product owner, 2026-08-02. This supersedes any ordering in the part files.

**Why this order:** matching faults and data-quality gotchas cannot be discovered until
there are real owners to match against. Ingest first is what makes the rest testable
against reality rather than against fixtures we invented.

1. **Get owner ingest solid.** Owners come in before anything can be matched to them.
2. **Node matching.** Nodes resolve to their owners. Found to be the low-hanging one on the
   previous attempt.
3. **Git repo matching.** Repos resolve to their owners. The previous attempt treated this
   as the hard one because it assumed a re-keying job. Diagnosing that failure suggested the
   repo name is **already carried alongside the URL and already used elsewhere**, so the
   re-key may not be needed here at all. **Verify before assuming either way** — the repo
   URL gotcha below is why the assumption was made, not evidence that it holds.
4. **Alias management.** Last, but **not optional** — see below.

**The incoming ownership data is not clean.** It identifies people inconsistently: sometimes
an email, sometimes a username, sometimes a team. Matching has to cope with all three, and
alias management is what makes it correctable when it gets one wrong. Anything built on the
assumption of a single clean identifier will be wrong on real data.

Chunk part files carry per-feature detail from an earlier draft. They are **subordinate to the
journeys and this work order** — where they disagree, these win.

---

## How to work

**Per chunk:** its own branch, failing tests first, implement, green, then an independent adversarial
review of the diff against that chunk's acceptance criteria whose job is to *refute* that it is done.
Fix what the review confirms, re-run, then a human approves the merge. Nothing self-merges.

**Verify before you rely.** Any citation here was true when written and the tree moves. Open the file.
A mismatch means the surrounding reasoning may also be stale — report it rather than adjusting quietly.

**Stop rather than widen.** A chunk that cannot reach green inside its own scope reports; it does not
reach into another chunk's files.

**Repo hygiene.** This repository is public. Code, specs, comments, commit messages, fixtures and UI
copy describe capability only — no customer identifiers, no characterisation of any organisation or
its people, no scheduling detail. Use `example-corp` / `acme` placeholders.

---

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
