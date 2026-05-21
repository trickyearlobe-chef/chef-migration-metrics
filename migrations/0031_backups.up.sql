-- Backup metadata cache/index. Filesystem (sidecar manifests) is authoritative.
CREATE TABLE IF NOT EXISTS backups (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename        TEXT NOT NULL,
    size_bytes      BIGINT,
    sha256          TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    error           TEXT,
    app_version     TEXT,
    schema_version  INTEGER,
    pg_server_version TEXT,
    pg_dump_version TEXT,
    initiated_by    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_backups_status ON backups (status);
CREATE INDEX idx_backups_created_at ON backups (created_at DESC);
