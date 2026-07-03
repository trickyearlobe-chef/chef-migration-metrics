-- SPDX-License-Identifier: Apache-2.0
-- Migration 0042: Node readiness 3-state rollup + needs-review cookbooks.
--
-- Chunk 6 makes node readiness consume the CookStyle rollup status and adds the
-- review_blocks_readiness toggle. The node rollup now mirrors the CookStyle
-- vocabulary (ready / needs_review / blocked). The boolean is_ready stays as the
-- back-compat convenience (= status == 'ready'); with the toggle off no node is
-- needs_review, so the ready set is unchanged.
--
-- review_cookbooks holds the cookbooks at needs_review (same JSON shape as
-- blocking_cookbooks); it is populated only when the toggle is on.

ALTER TABLE node_readiness
    ADD COLUMN status TEXT NOT NULL DEFAULT 'blocked';

ALTER TABLE node_readiness
    ADD CONSTRAINT chk_node_readiness_status
        CHECK (status IN ('ready', 'needs_review', 'blocked'));

ALTER TABLE node_readiness
    ADD COLUMN review_cookbooks JSONB;

-- Backfill the rollup from the existing is_ready boolean: ready when is_ready,
-- otherwise blocked. needs_review is impossible to recover from is_ready alone
-- (it requires the cookbook verdicts under the toggle), so existing rows
-- self-heal to the precise value on the next readiness evaluation.
UPDATE node_readiness
    SET status = CASE WHEN is_ready THEN 'ready' ELSE 'blocked' END;

CREATE INDEX idx_node_readiness_status ON node_readiness (status);
