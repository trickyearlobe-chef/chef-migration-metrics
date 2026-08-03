-- SPDX-License-Identifier: Apache-2.0

-- Re-materialise the TK columns under the old rule, where a timed-out run
-- counted as a cookbook failure. This restores what the old binary's rollup
-- queries would produce, so the columns match the code being rolled back to.
UPDATE git_repos gr
SET tk_passed = counts.passed_count,
    tk_total  = counts.total_count,
    tk_status = CASE
        WHEN counts.passed_count > 0 AND counts.failed_count > 0 THEN 'partial'
        WHEN counts.failed_count > 0 THEN 'failed'
        WHEN counts.passed_count > 0 THEN 'passed'
        ELSE 'untested'
    END,
    updated_at = now()
FROM (
    SELECT
        git_repo_name,
        git_repo_url,
        COUNT(*) FILTER (WHERE passed = true) AS passed_count,
        COUNT(*) FILTER (WHERE passed = false OR timed_out = true) AS failed_count,
        COUNT(*) FILTER (WHERE passed IS NOT NULL OR timed_out = true) AS total_count
    FROM git_kitchen_results_active
    GROUP BY git_repo_name, git_repo_url
) counts
WHERE gr.name = counts.git_repo_name
  AND gr.git_repo_url = counts.git_repo_url;
