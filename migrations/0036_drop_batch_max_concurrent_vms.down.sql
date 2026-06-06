-- Migration 0036 (down): Restore the per-batch max_concurrent_vms column.

ALTER TABLE kitchen_batches
    ADD COLUMN IF NOT EXISTS max_concurrent_vms INTEGER;
