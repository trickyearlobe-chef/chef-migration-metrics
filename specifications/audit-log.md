# Audit Log — Who Did What

> **Status: proposed, not built.** Nothing in this file describes shipped behaviour. What
> exists today is described under *Where we are now*; everything after that is intent.

## TL;DR

One append-only record of deliberate human actions across the whole application — ownership
changes, configuration edits, credential changes, and triggered processes like a full rescan —
answering *who did what, when, and to what*.

Today there are two narrow audit tables and no coverage of configuration or triggered processes
at all. This describes the general facility they should converge on.

## Why

Three questions have no answer today:

- **"Who changed this setting, and what was it before?"** Configuration is edited through the UI
  and stored in the database. The store keeps who last touched a key, and nothing else — no
  previous value, no history, and the answer is overwritten by the next edit.
- **"Who triggered this, and when?"** Actions that invalidate large amounts of work (rescan
  everything, purge, restore) leave a line in the application log and nothing durable. The
  application log is subject to its own volume and retention pressure, and is not a record
  anybody is expected to reconstruct an account from.
- **"What happened around the time this broke?"** Answering it means reading several tables that
  do not share a shape, plus the application log, and correlating by eye.

The customer ships application logs to a broadly-readable Splunk instance, so the application log
is also the wrong place for a record that names people and settings.

## Where we are now

Verify each of these before relying on them; they were true when written.

- **`ownership_audit_log`** — ownership mutations only. `owner_name` is `NOT NULL` and the indexes
  are owner- and entity-shaped, so a config key or a job has nowhere to go. Retention is a single
  configurable age, purged on a timer. Its action vocabulary is **no longer constrained** —
  migration `0058` removed that, so the invariant below already holds here.
- **`cookstyle_audit_log`** — cop reclassification and custom-cop changes. Its own comment says it
  mirrors the ownership table but is cop-centric. Same shape, different subject, no shared code.
- **`config_store`** — carries who last updated a key and when. Current state only.
- **Triggered processes** — not recorded anywhere durable.

**The pattern has been copied rather than shared, twice.** That is the finding that motivates this
spec: a third narrow table would be the third time.

## What an entry is

An entry records **a deliberate action by an actor against a subject**. Not a state change, not a
system event, not a metric — those belong to the application log and to the tables that own the
state.

Every entry carries: **when**, **who** (a username, or an explicit non-human actor for scheduled
and startup work), **what** (the action), **what it was done to** (a subject type and a subject
key), and **the particulars** as a structured document.

**Subject is a pair, not a column per subject kind.** An owner, a cop, a configuration key and a
job all have to fit, and adding a column per kind is how the current tables ended up
un-generalisable.

## Invariants

**Append-only.** Entries are never updated. Only retention removes them.

**An action the code does not recognise is still recorded.** The action vocabulary must not be a
database constraint. A constrained vocabulary means every new action needs a schema migration
before it can be written — and, combined with the next invariant, means the failure is invisible.

The cost of dropping the constraint is that a typo is recorded rather than rejected. That is the
better failure: a mislabelled entry is visible and correctable, where a rejected one is gone.
Spelling is a job for shared constants at the call sites, not for the database.

**A failed audit write is never silent.** Today the ownership audit write logs a warning and
carries on, so an entry rejected by the action constraint produces an action that *looks* audited.
Either the audit write shares the transaction of the change it describes — so both happen or
neither does — or its failure is surfaced where somebody sees it. Best-effort auditing is worse
than none, because it reads as coverage.

**Redaction is by construction, not by discipline.** Secret values never enter an entry. A
credential change records *that* it changed, by whom, and which key — never the value, old or new.
The store already knows which keys are secret; the audit record must derive that rather than
relying on each call site to remember.

**Retention varies by subject.** One age for "somebody edited an assignment" and "somebody
replaced the encryption key" will be wrong for at least one of them.

## What it is not

- **Not the application log.** That is diagnostic, high-volume, and shipped off-box. This is
  low-volume, deliberate, and stays in the database.
- **Not a change-data-capture feed.** Only actions a person (or a named scheduled process) takes,
  not every row that changes.
- **Not a state history.** It records that a configuration key changed and what the change was
  understood to be. Reconstructing every past state from it is not a promise it makes.

## Reading it

One place to ask "what happened", filterable by actor, by subject, by action and by time. The
existing ownership audit view is a filtered read of the same thing, not a separate screen backed
by a separate table.

## Migrating what exists

The two existing tables have shipped and carry real rows, and the ownership audit view queries
owner-shaped columns.

**Build the general facility for the uncovered cases first — configuration and triggered
processes — and fold the two existing tables in once it has proved out.** A three-way migration
before anything uses it puts the risk in the wrong place. Nothing already recorded is discarded.

## Related

- [configuration](configuration.md) — the configuration surface whose edits this records.
- [encrypted-config-store](encrypted-config-store.md) — where configuration lives, and which keys
  are secret.
- [secrets-storage](secrets-storage.md) — credential storage and rotation.
- [logging](logging.md) — the application log, which this is not.
- [ownership](ownership.md) and [ownership-datastore](ownership-datastore.md) — the existing
  ownership audit log.
