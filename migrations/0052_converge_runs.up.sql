-- Event ingest: per-node converge run telemetry (see specifications/run-history.md).
--
-- Append-only, time-partitioned, short-retention. Populated by the passive
-- POST /api/v1/ingest sink from three producer shapes (node-direct run_converge,
-- Chef Server proxy relay, Chef Automate Data Feed), all normalised to one row.
--
-- DECOUPLED BY DESIGN: no FKs to organisations / node_snapshots. A run for a node
-- CMM does not pull is still valid telemetry and MUST persist. `organisation` holds
-- the org *name* as delivered by the producer (Chef's organization/organization_name),
-- joined to node_snapshots at read time via organisations.name — NOT an FK. This table
-- NEVER writes node primary associations (used/unused, blast radius); observed cookbooks
-- here are per-run facts only.
--
-- PARTITIONING: range on end_time; retention is by dropping whole day partitions (see
-- converge_runs_ensure_partition below + the store's scheduled purge). Postgres requires
-- the partition key in every unique/primary key, so the dedup key is (run_id, end_time)
-- rather than run_id alone — a run's end_time is fixed, so this dedups the same run even
-- when it arrives twice (e.g. Server proxy AND Automate). end_time is NOT NULL because a
-- NULL partition-key row cannot route; the normaliser skips any converge lacking end_time.
CREATE TABLE converge_runs (
    run_id                 TEXT        NOT NULL,
    organisation           TEXT        NOT NULL,
    node_name              TEXT        NOT NULL,
    source_fqdn            TEXT,
    chef_server_fqdn       TEXT,
    status                 TEXT        NOT NULL,
    chef_version           TEXT,
    start_time             TIMESTAMPTZ,
    end_time               TIMESTAMPTZ NOT NULL,
    run_list               JSONB       NOT NULL DEFAULT '[]',
    expanded_run_list      JSONB,
    cookbooks              JSONB       NOT NULL DEFAULT '{}',
    total_resource_count   INTEGER,
    updated_resource_count INTEGER,
    error                  JSONB,        -- on failure: {class, message, description, backtrace[]} (backtrace bounded)
    failed_resource        JSONB,        -- on failure: {cookbook_name, recipe_name, name, type}
    shape                  TEXT        NOT NULL,   -- provenance: 'datafeed' | 'run_converge'
    received_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT pk_converge_runs PRIMARY KEY (run_id, end_time)
) PARTITION BY RANGE (end_time);

-- Read path: recent runs for a node, most-recent first.
CREATE INDEX idx_converge_runs_org_node_time
    ON converge_runs (organisation, node_name, end_time DESC);

-- Create the day partition covering p_day [00:00Z, next-day 00:00Z) if absent.
-- Called by the store before an insert (for the row's UTC end_time date) and by
-- the retention job's roll-forward. Bounds are pinned to UTC so partition edges are
-- deterministic regardless of session TimeZone. Idempotent and race-safe.
CREATE OR REPLACE FUNCTION converge_runs_ensure_partition(p_day date)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    part_name text := format('converge_runs_%s', to_char(p_day, 'YYYYMMDD'));
    lo        text := to_char(p_day,     'YYYY-MM-DD') || ' 00:00:00+00';
    hi        text := to_char(p_day + 1, 'YYYY-MM-DD') || ' 00:00:00+00';
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF converge_runs FOR VALUES FROM (%L) TO (%L)',
        part_name, lo, hi
    );
EXCEPTION
    WHEN duplicate_table THEN
        NULL;  -- a concurrent creator won the race; the partition now exists.
END;
$$;
