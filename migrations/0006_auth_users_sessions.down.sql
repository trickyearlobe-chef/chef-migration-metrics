-- =============================================================================
-- Migration 0006: Authentication — Users and Sessions (DOWN)
-- =============================================================================
-- Reverses migration 0006 by dropping the sessions and users tables.
-- Sessions must be dropped first due to the foreign key constraint.
-- =============================================================================

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
