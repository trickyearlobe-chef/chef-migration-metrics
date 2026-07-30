-- SPDX-License-Identifier: Apache-2.0
-- Migration 0054: Remove blocking_cookbooks from the node_readiness covering index.
--
-- idx_node_readiness_target_name_eval carried blocking_cookbooks (JSONB) in its
-- INCLUDE list. Postgres does NOT TOAST columns stored in an index, so the whole
-- index tuple must fit the btree limit of 2704 bytes. Once a node accumulates
-- enough blocking cookbooks the upsert is rejected outright:
--
--   pq: index row size 3360 exceeds btree version 4 maximum 2704
--       for index "idx_node_readiness_target_name_eval" (54000)
--
-- Measured at customer scale: 22,043 rejected writes in six hours, 10,605 nodes
-- left with readiness older than their own snapshot and 89 with none at all.
-- The failure is loud in the log but invisible in the UI, and it self-selects
-- for the nodes with the most blocking cookbooks — precisely the ones a
-- migration assessment cares about.
--
-- The key columns are unchanged, so the index serves the same lookups. Dropping
-- blocking_cookbooks from INCLUDE costs a heap fetch for queries that need it,
-- on a lookup already narrowed to a single (target_chef_version, node_name).
--
-- Plain DROP/CREATE rather than CONCURRENTLY: migrations run inside a
-- transaction. This takes a brief ACCESS EXCLUSIVE lock on node_readiness while
-- the index rebuilds.
--
-- Invariant: no unbounded column (JSONB, unbounded TEXT, arrays) may appear in
-- an index key or INCLUDE list. Size it against the largest real value, not a
-- typical one.

DROP INDEX IF EXISTS idx_node_readiness_target_name_eval;

CREATE INDEX idx_node_readiness_target_name_eval
    ON node_readiness (target_chef_version, node_name, evaluated_at DESC)
    INCLUDE (is_ready, stale_data);
