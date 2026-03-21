-- ---------------------------------------------------------------------------
-- 0005_drop_cookbook_role_usage
-- ---------------------------------------------------------------------------
-- Drop the cookbook_role_usage table. This table was defined in the initial
-- schema but never wired up — no Go code reads or writes to it. Role-to-
-- cookbook relationships are captured by the role_dependencies table instead.
-- ---------------------------------------------------------------------------

DROP TABLE IF EXISTS cookbook_role_usage;
