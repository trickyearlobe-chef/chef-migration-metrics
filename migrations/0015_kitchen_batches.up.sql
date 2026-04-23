-- SPDX-License-Identifier: Apache-2.0

-- kitchen_batches: batch definitions for controlled bulk Git Kitchen runs.
CREATE TABLE kitchen_batches (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    filters             JSONB NOT NULL DEFAULT '{}',
    max_count           INTEGER,
    max_concurrent_vms  INTEGER,
    dry_run             BOOLEAN NOT NULL DEFAULT false,
    status              TEXT NOT NULL DEFAULT 'draft',
    created_by          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,

    CONSTRAINT chk_kb_status CHECK (status IN ('draft', 'previewing', 'running', 'completed', 'cancelled'))
);

CREATE INDEX idx_kitchen_batches_status ON kitchen_batches (status);
CREATE INDEX idx_kitchen_batches_created_at ON kitchen_batches (created_at DESC);

-- Persistent cookbook exclusions on git_repos.
ALTER TABLE git_repos
    ADD COLUMN kitchen_excluded    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN kitchen_exclude_reason TEXT,
    ADD COLUMN kitchen_excluded_by TEXT,
    ADD COLUMN kitchen_excluded_at TIMESTAMPTZ;

CREATE INDEX idx_git_repos_kitchen_excluded ON git_repos (kitchen_excluded) WHERE kitchen_excluded = true;
