-- SPDX-License-Identifier: Apache-2.0

DROP FUNCTION IF EXISTS gkr_failure_kind(TEXT, BOOLEAN, BOOLEAN);

-- Restore the view without failure_kind, so 0065's down script can drop the
-- column it names.
DROP VIEW IF EXISTS git_kitchen_results_active;

CREATE VIEW git_kitchen_results_active AS
SELECT gkr.id, gkr.git_repo_name, gkr.git_repo_url, gkr.target_chef_version,
       gkr.commit_sha, gkr.platform_name, gkr.suite_name, gkr.instance_name,
       gkr.driver_used, gkr.passed, gkr.timed_out, gkr.output,
       gkr.duration_seconds, gkr.error_message, gkr.started_at,
       gkr.completed_at, gkr.created_at
FROM git_kitchen_results gkr
WHERE NOT EXISTS (
    SELECT 1 FROM kitchen_instance_exclusions kie
    WHERE kie.git_repo_name = gkr.git_repo_name
      AND kie.suite_name = gkr.suite_name
      AND kie.platform_name = gkr.platform_name
);
