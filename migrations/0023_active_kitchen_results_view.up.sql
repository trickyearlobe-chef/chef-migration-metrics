-- SPDX-License-Identifier: Apache-2.0

-- git_kitchen_results_active: kitchen results excluding user-excluded instances.
-- All aggregate TK status queries should use this view to ensure consistency.
CREATE VIEW git_kitchen_results_active AS
SELECT gkr.*
FROM git_kitchen_results gkr
WHERE NOT EXISTS (
    SELECT 1 FROM kitchen_instance_exclusions kie
    WHERE kie.git_repo_name = gkr.git_repo_name
      AND kie.suite_name = gkr.suite_name
      AND kie.platform_name = gkr.platform_name
);
