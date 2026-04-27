-- SPDX-License-Identifier: Apache-2.0

-- Drop the old per-cookbook result table (broken, two conflicting code paths).
DROP TABLE IF EXISTS git_repo_test_kitchen_results;

-- Reshape git_kitchen_results: drop batch/vm FKs, consolidate output columns,
-- add instance_name and passed columns, remove split outputs.
ALTER TABLE git_kitchen_results
    DROP COLUMN IF EXISTS batch_id,
    DROP COLUMN IF EXISTS vm_tracking_id,
    DROP COLUMN IF EXISTS template_used,
    DROP COLUMN IF EXISTS converge_passed,
    DROP COLUMN IF EXISTS tests_passed,
    DROP COLUMN IF EXISTS converge_output,
    DROP COLUMN IF EXISTS verify_output,
    DROP COLUMN IF EXISTS destroy_output;

ALTER TABLE git_kitchen_results
    ADD COLUMN IF NOT EXISTS instance_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS passed BOOLEAN,
    ADD COLUMN IF NOT EXISTS output TEXT;

-- Drop old indexes that reference removed columns.
DROP INDEX IF EXISTS idx_gkr_batch_id;
DROP INDEX IF EXISTS idx_gkr_status;

-- New status index on the single passed column.
CREATE INDEX IF NOT EXISTS idx_gkr_passed ON git_kitchen_results (passed);
