-- Migration 0036: Drop the per-batch max_concurrent_vms column.
-- Concurrency is managed globally (the kitchen queue worker-pool size),
-- not per batch. The column was never wired to anything.

ALTER TABLE kitchen_batches
    DROP COLUMN IF EXISTS max_concurrent_vms;
