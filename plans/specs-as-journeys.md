# Experiment — specs carry journeys, code carries contracts

Hypothesis: the assistant hallucinates requirements because specs, plans and memory carry
technical claims that nothing re-validates. Remove the *ability* to state a technical claim
and the hallucination has nothing to feed on.

Whole scope, not a pilot — the material is cross-cutting, so a partial conversion leaves
the un-converted spec next door to be believed. Reversible: archive first, delete nothing,
one branch.

## What the evidence says (verified 2026-08-04 @ a0abd66)

- 129 spec files, 20,422 lines, describing 225,378 lines of Go/TS. ~15 untouched since
  2026-06-02; code moved to 2026-08-03.
- `specifications/semantic-contracts.md` §6/§7 assert a *plan* as a *state*: a duplicate
  read-time derivation "will be eliminated in Phase 2". `internal/webapi/check_status.go:109`
  and `:166` still hold `deriveCookstyleStatus` / `deriveKitchenStatus`. Never happened,
  nothing noticed.
- `internal/analysis/semantic_contracts_test.go` exists but tests only the write-time side.
  The one thing the contract exists to prevent — two implementations drifting — is untested.
- The "NOT TO BE TRUSTED" banner is already atop `overview.md` and twice in `CLAUDE.md`.
  The failure happened anyway. **Prose enforcement is disproven; do not propose more of it.**
- The same disease is in memory, which loads every session and is exempt from every rule:
  `cookstyle-branch-state.md` carries commit SHAs, function names, table names and offence
  counts; `event-ingest-build-state.md` says "spec `specifications/event-ingest.md` is
  authoritative" — a memory instructing the assistant to trust a spec.

CLAUDE.md already says "specs reference contracts, never copy them". The rule is right and
is ignored. This makes it mechanical instead of aspirational.

## The three rules

- **R1 — a spec is a journey.** Who the person is, what they are trying to get done, what
  must be true to succeed, how they would know it worked — in that person's words. Nothing
  else. No tables, columns, endpoints, packages, function names, paths, config keys, code.
- **R2 — a contract is a test.** Any "X must equal Y", "the canonical function is", "these
  are the fields" becomes an executable test, or it goes. If it cannot be written as a
  failing test, it was never a contract.
- **R3 — superseded material leaves the retrieval path.** Moved, not deleted. Ideas are
  preserved somewhere nothing reads by default and everything is marked non-current.

## Enforcement — structural, because prose failed

Extend `.githooks/pre-commit` to block a staged `specifications/*.md` containing a fenced
code block, `internal/`, a `.go`/`.tsx`/`.sql` path, SQL DDL/DML keywords, an HTTP verb +
path, or `func`/`()` call syntax. Warn on bare `snake_case`.

The hook replaces the A/B measurement a pilot would have given: if a technical claim cannot
be written, there is nothing to test the removal of.

## Steps

1. **Archive first, in its own commit.** `git mv` every spec to `archive/specifications/`
   with a README marking it historical. Nothing is rewritten yet, so the revert is exact
   and no requirement can be silently lost in a rewrite.
2. **Enforcement.** Hook check + its own test.
3. **Classify all 129** (mechanical, one pass): design-only/superseded → stays archived;
   describes existing code → journey extracted, rest dropped; contract claims → test.
   Expect `web-api-*` (20 files) to be almost entirely API documentation the code owns.
4. **Write the journeys** back into `specifications/`, one per feature area. Target ~25–30
   files, not 129.
5. **Contracts to tests.** Flagship: a differential test asserting the write-time and
   read-time cookstyle/kitchen derivations agree. It should fail on first run.
6. **CLAUDE.md and memory** — see below.

## CLAUDE.md changes (proposed, needs sign-off)

- Delete both "NOT TO BE TRUSTED" blocks once the hook lands. Keeping a disproven warning
  teaches that warnings are the mechanism.
- Collapse the 10-bullet Specifications section to R1–R3. Its "specs hold intent,
  invariants, expected behaviour and reference data" is the loophole the 20k lines came in
  through, and it contradicts "reference contracts, never copy them" four bullets later.
- **Lower the gate on small work.** "Features need a specification" + "write one first" +
  "no implementation without a plan in `plans/`" is three documents before any code, and is
  the direct tax on shipping a good-enough MVP and iterating. Proposal: a change needs a
  *journey* — one or two sentences of who wants what and how they will know it worked — and
  a plan only when the work spans more than one session.

## Memory changes (proposed, needs sign-off)

42 memories. The 11 `feedback` ones (how to work) and the 8 `reference` ones (lab SSH,
release runbook, validation box) are the durable value. The 22 `project` ones are largely
status — the thing CLAUDE.md forbids in plans ("done lives in code, not prose") and permits
in memory.

- Strip build-state memories to the load-bearing *lesson* and drop the status. The
  defensive-decode lesson in `event-ingest-build-state` is worth keeping; the commit list in
  `cookstyle-branch-state` is not.
- Delete the "spec X is authoritative" line from `event-ingest-build-state`.
- Rewrite or delete `specs-optimized-for-llm` — it justifies the 129-file fragmented layout
  and the 500-line cap, and will otherwise argue against this work in a later session.

## Acceptance

- The hook blocks a spec containing a code fence or a package path, and passes a journey.
- Every technical claim removed is either an executable test or in `archive/specifications/`.
- The differential derivation test exists and its first run fails.
- `git revert -m 1 <merge>` restores the previous state exactly.

## Not in scope

`plans/` content. Same disease, but changing it in the same pass makes the result
unreadable. It follows once the spec rules are proven.
