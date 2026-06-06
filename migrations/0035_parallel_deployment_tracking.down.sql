ALTER TABLE node_snapshots DROP COLUMN IF EXISTS migration_state;
ALTER TABLE node_snapshots DROP COLUMN IF EXISTS active_chef_version;
ALTER TABLE node_snapshots DROP COLUMN IF EXISTS dormant_installed;
ALTER TABLE node_snapshots DROP COLUMN IF EXISTS dormant_chef_version;
ALTER TABLE node_snapshots DROP COLUMN IF EXISTS target_version;
ALTER TABLE node_snapshots DROP COLUMN IF EXISTS target_execution_time;
ALTER TABLE node_snapshots DROP COLUMN IF EXISTS target_converge_status;
