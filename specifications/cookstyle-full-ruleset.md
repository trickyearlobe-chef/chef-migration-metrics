# CookStyle Full-Ruleset Scanning & Addon Cops — Component Specification

> **TL;DR** — Run the complete CookStyle ruleset (drop the `--only
> Chef/Deprecations,Chef/Correctness` narrowing) and let **cop classification**,
> not a department filter, decide the rollup verdict and complexity. Operators can
> add real custom RuboCop cops as on-disk `.rb` files referenced by config. The
> existing impact-grouped detail view and autocorrect preview render it — no new UI.

## Overview

A target-version scan currently runs `--only Chef/Deprecations,Chef/Correctness`
(`buildCookstyleArgs`, `internal/analysis/cookstyle.go`). That predates the
classification system and now causes two problems:

- **Classified blockers outside those departments never run.** The curated
  default `Lint/DeprecatedClassMethods` (`File.exists?` removed in Ruby 3 —
  Blocker at target ≥18, `cop_classification_defaults.go`) lives in `Lint/`, which
  the filter excludes. The classification is correct; the scan never produces the
  offence, so the blocker can never fire.
- **No full impact inventory, and no real custom cops.** Operators can't see the
  complete cop set grouped by impact, and the existing "custom cops" are
  line-by-line regex/literal matchers (`custom_cops.go`), not real RuboCop cops
  with AST matchers / autocorrect.

The classification system already separates *detection* from *blocking*: status
blocks only Blockers (plus unclassified offences that severity-fail), and
complexity weights cosmetic offences at 0. So the department filter is a blunt,
redundant pre-filter that hides classified blockers. Removing it and relying on
classification is simpler and more correct.

## Decisions (agreed)

- **Full ruleset.** Drop `--only`; run every cop (Chef + generic RuboCop + addon).
  Classification drives verdict + complexity.
- **Blocking principle unchanged.** Only Blockers block; unclassified blocks only
  at `error`/`fatal` (default failure rules). The cosmetic long tail
  (Style/Layout) is weight-0 and non-blocking, so widening does **not** turn
  cookbooks red or inflate scores.
- **Addon cops** are operator-supplied `.rb` RuboCop cop files on the app host,
  referenced by config and `require:`d into the scan. Trust boundary = deploying
  the app; **not** web-uploaded.
- **Autocorrect preview covers the full ruleset too — nothing is skipped.** Kept
  as an explanation aid (a before→after helps non-Ruby/Chef users understand a
  cop). Visibility is a UI concern, not an engine one: the per-cop Before/After
  (from a cop's remediation mapping) renders inside the classification-bucketed
  sections; the whole-cookbook unified diff stays one collapsible card. Dropping
  `--only` here also lets **addon-cop** fixes appear (they have no embedded
  remediation mapping, so the diff is their only preview).
- **Noise seeding.** Curated defaults classify the cosmetic departments
  (`Chef/Style`, cosmetic `Chef/Modernize`, generic `Style/`, `Layout/`) as Noise
  so the new flood pre-sorts into the collapsed Noise section.
- **No new UI.** The existing classification-grouped collapsible sections
  (`ClassificationSections.tsx`) and autocorrect preview card render everything.

## Invariants

- Verdict + complexity derive from `(offences + resolved classification)`, never
  from the department filter. SoT: `DeriveCookstyleStatus`,
  `ComputeCookstyleComplexity`.
- A cop outside Deprecations/Correctness/Modernize at non-error severity
  contributes complexity weight 0 (`complexity_classification.go`
  `unclassifiedWeight`) and does not block (default rules `*: {error,fatal}`,
  `failure_rules.go`).
- Addon-cop offences are ordinary offences keyed by `cop_name`; they flow through
  classification resolution, the rollup, complexity, propagation, and fingerprint
  history with no special-casing.
- A broken/invalid addon cop file (load/syntax error) MUST NOT fail every scan:
  addon-cop loading is isolated and a load failure is surfaced (admin/log), not
  recorded as a cookbook compatibility error.
- Detection, complexity, and the autocorrect preview skip no cop at the engine
  level. What a user sees is governed by the UI classification buckets/filter, not
  by excluding cops from the scan or autocorrect run.
- Reclassifying a cop stays rescan-free (existing propagation/recompute closure).

## Configuration

Addon cops are declared under `analysis_tools` (live, dynamic config). The
authoritative shape lives in the Go config type (`internal/config`) + the admin
config API; reference data only:

```yaml
analysis_tools:
  cookstyle_addon_cop_paths:
    - "/var/lib/chef-migration-metrics/addon-cops/*.rb"
```

Entries are files, directories (expanded to `*.rb`), or globs on the app host.
With `--only` dropped, a `require:`d cop runs without any department declaration —
so the config is just the paths. (A cop RuboCop reports as `pending` may still need
enabling; handled as an in-chunk detail, not config surface.)

## Scan & Autocorrect Invocation

- The scan sidecar `.rubocop_cmm.yml` (`writeCookstyleTargetConfig`) gains a
  `require:` entry per resolved addon `.rb`, alongside cookstyle; the `--only`
  narrowing is removed for the scan. `AllCops.TargetChefVersion` is unchanged.
- The autocorrect preview (`internal/remediation/autocorrect.go`) shares that one
  sidecar/args helper (today the two builders are duplicated) and likewise drops
  `--only`, so its whole-cookbook diff covers every available fix, including addon
  cops. No cop is excluded at the engine level.

## Operator Workflow

Scan surfaces every cop → the detail view's **Blockers** section (expanded) is the
must-fix list; Noise/Unclassified start collapsed. Operators bulk-classify the
unclassified tail via the Cop Analysis / Cop Classifications admin surfaces;
reclassification propagates without a rescan. Over time the Blockers section stays
clean and the tail shrinks.

## Performance

The full ruleset is slower per scan than two departments, bounded by the existing
per-scan timeout — acceptable for the migration-planning cadence. Addon cops add
per-file AST work proportional to cop count.

## Related

- [cop-classification](cop-classification.md) — classification levels, resolution,
  rollup status, complexity weighting, custom (pattern) cops.
- [analysis-cookstyle](analysis-cookstyle.md) — CookStyle invocation / parsing.
- [cookstyle-failure-rules](cookstyle-failure-rules.md) — severity fallback for
  unclassified cops.
- [dual-compatibility-signals](dual-compatibility-signals.md) — CS vs TK
  separation (unaffected).
