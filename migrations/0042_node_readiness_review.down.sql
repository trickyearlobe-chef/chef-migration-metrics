-- SPDX-License-Identifier: Apache-2.0
-- Migration 0042 down: drop the node readiness rollup status + review cookbooks.

DROP INDEX IF EXISTS idx_node_readiness_status;

ALTER TABLE node_readiness
    DROP CONSTRAINT IF EXISTS chk_node_readiness_status;
ALTER TABLE node_readiness
    DROP COLUMN IF EXISTS status;

ALTER TABLE node_readiness
    DROP COLUMN IF EXISTS review_cookbooks;
