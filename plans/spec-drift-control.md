# Spec/Plan Drift Control

Goal: stop specs, plans, and "done" status drifting from the code. Root cause is
duplication without a mechanical link — the same fact stated in two places that
nothing checks for agreement. Each chunk removes a duplicate or adds a link.

Core principle (CLAUDE.md § Specifications): this is a code-first repo with no
externally-owned contracts of our own — so specs **reference contracts, never copy
them**. Source of truth is the code that owns the shape: internal Go types for our
shapes; `vcenter.go`/`proxmox.go` for external Proxmox/VMware shapes we consume but
don't own. A pasted struct/interface is drift surface; replace with a reference +
the invariants code can't express.

Sequence: A + B/D (prevention, cheap) → E (diagnose current backlog) → C (durable
linkage, prioritised by E's findings).

## Chunk A — Lint implementation out of specs [prevention, DONE 2026-06-23]

`.githooks/pre-commit` §5 added + tested across all 118 specs. Blocks executing
statements (`:=`, `return <expr>`, control flow) inside go/ruby/rust/python fences
(= function body/impl); warns on ≥8-line constant-list runs. Allows contracts
(JSON/YAML, struct/interface/type/const, bare method signatures). Verified: blocks a
synthetic impl spec with line citations, passes contract specs.

Also added (per the "reference, don't copy" principle): a **warning** on
struct/interface blocks inside go/ts fences — copied contracts that should become a
reference + invariants. Warns on 6 specs (the copied-contract backlog):
`websocket-log-streaming.md`, `diagnostic-bundle.md`, `system-health-package-layout.md`,
`system-health-frontend.md`, `system-health-api-endpoint.md`,
`system-health-configuration.md`. Cleanup needs owner sign-off per spec (CLAUDE.md);
remediate opportunistically or via E. Warning, not block — won't break commits.

Two open follow-ups surfaced:
- **Pre-existing violation:** `specifications/websocket-log-streaming.md` has one real
  impl block (`json.Unmarshal` / `if err != nil { return nil }`, ~9 lines). Editing a
  spec needs owner sign-off (CLAUDE.md) — convert to intent prose / signatures. The
  hook will block the next commit that touches this file until fixed.
- **Enforcement gap:** the hook is local-opt-in via `make install-hooks` (sets
  `core.hooksPath=.githooks`). It IS active in this clone (verified
  `git config core.hooksPath` = `.githooks`), but is NOT run in CI, and any clone
  that hasn't run install-hooks — or a `--no-verify` commit — skips it. So §5 and the
  existing secret/deny/spec-size checks aren't a hard guarantee. A CI job running the
  hook's checks on PRs would close this — see `todo-ci.md`.

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
Add a **copied-contract** category: spec blocks that paste a code-owned struct/
interface (the 6 specs above + any others) → remediation = replace with a reference
to the authoritative type + invariants.
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
