-- SPDX-License-Identifier: Apache-2.0

-- kitchen_batch_instances: tracks individual work items within a batch run.
-- Created before execution starts so progress can be calculated as
-- total - completed.
CREATE TABLE kitchen_batch_instances (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id            UUID NOT NULL REFERENCES kitchen_batches(id) ON DELETE CASCADE,
    git_repo_name       TEXT NOT NULL,
    git_repo_url        TEXT NOT NULL,
    instance_name       TEXT NOT NULL,
    platform_name       TEXT NOT NULL,
    suite_name          TEXT NOT NULL,
    target_chef_version TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    error_message       TEXT,
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_kbi_status CHECK (status IN ('pending', 'running', 'passed', 'failed', 'errored', 'timed_out', 'network_timeout', 'cancelled'))
);

CREATE INDEX idx_kbi_batch_id ON kitchen_batch_instances (batch_id);
CREATE INDEX idx_kbi_batch_status ON kitchen_batch_instances (batch_id, status);
