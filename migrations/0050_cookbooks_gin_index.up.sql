-- 0050_cookbooks_gin_index.up.sql
--
-- P1 coverage full-scan hotspot: GetProductionPlatformsForCookbook runs
-- `WHERE cookbooks ? $1` (JSONB key-existence) which, with no index on
-- node_snapshots.cookbooks, sequentially scans one row per node in the fleet
-- per call.
--
-- Fix: a GIN index using the DEFAULT jsonb_ops opclass. jsonb_ops indexes the
-- `?`, `?|`, `?&`, `@>`, `@?`, `@@` operators. jsonb_path_ops is deliberately
-- NOT used here: it only indexes `@>`/`@?`/`@@` and cannot serve the `?`
-- key-existence operator this query relies on. (The roles GIN index uses
-- jsonb_path_ops because roles is queried with `@>`.)
--
-- Built with a plain CREATE INDEX (not CONCURRENTLY) because migrations run
-- inside a transaction; this takes a one-time SHARE lock on node_snapshots at
-- deploy time while the index builds.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_cookbooks_gin
    ON node_snapshots USING GIN (cookbooks);
