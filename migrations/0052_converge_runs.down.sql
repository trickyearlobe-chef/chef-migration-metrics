DROP FUNCTION IF EXISTS converge_runs_ensure_partition(date);
-- Dropping the partitioned parent drops all child day partitions with it.
DROP TABLE IF EXISTS converge_runs;
