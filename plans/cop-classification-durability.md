# Plan — Cop Classification Durability

Spec: `specifications/cop-classification.md` → "Data Provenance & Durability".
Goal: stop the classification tables silently rotting as cookstyle evolves.
Branch: continue on `feature/cookstyle-violations-browser` (unshipped, so #3's
data-model change carries no back-compat cost).

Ordered; each chunk = one session. #1 and #2 are the high-leverage, lower-risk
pair; #3 converts "recompile" into "data edit".

## Chunk 1 — Department-default classification (unknowns are never invisible)

Scope: `internal/analysis/cop_classification_defaults.go` (+ resolver test).
Steps (TDD):
- Add prefix/department defaults: `Chef/Deprecations/` and `Chef/Correctness/`
  → **Review** (below exact defaults + RemovedIn, above the cosmetic-Noise
  prefixes and Unclassified — longest-prefix-wins already implemented).
- Prove: a brand-new *unmapped* `Chef/Deprecations/Foo` resolves to Review, not
  Unclassified; existing curated exacts + `RemovedIn ≤ target` still win as
  Blocker; `Chef/Style/*` still Noise.
Acceptance: unknown Chef deprecation/correctness cops surface as Review (visible,
non-blocking); no cookbook turns red (only Blocker blocks); curated exacts and
RemovedIn unchanged. Reclassification stays rescan-free (recompute closure).
Note: measure complexity-weight impact — many previously-Unclassified cops become
Review; Review must not inflate status, only advisory complexity.
Depends on: none.

## Chunk 2 — Live cop inventory + drift/coverage report

Scope: new registry provider (`internal/analysis` or `internal/remediation`),
`cmd/.../main.go` wiring, a drift endpoint under `internal/webapi`, a small admin
panel. Union `Chef/*` registry cops into the cops list universe.
Steps (TDD):
- Parser: `cookstyle --show-cops` output (fixture) → cop names + department
  (+ Enabled/Severity/Description). Column-0 `Dept/Name:` lines.
- Registry provider: run once, cache (keyed by cookstyle path/version), fallback
  to the static universe on failure (non-fatal).
- Drift computation: registry vs static tables → **stale** (mapping/curated entry
  for a cop the binary no longer emits) + **coverage gaps** (`Chef/*` cops with no
  classification).
- Wire `Chef/*` registry cops into `handleCookstyleCops`' known-cop universe
  (generic-Ruby cops excluded from the default view; still auto-added on trigger).
- Admin panel surfacing the two lists.
Acceptance: unclassified `Chef/*` cops are listable pre-scan; drift report shows
stale + gaps; `--show-cops` failure degrades to today's static universe; generic
Style/Layout/Lint stay out of the default list.
Depends on: none (complements the shipped cops-list universe fix).

## Chunk 3 — Seed static defaults into the DB (code edit → data edit)

Scope: migration + datastore + resolver. Since the branch is unshipped there is no
migration back-compat to preserve — seed cleanly.
Steps (TDD):
- Decide model: seed the curated defaults (classification + min-target) and the
  `RemovedIn`/description/URL mapping into editable DB rows. Reuse
  `cop_classifications` with a `system` provenance vs a dedicated `cop_defaults`
  table — pick in-chunk; keep operator overrides distinct so they still win.
- Migration seeds the 89 from the existing Go tables (the Go tables become the
  seed source only).
- Resolver consults DB defaults (priority: operator override > DB RemovedIn > DB
  curated > prefix defaults > unclassified); compiled tables remain as the seed +
  a last-resort fallback if the DB is empty.
- Admin edit path for a default (reuse the override endpoint or add an edit).
Acceptance: editing a default at runtime changes resolution with no recompile;
fresh install seeds the 89; operator overrides still take precedence; prefix
defaults (chunk 1) unaffected.
Depends on: 1 (department defaults are part of the seeded set).

## Cross-cutting

- `go test ./...` + frontend test/lint green per chunk.
- Reclassification/propagation stays rescan-free throughout.
- RemovedIn (Chef removal version) remains curated by design — no machine source
  supplies it; chunks 1–2 ensure its staleness is *visible* and unknowns are
  *safely defaulted*, not silent.
