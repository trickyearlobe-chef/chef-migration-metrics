-- Revert log_entries to a plain (unpartitioned) table.
-- Log rows are not preserved across the revert, matching the up migration.
DROP FUNCTION IF EXISTS log_entries_ensure_partition(date);
-- Dropping the partitioned parent drops all child day partitions with it.
DROP TABLE IF EXISTS log_entries;

CREATE TABLE log_entries (
    id                   BIGSERIAL   PRIMARY KEY,
    timestamp            TIMESTAMPTZ NOT NULL,
    severity             TEXT        NOT NULL,
    scope                TEXT        NOT NULL,
    message              TEXT        NOT NULL,
    organisation         TEXT,
    cookbook_name        TEXT,
    cookbook_version     TEXT,
    commit_sha           TEXT,
    chef_client_version  TEXT,
    process_output       TEXT,
    notification_channel TEXT,
    export_job_id        TEXT,
    tls_domain           TEXT,
    collection_run_org   TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_log_entries_severity
        CHECK (severity = ANY (ARRAY['DEBUG'::text, 'INFO'::text, 'WARN'::text, 'ERROR'::text]))
);

CREATE INDEX idx_log_entries_timestamp ON log_entries ("timestamp");
CREATE INDEX idx_log_entries_retention ON log_entries ("timestamp");
CREATE INDEX idx_log_entries_severity ON log_entries (severity);
CREATE INDEX idx_log_entries_scope ON log_entries (scope);
CREATE INDEX idx_log_entries_organisation ON log_entries (organisation);
CREATE INDEX idx_log_entries_cookbook_name ON log_entries (cookbook_name);
CREATE INDEX idx_log_entries_collection_run_org ON log_entries (collection_run_org);
