-- Revert migration 0005: Remove error_message from CookStyle result tables

ALTER TABLE git_repo_cookstyle_results
    DROP COLUMN IF EXISTS error_message;

ALTER TABLE server_cookbook_cookstyle_results
    DROP COLUMN IF EXISTS error_message;
