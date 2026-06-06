-- Add parallel deployment tracking columns to node_snapshots.
-- All nullable — graceful when the migration cookbook is not deployed.

ALTER TABLE node_snapshots ADD COLUMN migration_state TEXT;
ALTER TABLE node_snapshots ADD COLUMN active_chef_version TEXT;
ALTER TABLE node_snapshots ADD COLUMN dormant_installed BOOLEAN;
ALTER TABLE node_snapshots ADD COLUMN dormant_chef_version TEXT;
ALTER TABLE node_snapshots ADD COLUMN target_version TEXT;
ALTER TABLE node_snapshots ADD COLUMN target_execution_time TEXT;
ALTER TABLE node_snapshots ADD COLUMN target_converge_status TEXT;
