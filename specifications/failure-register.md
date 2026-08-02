# Failure Register — What Is Broken, Why, and Who Is On It

> **Status: specified, not built.** Nothing here describes shipped behaviour.

## TL;DR

A person's verdict on whether a cookbook actually works on the target version, recorded with a
reason, and read back every morning.

It exists because the automated signals are wrong in both directions: CookStyle marks cookbooks
blocked that demonstrably run fine, and Test Kitchen reports failures that were really the test
environment falling over. A human who has watched a real converge knows better than either, and
today there is nowhere to say so.

Serves **journey 4** (record a failure nobody predicted) and **journey 6** (the standup view).
Both were declared in scope; neither is built.

## Why a person's verdict has to be first class

**The automated signals disagree with reality in both directions.**

- **False blockers.** A cookbook is marked incompatible and production has been running it for
  months. A Test Kitchen run that never got as far as converging is recorded as a cookbook that
  does not work, and that verdict outranks a CookStyle pass.
- **Missed blockers.** A cookbook breaks on a real converge for a reason no static scan and no
  test suite predicted. Nothing records it, so it is rediscovered by the next person.

Both are the same act: **a person overruling a machine, with evidence.** One says "this is not
actually broken", the other says "this is broken and you missed it". The register holds both.

**Consequence for trust.** Dispatching somebody to fix a false blocker costs their time and the
tool's credibility, and credibility is the harder of the two to get back. A register that lets a
verdict be corrected once, permanently, in the place everybody reads, is what stops the same
argument happening every week.

## What an entry is

**A person's verdict about a git repo**, carrying why they believe it.

- **The subject is the repo, labelled with the cookbook, never a version.** Several versions are
  in use at once and the failure is discussed version-agnostically. This follows the vocabulary
  rule in `plans/ownership-work-attribution.md`.
- **The verdict is two-sided** — this repo is broken, or this repo is not broken whatever the
  scans say.
- **A reason is mandatory.** A verdict with no reason is an opinion, and it will be overturned by
  the next person who disagrees. The reason is what makes it survive.
- **Evidence is optional but expected** — the stacktrace, the run that failed, or the fleet
  observation that contradicts the scan.

Alongside the verdict, an entry carries what journey 4 asks for: **the diagnosis**, **what is
planned about it**, and **a target date where one has been given**. All three are optional: a
failure is worth recording the moment it is seen, before anybody knows what to do about it.
Requiring a plan up front means failures go unrecorded.

**The commitment holder may be an owner, a user, or a ticket reference** — the decision already
taken in the plan. Where work is tracked elsewhere, CMM holds the URL or ticket number and does
not integrate with the system behind it.

## It is a third verdict source, not a parallel list

**This is the most important structural decision.** Node readiness already carries, per blocking
cookbook, the verdicts of every source that had an opinion, each tagged with which source it came
from, and a rule that decides which one wins. Today those sources are CookStyle (from the server
and from git) and Test Kitchen.

**A register entry joins that set as another source, and outranks all of them.** It does not
create a second place where truth lives.

What that buys, without building any of it twice: readiness reflects the human verdict; the
existing rollups, list views and exports pick it up; and the disagreement stays visible, because
the losing verdicts are retained rather than overwritten. Somebody looking at a repo can see that
a scan called it incompatible, a person overruled it, and why.

**A parallel list would have to be joined to by every consumer**, and the ones that forgot would
quietly show the machine's answer. That is the failure mode this decision exists to prevent.

## The standup view

Journey 6, and the reason anybody bothers recording anything. Without it, journey 4 is data entry
nobody reads and people stop doing it within a fortnight.

Read together every morning, it must answer:

- **Which repos are currently broken**, labelled by cookbook — what the team says out loud.
- **Why each one is broken** — the reason, at a glance, not a click away.
- **What is being done about it** — the plan, the holder, and the target date where there is one.
- **Whether the list is getting too large.** The size and the direction matter as much as the
  contents: a register that is growing is a different message from one that is shrinking.

**That last requirement shapes the entry, not just the view.** Direction of travel needs entries
to have a lifecycle — raised, and later resolved — and it needs resolution to be recorded rather
than the entry disappearing. An entry that is deleted when fixed makes the list look permanently
static.

## Invariants

**A human verdict outranks every automated one.** Where they disagree, the person wins and the
disagreement stays on the record.

**Every verdict carries a reason.** No exceptions — this is what distinguishes the register from
an opinion and what lets a later reader judge whether it still holds.

**Verdicts are superseded, never silently replaced.** Who said what, when, and why is the point.
A reversal is a new verdict, and the old one remains readable.

**Entries are never keyed on a version.** A verdict about a repo applies to the repo.

**Resolution is recorded, not deleted.** Journey 6 needs the direction of travel, which is
unavailable if resolved entries vanish.

## It is also the only measure of how wrong the tools are

A side effect worth protecting deliberately.

Every entry that says *"broken, and no tool saw it"* is a **false negative** of the automated
signals. Every entry that says *"not broken whatever the scan says"* is a **false positive**.
Nothing else in the product can produce either number.

That makes even a handful of entries valuable out of proportion to their count: they are the only
available evidence of how far the blocking list can be trusted, and therefore of whether work
should be dispatched from it at all. **The register should be readable as an accuracy report of
CookStyle and Test Kitchen**, not only as a list of broken things.

## Storing the evidence

A stacktrace is unbounded text. Storing it is not a problem — the database handles large text
columns without difficulty.

**The risk is narrow and specific: never let unbounded text into an index or a unique
constraint.** A btree entry is capped at roughly a third of a page, which a few tens of lines of
trace already exceeds, and the failure mode is a hard write error rather than slowness. This has
caused a production outage in this project before.

If repeat occurrences of the same failure need to be recognised as the same failure, **hash a
bounded, canonicalised projection and index that** — the pattern already used by the CookStyle
offence fingerprints. Canonicalisation must remove what varies between runs and keep what
identifies the failure; the projection must be bounded before it is hashed, not after.

## Out of scope

- **Integrating with the system where work is tracked.** CMM holds a reference; it does not read
  or write the other system.
- **Inferring entries from telemetry.** Entries are made by people. Deriving them from converge
  failures is a separate question and depends on ingest that is not in place.
- **Per-version verdicts.** Deliberately excluded — see the vocabulary rule.

## Related

- `plans/ownership-work-attribution.md` — journeys 4 and 6, the vocabulary rule, and the decision
  that a commitment holder may be an owner, a user or a ticket reference.
- `specifications/dual-compatibility-signals.md` — how the existing automated verdicts are
  combined, which this extends rather than replaces.
- `specifications/ownership.md` — owners, and how a commitment holder resolves to one.
