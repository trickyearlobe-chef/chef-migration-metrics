# Plan — CookStyle Full-Ruleset Scanning & Addon Cops

Spec: `specifications/cookstyle-full-ruleset.md`. Status: **not started** (design
approved 2026-06-30).

Branch: **this branch** (`feature/cookstyle-violations-browser`). Chunks A–C are a
**merge-blocker**: the reclassification feature introduced on this branch is
broken as shipped — a cop classified Blocker outside `Chef/Deprecations,Chef/
Correctness` silently never runs (the `--only` filter excludes it), so the
classification claims a block the scan can never produce. That must be fixed
before this branch merges. D (addon cops) and E (autocorrect widening) are
additive but land on the same branch.

Chunks are ordered; A is the foundation. Each = one session.

## Chunk A — Drop `--only`; one shared scan/autocorrect arg+sidecar helper

Scope: `internal/analysis/cookstyle.go` (`buildCookstyleArgs`,
`writeCookstyleTargetConfig`), `internal/remediation/autocorrect.go`
(`buildAutocorrectArgs`, `writeAutocorrectTargetConfig`).
- Extract the duplicated sidecar/args construction into one shared helper.
- Remove the `--only Chef/Deprecations,Chef/Correctness` narrowing from the scan
  (autocorrect handled in Chunk E).
Steps (TDD): test that a Blocker-classified cop outside the two departments (e.g.
`Lint/DeprecatedClassMethods` at target ≥18) now produces an offence and yields
`blocked`; test that a cosmetic cop (`Style/*` at convention) is weight-0 and
non-blocking. Then make the change.
Acceptance: full ruleset runs; verdict/complexity unchanged for cosmetic cops;
previously-hidden classified blockers fire. Existing scan tests green.

## Chunk B — Seed Noise curated defaults for cosmetic departments

Scope: `internal/analysis/cop_classification_defaults.go`, resolver
(`cop_classification.go`).
- Open Q: `curatedDefaults` is keyed by exact cop name; Noise-seeding whole
  departments needs **prefix** defaults (e.g. `Chef/Style/`, `Style/`, `Layout/`)
  → add prefix support to the resolver's curated-default lookup, after operator
  override + RemovedIn, before unclassified.
Steps (TDD): assert common `Style/`/`Layout/`/`Chef/Style/` cops resolve to Noise
(source = curated_default) and contribute 0 complexity; operator override still
wins. Then implement.
Acceptance: cosmetic tail pre-sorts into the collapsed Noise section; Blockers
section stays clean. Depends on A.

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
