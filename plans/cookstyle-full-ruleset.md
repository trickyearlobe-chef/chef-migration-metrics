# Plan — CookStyle Full-Ruleset Scanning & Addon Cops

Spec: `specifications/cookstyle-full-ruleset.md`. Status: **not started** (design
approved 2026-06-30).

Branch: **this branch** (`feature/cookstyle-violations-browser`). Chunk C is a
**merge-blocker** (functional verification that the previously-hidden classified
blocker now fires end-to-end). D (addon cops) and E (autocorrect widening) are
additive but land on the same branch.

Chunks A (drop `--only` + shared helper) and B (Noise prefix defaults for
cosmetic departments) are complete. Remaining chunks are ordered; each = one
session.

## Chunk C — Verify the `Lint/DeprecatedClassMethods` gap is closed

Scope: functional test only (likely automatic after A).
Steps: functional scan of a cookbook using `File.exists?` at target ≥18 → asserts
a `Lint/DeprecatedClassMethods` offence + `blocked` rollup.
Acceptance: the curated blocker fires end-to-end. Depends on A.

## Chunk D — Addon cop files (config + require injection + isolation)

Scope: `internal/config` (config type + validation), admin config API/UI for
`analysis_tools.cookstyle_addon_cop_paths` (a plain path/glob list), the shared
sidecar helper (inject `require:` per resolved `.rb`), startup/admin validation
surface.
Steps (TDD): glob resolution (file/dir/glob); requires injected into the sidecar;
**load-failure isolation** — a broken `.rb` does not mark cookbooks errored, it is
logged/surfaced; an addon cop's offences classify + roll up like any cop;
a `pending` cop is enabled so it actually runs.
Acceptance: an operator-placed cop (e.g. the `=~`-on-node-attributes example)
appears in scans, classifiable, blocking when set to Blocker. Trust = on-disk
only (no web upload). Depends on A.

## Chunk E — Autocorrect preview covers the full ruleset (no skipping)

Scope: shared autocorrect arg helper (`internal/remediation/autocorrect.go`).
- Drop `--only` from the autocorrect run too (same shared helper as A), so the
  whole-cookbook `--auto-correct` diff includes every available fix — including
  **addon-cop** fixes (addon cops have no embedded remediation mapping, so the diff
  is their only preview). Nothing is excluded at the engine level; what a user sees
  is governed by the UI buckets/filter.
Steps (TDD): an addon cop with an AutoCorrector appears in the diff; built-in
per-cop Before/After (from remediation mappings) still renders inside its
classification section.
Acceptance: all available fixes shown; nothing skipped. Depends on A.
Note: full-ruleset `--auto-correct` is heavier per cookbook (bounded by timeout). A
later refinement could split the monolithic diff per cop for finer in-UI
filtering — not in scope.

## Cross-cutting acceptance

- No surface blocks on cosmetic cops; previously-hidden classified blockers fire.
- Detail view (existing `ClassificationSections`) and autocorrect card render the
  full set with no UI change; Blockers expanded, Noise collapsed.
- Reclassification stays rescan-free.
- `go test ./...` + functional suite + frontend test/lint green.

## Decisions

- Classify by **prefix/department**, not by enumerating cops: the resolver gains a
  prefix tier so one rule covers a whole department (Chunk B). Agreed 2026-06-30.
- **Skip nothing** at the engine level — scan and autocorrect both run the full
  ruleset; visibility is via the UI classification buckets/filter. Agreed
  2026-06-30.

## Open questions (resolve in-chunk)

- Scan / full-ruleset `--auto-correct` performance on large cookbooks (measure in
  A / C / E; bounded by the per-scan timeout).
