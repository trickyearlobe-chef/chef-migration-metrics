-- Cookbook-subject entries cannot be represented once the column means repos
-- again. They are left in place with their names intact rather than deleted —
-- the register never destroys a verdict — so a re-migration recovers them.
ALTER INDEX idx_failure_register_subject
    RENAME TO idx_failure_register_repo;
ALTER INDEX idx_failure_register_one_open_per_subject
    RENAME TO idx_failure_register_one_open_per_repo;

ALTER TABLE failure_register_entries DROP COLUMN subject_type;

ALTER TABLE failure_register_entries
    RENAME COLUMN subject_name TO git_repo_name;
