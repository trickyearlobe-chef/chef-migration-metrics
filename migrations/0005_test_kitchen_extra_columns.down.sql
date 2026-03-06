-- =============================================================================
-- Migration 0005 (down): Revert Test Kitchen Extra Columns
-- =============================================================================
-- Removes the per-phase output, timed_out, and driver/platform tracking
-- columns added by the up migration.
-- =============================================================================

ALTER TABLE test_kitchen_results
    DROP COLUMN IF EXISTS converge_output,
    DROP COLUMN IF EXISTS verify_output,
    DROP COLUMN IF EXISTS destroy_output;

ALTER TABLE test_kitchen_results
    DROP COLUMN IF EXISTS timed_out;

ALTER TABLE test_kitchen_results
    DROP COLUMN IF EXISTS driver_used,
    DROP COLUMN IF EXISTS platform_tested,
    DROP COLUMN IF EXISTS overrides_applied;
