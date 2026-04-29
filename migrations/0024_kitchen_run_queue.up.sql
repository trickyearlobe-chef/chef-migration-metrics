-- Kitchen run queue: DB-backed queue for all test kitchen execution
CREATE TABLE kitchen_run_queue (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_type            TEXT NOT NULL CHECK (run_type IN ('git', 'node')),
    git_repo_name       TEXT,
    git_repo_url        TEXT,
    suite_name          TEXT,
    platform_name       TEXT,
    instance_name       TEXT,
    target_chef_version TEXT NOT NULL,
    head_commit_sha     TEXT,
    node_name           TEXT,
    organisation_name   TEXT,
    cookbook_source      TEXT,
    batch_id            UUID REFERENCES kitchen_batches(id) ON DELETE SET NULL,
    priority            INT NOT NULL DEFAULT 10,
    status              TEXT NOT NULL DEFAULT 'queued'
                        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'interrupted')),
    enqueued_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    error_message       TEXT,
    output              TEXT,
    retry_of            UUID REFERENCES kitchen_run_queue(id) ON DELETE SET NULL
);

-- Efficient dequeue: claim highest-priority oldest item
CREATE INDEX idx_kitchen_run_queue_dequeue
    ON kitchen_run_queue (priority DESC, enqueued_at ASC)
    WHERE status = 'queued';

-- Dedup for git runs: only one queued/running per instance+commit
CREATE UNIQUE INDEX idx_kitchen_run_queue_dedup_git
    ON kitchen_run_queue (git_repo_name, suite_name, platform_name, target_chef_version, head_commit_sha)
    WHERE status IN ('queued', 'running') AND run_type = 'git';

-- Dedup for node runs: only one queued/running per node+version
CREATE UNIQUE INDEX idx_kitchen_run_queue_dedup_node
    ON kitchen_run_queue (organisation_name, node_name, target_chef_version)
    WHERE status IN ('queued', 'running') AND run_type = 'node';

-- Lookup by repo (for queue panel on repo detail page)
CREATE INDEX idx_kitchen_run_queue_repo
    ON kitchen_run_queue (git_repo_name, status, enqueued_at DESC)
    WHERE run_type = 'git';

-- Lookup by batch
CREATE INDEX idx_kitchen_run_queue_batch
    ON kitchen_run_queue (batch_id)
    WHERE batch_id IS NOT NULL;
