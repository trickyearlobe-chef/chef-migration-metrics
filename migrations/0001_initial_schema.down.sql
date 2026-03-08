-- =============================================================================
-- Migration 0001: Initial Schema — Rollback
-- =============================================================================
-- Drops all tables created by the up migration in reverse dependency order
-- to respect foreign key constraints.
--
-- This migration is managed by golang-migrate/migrate. Do not edit after commit.
-- =============================================================================

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS export_jobs;
DROP TABLE IF EXISTS notification_history;
DROP TABLE IF EXISTS log_entries;
DROP TABLE IF EXISTS metric_snapshots;
DROP TABLE IF EXISTS role_dependencies;
DROP TABLE IF EXISTS node_readiness;
DROP TABLE IF EXISTS cookbook_complexity;
DROP TABLE IF EXISTS autocorrect_previews;
DROP TABLE IF EXISTS cookstyle_results;
DROP TABLE IF EXISTS test_kitchen_results;
DROP TABLE IF EXISTS cookbook_role_usage;
DROP TABLE IF EXISTS cookbook_node_usage;
DROP TABLE IF EXISTS cookbooks;
DROP TABLE IF EXISTS node_snapshots;
DROP TABLE IF EXISTS collection_runs;
DROP TABLE IF EXISTS organisations;
DROP TABLE IF EXISTS credentials;
