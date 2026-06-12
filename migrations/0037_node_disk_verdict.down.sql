ALTER TABLE node_snapshots DROP COLUMN IF EXISTS sufficient_disk_space;
ALTER TABLE node_snapshots DROP COLUMN IF EXISTS available_disk_mb;
ALTER TABLE node_snapshots DROP COLUMN IF EXISTS required_disk_mb;
