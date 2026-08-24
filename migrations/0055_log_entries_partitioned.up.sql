-- log_entries: convert to a day-partitioned table so retention is a partition
-- drop rather than a row-level DELETE.
--
-- WHY: expiry was `DELETE FROM log_entries WHERE timestamp < cutoff`. This table
-- grows fast enough that the delete leaves dead tuples faster than autovacuum
-- reclaims them, so the mechanism meant to bound the table was itself a source
-- of bloat. Dropping a whole day partition reclaims the space immediately and
-- leaves nothing to vacuum. Mirrors converge_runs (migration 0052).
--
-- EXISTING ROWS ARE DISCARDED. Postgres cannot convert a table to a partitioned
-- one in place, and the alternatives all cost more than this data is worth:
-- attaching the old table as a historical partition would forbid the primary
-- key below (Postgres requires the partition key in every unique key) or
-- require rebuilding the index over the whole table while holding an exclusive
-- lock. These are the application's own diagnostic logs, not collected estate
-- data, and they expire on a 90-day clock anyway.
--
-- Because the new table starts empty, PRIMARY KEY (id, timestamp) costs nothing
-- to establish and keeps id uniqueness enforced. The partition key must appear
-- in it, hence the composite; `WHERE id = $1` still uses the index by prefix.
DROP TABLE IF EXISTS log_entries;

CREATE TABLE log_entries (
    id                   BIGSERIAL   NOT NULL,
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

    CONSTRAINT pk_log_entries PRIMARY KEY (id, timestamp),
    CONSTRAINT chk_log_entries_severity
        CHECK (severity = ANY (ARRAY['DEBUG'::text, 'INFO'::text, 'WARN'::text, 'ERROR'::text]))
) PARTITION BY RANGE (timestamp);

-- Read path: the log viewer orders by timestamp DESC and filters on these.
-- The pre-partition table carried two identical btree indexes on timestamp
-- (idx_log_entries_retention and idx_log_entries_timestamp), doubling write
-- cost and disk for no benefit; only one is recreated here. Retention no longer
-- needs an index on timestamp at all — it drops partitions.
CREATE INDEX idx_log_entries_timestamp ON log_entries ("timestamp" DESC);
CREATE INDEX idx_log_entries_severity ON log_entries (severity);
CREATE INDEX idx_log_entries_scope ON log_entries (scope);
CREATE INDEX idx_log_entries_organisation ON log_entries (organisation);
CREATE INDEX idx_log_entries_cookbook_name ON log_entries (cookbook_name);
CREATE INDEX idx_log_entries_collection_run_org ON log_entries (collection_run_org);

-- Create the day partition covering p_day [00:00Z, next-day 00:00Z) if absent.
-- Called by the store before an insert, for the row's UTC timestamp date.
-- Bounds are pinned to UTC so partition edges are deterministic regardless of
-- session TimeZone. Idempotent and race-safe.
CREATE OR REPLACE FUNCTION log_entries_ensure_partition(p_day date)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    part_name text := format('log_entries_%s', to_char(p_day, 'YYYYMMDD'));
    lo        text := to_char(p_day,     'YYYY-MM-DD') || ' 00:00:00+00';
    hi        text := to_char(p_day + 1, 'YYYY-MM-DD') || ' 00:00:00+00';
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF log_entries FOR VALUES FROM (%L) TO (%L)',
        part_name, lo, hi
    );
EXCEPTION
    WHEN duplicate_table THEN
        NULL;  -- a concurrent creator won the race; the partition now exists.
END;
$$;

-- Today's partition, so the very first log line after this migration has
-- somewhere to land even if it races the store's ensure call.
SELECT log_entries_ensure_partition((now() AT TIME ZONE 'UTC')::date);
