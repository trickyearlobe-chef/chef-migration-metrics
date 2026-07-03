# Plan — CookStyle Reliability (Trustworthy Reds)

Goal: make the CookStyle signal a reliable migration indicator. Reds mean "we
know", Review means "operator decides", Noise means "provably harmless". No
bucket presents a guess as knowledge. Single target (the per-target dimension is
removed — only one target is ever active).

Spec: `specifications/cop-classification.md` — revision required (Phase 1).
Branch: `feature/cookstyle-violations-browser` (continues).

## Model (decision record — 2026-07-03)

- **Blocker** — verified removal knowledge, OR operator-confirmed, OR a
  custom/manual cop (blocker by intent; resolve directly, not via severity).
- **Review** — migration-relevant but unproven (`Chef/Deprecations`,
  `Chef/Correctness`, `Lint`, anything unresolved). A worklist the operator
  triages into Blocker-or-clear. Absorbs the old "Unclassified".
- **Noise** — only from a positive structural reason (cosmetic RuboCop dept:
  `Style/`, `Layout/`; or test/CI-tooling-only). Never a fallback.
- **Removed** — the `error`-severity → Blocked fallback (severity is the signal
  this feature exists to distrust); the per-target dimension; chunk-3 DB defaults.
- `fatal`/parse-failure → separate "won't parse — fix first" flag, not a
  classification Blocker.
- **Added** — CI linter guarding curated removal data (validate curated
  `RemovedIn` against cop descriptions + flag cops the binary no longer has).

## Phase 0 — Reset base
- Rewind chunk 3 (`git reset --hard 729e267`; reflog-recoverable).
- Delete superseded flat-list violations code (dead under the Cop Analysis view).
Acceptance: build + tests green; no `cop_defaults` table/endpoint.

## Phase 1 — Spec revision (approval-gated)
- Rewrite `cop-classification.md`: levels/resolution, blocker sources,
  review-as-worklist, structural noise, single target, drop severity reds +
  parse-failure flag, linter, remove the DB-seed durability principle.
- **Open question to settle in the spec:** curated `RemovedIn` entries the linter
  can't confirm from the cop description — demote to Review (fully trustworthy
  reds) or keep Blocker on curator authority? Soundness vs coverage.
Acceptance: user approves the revised spec.

## Phase 2 — Single target
- config: `TargetChefVersions []string` → `target_chef_version` scalar; drop
  `HighestVersion`.
- `cop_classifications`: drop `target_chef_version` from the key (migration);
  resolver drops the target param.
- Collapse per-target loops (propagation, rescore).
Acceptance: one target end-to-end; tests green.

## Phase 3 — Trustworthy-reds resolver
- Priority: operator override > verified-removal (Blocker) > custom-cop (Blocker)
  > structural Noise > Review (default). No severity fallback.
- Custom cops resolve Blocker directly (drop the severity side-channel).
- "won't parse" (fatal/parse-failure) signal, separate from classification.
Acceptance: unit tests per source; no severity-derived reds anywhere.

## Phase 4 — Curation linter
- CI test: each curated `RemovedIn` agrees with the shipped cop description;
  flag stale entries (cop no longer in the binary).
Acceptance: linter fails on injected drift; passes on the corrected table.

## Phase 5 — UI provenance
- Show classification source honestly (verified removal / operator / custom /
  department-default-review); Review presented as a worklist.
Acceptance: frontend tests + lint green.
```
