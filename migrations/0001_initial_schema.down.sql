-- =============================================================================
-- Migration 0001: Initial Schema (Consolidated) — Rollback
-- =============================================================================
-- Drops all tables created by the up migration in reverse dependency order
-- to respect foreign key constraints.
-- =============================================================================

-- 29. ownership_audit_log (no dependents)
DROP TABLE IF EXISTS ownership_audit_log;

-- 28. git_repo_committers (no dependents)
DROP TABLE IF EXISTS git_repo_committers;

-- 27. ownership_assignments (no dependents)
DROP TABLE IF EXISTS ownership_assignments;

-- 26. owners (referenced by ownership_assignments — already dropped)
DROP TABLE IF EXISTS owners;

-- 25. cookbook_usage_detail (no dependents)
DROP TABLE IF EXISTS cookbook_usage_detail;

-- 24. cookbook_usage_analysis (referenced by cookbook_usage_detail — already dropped)
DROP TABLE IF EXISTS cookbook_usage_analysis;

-- 23. sessions (no dependents)
DROP TABLE IF EXISTS sessions;

-- 22. users (referenced by sessions — already dropped)
DROP TABLE IF EXISTS users;

-- 21. export_jobs (no dependents)
DROP TABLE IF EXISTS export_jobs;

-- 20. notification_history (no dependents)
DROP TABLE IF EXISTS notification_history;

-- 19. log_entries (no dependents)
DROP TABLE IF EXISTS log_entries;

-- 18. metric_snapshots (no dependents)
DROP TABLE IF EXISTS metric_snapshots;

-- 17. role_dependencies (no dependents)
DROP TABLE IF EXISTS role_dependencies;

-- 16. node_readiness (no dependents)
DROP TABLE IF EXISTS node_readiness;

-- 15. git_repo_complexity (no dependents)
DROP TABLE IF EXISTS git_repo_complexity;

-- 14. server_cookbook_complexity (no dependents)
DROP TABLE IF EXISTS server_cookbook_complexity;

-- 13. git_repo_autocorrect_previews (no dependents)
DROP TABLE IF EXISTS git_repo_autocorrect_previews;

-- 12. server_cookbook_autocorrect_previews (no dependents)
DROP TABLE IF EXISTS server_cookbook_autocorrect_previews;

-- 11. git_repo_test_kitchen_results (no dependents)
DROP TABLE IF EXISTS git_repo_test_kitchen_results;

-- 10. git_repo_cookstyle_results (referenced by git_repo_autocorrect_previews — already dropped)
DROP TABLE IF EXISTS git_repo_cookstyle_results;

-- 9. server_cookbook_cookstyle_results (referenced by server_cookbook_autocorrect_previews — already dropped)
DROP TABLE IF EXISTS server_cookbook_cookstyle_results;

-- 8. cookbook_role_usage (no dependents)
DROP TABLE IF EXISTS cookbook_role_usage;

-- 7. cookbook_node_usage (no dependents)
DROP TABLE IF EXISTS cookbook_node_usage;

-- 6. git_repos (referenced by git_repo_cookstyle_results, git_repo_test_kitchen_results, git_repo_autocorrect_previews, git_repo_complexity — all already dropped)
DROP TABLE IF EXISTS git_repos;

-- 5. server_cookbooks (referenced by server_cookbook_cookstyle_results, server_cookbook_autocorrect_previews, server_cookbook_complexity, cookbook_node_usage — all already dropped)
DROP TABLE IF EXISTS server_cookbooks;

-- 4. node_snapshots (referenced by cookbook_node_usage, node_readiness — all already dropped)
DROP TABLE IF EXISTS node_snapshots;

-- 3. collection_runs (referenced by node_snapshots, cookbook_usage_analysis, metric_snapshots, log_entries — all already dropped)
DROP TABLE IF EXISTS collection_runs;

-- 2. organisations (referenced by server_cookbooks, collection_runs, node_snapshots, cookbook_usage_analysis, cookbook_usage_detail, cookbook_role_usage, role_dependencies, metric_snapshots, node_readiness, ownership_assignments — all already dropped)
DROP TABLE IF EXISTS organisations;

-- 1. credentials (referenced by organisations — already dropped)
DROP TABLE IF EXISTS credentials;
