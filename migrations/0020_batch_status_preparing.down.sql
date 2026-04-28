-- SPDX-License-Identifier: Apache-2.0

-- Revert to original status constraint (data may need manual cleanup).
ALTER TABLE kitchen_batches
    DROP CONSTRAINT chk_kb_status,
    ADD CONSTRAINT chk_kb_status CHECK (status IN ('draft', 'previewing', 'running', 'completed', 'cancelled'));
