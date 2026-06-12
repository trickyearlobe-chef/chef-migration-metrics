-- Store the version-invariant disk-space verdict on the node snapshot itself.
-- The verdict depends only on the node's filesystem + platform install size (not
-- the target Chef version), so it is computed once at collection time and stored
-- here, decoupled from the per-target node_readiness rows. All nullable;
-- sufficient_disk_space NULL means indeterminate (no usable filesystem data).

ALTER TABLE node_snapshots ADD COLUMN sufficient_disk_space BOOLEAN;
ALTER TABLE node_snapshots ADD COLUMN available_disk_mb INTEGER;
ALTER TABLE node_snapshots ADD COLUMN required_disk_mb INTEGER;
