-- SPDX-License-Identifier: Apache-2.0
-- Revert migration 0054.
--
-- WARNING: restoring blocking_cookbooks to the INCLUDE list reintroduces the
-- btree row-size failure — upserts are rejected for any node whose
-- blocking_cookbooks JSONB pushes the index tuple past 2704 bytes.

DROP INDEX IF EXISTS idx_node_readiness_target_name_eval;

CREATE INDEX idx_node_readiness_target_name_eval
    ON node_readiness (target_chef_version, node_name, evaluated_at DESC)
    INCLUDE (is_ready, stale_data, blocking_cookbooks);
