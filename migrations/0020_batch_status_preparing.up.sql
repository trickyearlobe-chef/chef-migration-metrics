-- SPDX-License-Identifier: Apache-2.0

-- Add 'preparing' and 'failed' to kitchen_batches status lifecycle.
-- preparing: batch resolution and planning in progress before execution.
-- failed: batch preparation failed (e.g. no repos matched, analysis missing).
ALTER TABLE kitchen_batches
    DROP CONSTRAINT chk_kb_status,
    ADD CONSTRAINT chk_kb_status CHECK (status IN ('draft', 'preparing', 'previewing', 'running', 'completed', 'cancelled', 'failed'));
