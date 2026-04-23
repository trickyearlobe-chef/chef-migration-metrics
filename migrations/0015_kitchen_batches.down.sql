-- SPDX-License-Identifier: Apache-2.0

DROP INDEX IF EXISTS idx_git_repos_kitchen_excluded;
ALTER TABLE git_repos
    DROP COLUMN IF EXISTS kitchen_excluded_at,
    DROP COLUMN IF EXISTS kitchen_excluded_by,
    DROP COLUMN IF EXISTS kitchen_exclude_reason,
    DROP COLUMN IF EXISTS kitchen_excluded;

DROP TABLE IF EXISTS kitchen_batches;
