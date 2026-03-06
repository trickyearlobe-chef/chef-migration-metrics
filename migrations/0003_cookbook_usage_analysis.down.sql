-- =============================================================================
-- Migration 0003: Cookbook Usage Analysis (DOWN)
-- =============================================================================
-- Drops the cookbook usage analysis tables created in the up migration.
-- =============================================================================

DROP TABLE IF EXISTS cookbook_usage_detail;
DROP TABLE IF EXISTS cookbook_usage_analysis;
