-- Migration 0005: Add error_message to CookStyle result tables
--
-- CookStyle (RuboCop) exit code 2 indicates a crash (invalid .rubocop.yaml,
-- Ruby exception, gem load error) as opposed to exit code 1 which means
-- offences were found. Previously, exit code 2 was treated the same as
-- exit code 1, causing crashed scans to be recorded as either "compatible"
-- or "incompatible" depending on what stdout contained.
--
-- The error_message column records the error detail when a scan fails due
-- to exit code >= 2. Consumers use it to distinguish real scan results
-- (error_message IS NULL) from errored scans (error_message IS NOT NULL).
--
-- Semantics:
--   passed=true,  error_message IS NULL  → scan passed, compatible
--   passed=false, error_message IS NULL  → scan failed, incompatible
--   passed=false, error_message NOT NULL → scan errored, treat as untested

ALTER TABLE server_cookbook_cookstyle_results
    ADD COLUMN error_message TEXT;

ALTER TABLE git_repo_cookstyle_results
    ADD COLUMN error_message TEXT;
