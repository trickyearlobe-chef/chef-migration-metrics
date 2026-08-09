# Fix B — poly-method cop: per-message remediation + classification

Branch `fix/cookstyle-polymethod-cop`. Scope = **Live derivations** (approved):
message-aware at every live grain (remediation guidance, detail badge/grouping,
materialised rollup status + weighted complexity). Trend-recompute-from-stored-
fingerprint stays cop-name-keyed (documented residual). No fingerprint schema change.

## Problem

`Lint/DeprecatedClassMethods` is one cop flagging several unrelated deprecations.
Message discriminates: `File.exists?`/`Dir.exists?` were **removed** (Ruby 3.2) →
Blocker + `File.exist?` guidance; `Socket.gethostbyname`/`Socket.gethostbyaddr`
are **deprecation-only** → Review + `Addrinfo` guidance. Remediation and
classification both key on cop NAME only, so every offence in the group inherits
one guidance block + one false-positive Blocker verdict.

## Variant reference data (curation-linter guarded)

Poly cop `Lint/DeprecatedClassMethods`, matched by deprecated-method token in the
offence message:

| Token | RemovedIn | Class | Guidance |
|-------|-----------|-------|----------|
| `File.exists?` | 18.0 | Blocker | `File.exist?` |
| `Dir.exists?` | 18.0 | Blocker | `Dir.exist?` |
| `Socket.gethostbyname` | (none) | Review | `Addrinfo.getaddrinfo` |
| `Socket.gethostbyaddr` | (none) | Review | `Addrinfo.getaddrinfo` |

`File.exists?`/`Dir.exists?` keep the existing `RemovedIn: 18.0` to preserve
current behaviour + tests (KB notes actual removal is Ruby 3.4/Chef 19 — separate
curation item, out of scope; record in tech-debt).

## Design — message-aware lookup, cop-name fallback

- **Mapping (`remediation`)**: `LookupCopForOffense(copName, message) *CopMapping`.
  For poly cops → variant selected by token substring; returns a `CopMapping` with
  the variant's Description/RemovedIn/ReplacementPattern. Non-poly, or no token
  match → falls back to `LookupCop(copName)`. `LookupCop` unchanged.
- **Classifier (`analysis`)**: `ResolveOffense(copName, message)` — identical to
  `Resolve` except step 3 (verified removal) consults `LookupCopForOffense`.
  `Resolve(copName)` = `ResolveOffense(copName, "")` (unchanged for non-poly / no
  message). Add `ClassifyOffense(copName, message)`.
- **`CopClassifier` interface**: add `ClassifyOffense(copName, message) string`;
  keep `Classify(copName)` for the fingerprint-recompute (message-free) path.

## Change sites

1. `remediation/copmapping.go` — variant data + `LookupCopForOffense`. Poly-cop
   variant table (separate from `embeddedCopMappings`; the base entry stays for
   cop-name fallback / cop-analysis view).
2. `analysis/cop_classification.go` — `ResolveOffense` / `ClassifyOffense`;
   `Resolve`/`Classify` delegate with "".
3. `analysis/cookstyle_status.go` — `DeriveCookstyleStatus` resolves each offence
   via `ResolveOffense(CopName, Message)` (stops projecting through the message-
   free fingerprint entry for the LIVE path). `DeriveStatusFromFingerprint`
   unchanged (trend residual — document the intentional poly-cop divergence).
4. `remediation/complexity_classification.go` — `storedOffense` gains `Message`;
   `classifyOffensesForComplexity` uses `ClassifyOffense`. `ComplexityFromFingerprint`
   keeps `Classify` (residual).
5. `analysis/cookstyle.go` `enrichOffenses` — `LookupCopForOffense(CopName, Message)`
   per offence (persisted per-offence remediation now correct).
6. `webapi/handle_cookbook_remediation.go` + `handle_git_repo_remediation.go` —
   group by **effective key** (cop + variant), not cop name. Each group resolves
   classification + remediation via the `*Offense` message-aware calls. Add
   `group_key` to the group JSON; `cop_name` stays the plain name for display.
7. Frontend: `CookbookRemediationPage.tsx` + `GitRepoRemediationPage.tsx` +
   `types/remediation.ts` — use `group.group_key ?? group.cop_name` for the React
   key + collapse-state identity (4 spots each). Header still shows `cop_name`.

## Residuals (documented, accepted)

- Trend recompute from stored fingerprint (`DeriveStatusFromFingerprint`,
  `ComplexityFromFingerprint`) stays cop-name-keyed — a poly cop's non-removed
  variant may over-blocker in *recomputed historical* trend points only. Live
  verdict is authoritative.
- Cop Analysis aggregation (`handle_cookstyle_cops.go`) stays cop-name grain —
  the poly cop shows as one Blocker row. Note as residual (tech-debt), not fixed
  here (not the customer-reported surface; splitting the aggregation grain is a
  bigger change).

## Spec delta (needs approval before applying)

`journeys/cop-classification.md`:
- Classification Resolution step 3: note verified-removal keys on the offence
  **message** for the curated poly-method cop set, cop-name otherwise.
- New short subsection "Poly-method cops" under Blocker & Noise Reference Data:
  one cop_name → several message-discriminated deprecations; variant table is the
  SoT; message-aware resolution is LIVE-only (fingerprint recompute residual).
- Single-source-of-truth invariant: add the poly-cop live-vs-fingerprint
  exception explicitly.

## Acceptance

- `Socket.gethostbyname` offence → Addrinfo guidance, Review, NOT counted in
  Blocked status / blocker complexity weight.
- `File.exists?`/`Dir.exists?` offence → unchanged (Blocker + File.exist? guidance).
- A cookbook whose only `Lint/DeprecatedClassMethods` offence is `Socket.*`
  materialises `needs_review`, not `blocked` (list/summary/detail agree).
- Detail view: the two variants render as separate groups under Blockers / Review.
- TDD; `make ci` green; no fingerprint schema change.
