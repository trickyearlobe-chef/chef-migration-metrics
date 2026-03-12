-- 0009_ownership_tracking.down.sql
-- Reverts ownership tracking tables.

ALTER TABLE node_snapshots DROP COLUMN IF EXISTS custom_attributes;

DROP TABLE IF EXISTS ownership_audit_log;
DROP TABLE IF EXISTS git_repo_committers;
DROP TABLE IF EXISTS ownership_assignments;
DROP TABLE IF EXISTS owners;
