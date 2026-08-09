# Plan — Cop Classification Durability

Spec: `journeys/cop-classification.md` → "Data Provenance & Durability".
Goal: stop the classification tables silently rotting as cookstyle evolves.
Branch: continue on `feature/cookstyle-violations-browser` (unshipped, so #3's
data-model change carries no back-compat cost).

Ordered; each chunk = one session. #3 converts "recompile" into "data edit".
(#1 department-default classification and #2 live inventory + drift report are
implemented — see `cop_registry.go`, `cop_drift.go`, `handle_cookstyle_drift.go`,
and the `AdminCopInventorySection` panel.)

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
