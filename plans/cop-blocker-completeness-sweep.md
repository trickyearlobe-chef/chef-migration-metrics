# Cop-blocker completeness sweep (false-negative discovery) — NEXT CHUNK

Handoff for a fresh thread. Everything below is committed on branch
`fix/cookstyle-polymethod-cop` (unmerged).

## Goal

Find curated-Blocker **gaps**: cops that flag something genuinely removed/broken on
the target Chef (CC19) but are **not** in our curated Blocker set, so they currently
default to Review — a *hidden* blocker. This is the dangerous direction per
`specifications/cop-classification.md` (asymmetric confidence: a missed blocker lets a
broken cookbook look ready).

## What's already done (do NOT redo)

- All 26 curated Blocker cops validated vs real CC19 → 6 over-claims demoted,
  `Lint/DeprecatedClassMethods` re-dated 18→19 and **fully disambiguated** (File/Dir
  .exists? + ENV.clone/dup/freeze = Blocker; Socket.gethostby* + iterator? + attr =
  Review). Commits `95515c6`, `d9eef1d`, `8b6254e`, `2b4b941`.
- That was **false-positive** validation only (checking cops we *already* flagged).
- The **false-negative** sweep (this chunk) has NOT been done.

## Approach (on the lab box — see memory `cmm-validation-box`)

Box: `root@172.24.1.198` (`cmm.trickyearlobe.com`), Chef Workstation 26 = Chef
19.3.15 + cookstyle 8.6.10 + chefspec. Run Ruby via `chef exec ruby`.

1. Enumerate ALL cookstyle cops in the migration-relevant departments
   (`Chef/Deprecations/*`, `Lint/*`, `Chef/Correctness/*`) via the RuboCop registry.
2. Extract each cop's probeable targets — `RESTRICT_ON_SEND`, and the cop's own
   authoritative tables (e.g. `DeprecatedClassMethods::PREFERRED_METHODS`). Poly cops
   need per-message handling.
3. **Behaviourally** probe each target against CC19 (call + catch — `respond_to?`
   lies; a runtime `TypeError` counts as broken → Blocker).
4. Diff: any cop whose target is removed/raises on CC19 but is **not** in our curated
   Blocker set (`internal/remediation/copmapping.go` `RemovedIn`) → a **missing
   blocker** candidate.
5. Close the two known loose ends too: `DeprecatedPlatformMethods` (probe its
   `[:provider_for_resource, :find_provider, :find_provider_for_node, :set]`) and the
   4 non-symbol cops (`ChefRewind`, `CookbookDependsOnCompatResource`,
   `CookbookDependsOnPartialSearch`, `LegacyNotifySyntax`).

## Tools + lessons (don't re-learn)

- Extend `scripts/cop-validation/cop_validator.rb` + `poly_disambiguate.rb`.
- Behavioural probe, not `respond_to?`. Resolve cop classes with `gsub` not `tr`.
- `chef exec` loads the client's vendored cookstyle 8.6.10 (fine).
- Arg-form deprecations (`attr :x, true`) are NOT removals → Review.
- `RESTRICT_ON_SEND` over-approximates → prefer the cop's authoritative table.

## Acceptance

A reconciliation list `(cop → removed-on-CC19? → in curated Blocker set?)` with the
missing-blocker candidates flagged; add confirmed ones to `copmapping.go` with tests,
or record why not. Update `plans/todo-tech-debt.md` and delete this file when done.
