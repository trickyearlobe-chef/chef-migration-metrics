-- Add platform_caption column to node_snapshots for OS caption data
-- collected from Ohai (kernel.os_info.caption on Windows, lsb.description on Linux).
ALTER TABLE node_snapshots ADD COLUMN platform_caption TEXT;
