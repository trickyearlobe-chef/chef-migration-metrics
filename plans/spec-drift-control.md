# Spec/Plan Drift Control

Goal: stop specs, plans, and "done" status drifting from the code. Root cause is
duplication without a mechanical link — the same fact stated in two places that
nothing checks for agreement. Each chunk removes a duplicate or adds a link.

Sequence: A + B/D (prevention, cheap) → E (diagnose current backlog) → C (durable
linkage, prioritised by E's findings).

## Chunk A — Lint implementation out of specs [prevention]

Scope: `.githooks/pre-commit` (extend, new §5 after the spec-size block).
Block specs that contain *implementation*; allow *contracts/reference data*.
- Block: fenced code in implementation languages (```go ```ruby ```rust ```python);
  runs of ≥6 consecutive `NAME = value` / `NAME: value` constant lines outside a
  fence (the "copy of code" rot).
- Allow: ```json ```http ```yaml ```sql ```text and ```ts/```tsx blocks whose body
  is only `interface`/`type` declarations (API response/contract shapes).
- Conservative: when ambiguous, warn rather than block; tune on real specs.
Steps: add check fn; run against all current `specifications/*.md` to find existing
violations (report, don't auto-edit); document skip path (`--no-verify`).
Acceptance: a spec with a ```go impl block or a long constant run is blocked; the
existing contract blocks (e.g. `interface TableSize` in `system-health-frontend.md`)
pass. List of pre-existing violations captured for cleanup (feeds E/C).

## Chunk B/D — CLAUDE.md rules [prevention, DONE 2026-06-23]

- B: plans hold only *open* work; "done" = git history + passing tests, never
  re-asserted in prose. Decisions (not status) may be recorded, but completed items
  leave the plan.
- D: verify-before-record — any claim about code state/completion must cite
  `file:line @ <short-SHA>` checked at that commit; never from memory/stale context.
Both added to CLAUDE.md (Planning / Quality Maintenance). No further work.

## Chunk E — One-time drift sweep [diagnose] (opt-in: user approved)

Scope: a Workflow that fans out over `specifications/*.md`. Per spec: extract
acceptance criteria / intent claims, check each against the code (grep + targeted
test/build evidence), classify matched / diverged / unverifiable, cite file:line.
Output: `plans/spec-drift-report.md` — per-spec divergence list with evidence and a
suggested action (fix code, fix spec w/ owner sign-off, or add test).
Acceptance: report covers every spec; each divergence has evidence + action. No spec
or code edits in this chunk — it only reports. Use the Workflow tool (many agents).

## Chunk C — Link acceptance criteria to tests [durable linkage]

Scope: specs + tests + a small checker script. Phased, prioritised by E's report.
- Give each spec acceptance criterion a stable ID (e.g. `[BKS-7]`).
- Reference the ID in the test that proves it (test name or comment).
- Add a script (`scripts/spec-coverage.sh` or Go tool) that parses criteria IDs from
  specs, greps tests for each ID, and reports criteria with no linked test
  ("unverified intent"). CI-runnable.
Steps: pilot on one high-divergence spec from E; agree the ID convention; roll out.
Acceptance: script lists every criterion ID and its linked test(s); unlinked ones are
flagged; folder-scoping-style gaps show as "no passing test", not silent.

## Notes
- Branch: `chore/spec-drift-control`. A is code (hook); E/C produce reports/scripts.
- Pending (separate): `docs/ui-revamp-followup-reconcile` awaiting merge.
