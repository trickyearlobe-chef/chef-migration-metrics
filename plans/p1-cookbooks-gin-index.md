# P1 — GIN index on node_snapshots.cookbooks (coverage full-scan hotspot)

## Problem

`GetProductionPlatformsForCookbook` (`cookbook_production_platforms.go`) runs
`WHERE cookbooks::jsonb ? $1` — a JSONB key-existence scan that seq-scans all
~119k `node_snapshots` rows per call (customer P1 hotspot; also the EXPLAIN
catalog's "Cookbook coverage containment"). No index on `cookbooks`.

## Design (verified against code, not the original plan note)

- **Index:** `CREATE INDEX idx_node_snapshots_cookbooks_gin ON node_snapshots USING GIN (cookbooks);`
- **Opclass = default `jsonb_ops`, NOT `jsonb_path_ops`.** `?` is only indexable by
  `jsonb_ops`; `jsonb_path_ops` supports only `@>`/`@?`/`@@`. (The `roles` GIN uses
  `jsonb_path_ops` because roles is queried with `@>` — different operator.)
- **Plain column index** — `cookbooks` is already `JSONB` (0001:136); the `::jsonb`
  cast is a no-op, so no expression index is needed.
- **Drop the redundant `::jsonb` cast** from `productionPlatformsForCookbookQuery`
  so `WHERE cookbooks ? $1` matches the index unambiguously. LSP: the const is
  referenced only by the query itself + `query_explain.go:148` (EXPLAIN catalog) —
  both stay in sync. `GetProductionPlatformsForCookbook` signature unchanged
  (caller `coverage.go:96` unaffected).
- **Plain `CREATE INDEX` (no CONCURRENTLY)** — migrations are transaction-wrapped
  (`applyMigrationFS`), so CONCURRENTLY is impossible. One-time SHARE lock on
  `node_snapshots` while the index builds at deploy (seconds-to-~1min at customer
  scale); thereafter normal per-INSERT GIN maintenance, no table lock.

## TDD

- **Red:** functional test — seed `node_snapshots`, assert (a) query correctness
  unchanged, and (b) `SET LOCAL enable_seqscan=off; EXPLAIN <query>` uses
  `idx_node_snapshots_cookbooks_gin` (Bitmap Index Scan). (b) catches the opclass
  choice: a `jsonb_path_ops` index would not satisfy `?` even with seqscan off.
- **Green:** add `0050_cookbooks_gin_index.{up,down}.sql`; drop the cast.
- Run functional suite (CMM_TEST_DATABASE_URL) after each change.

## Acceptance

- Migration 0050 applies + rolls back cleanly.
- Correctness invariant holds (existing coverage/production-platform tests pass).
- Plan uses the GIN index for `cookbooks ? $1` (enable_seqscan=off test).
