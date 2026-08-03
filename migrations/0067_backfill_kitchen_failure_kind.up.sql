-- SPDX-License-Identifier: Apache-2.0

-- Classify the results captured before failure_kind existed, and re-materialise
-- the repo verdicts that follow from them. Separate from 0065 and 0066 because a
-- migration file is executed as one batch: a statement cannot reference a
-- column added earlier in the same file.

UPDATE git_kitchen_results
SET failure_kind = gkr_failure_kind(output, passed, timed_out)
WHERE passed IS DISTINCT FROM TRUE;

-- Re-materialise the repo verdicts, now that a lab failure is no longer a
-- cookbook failure. Without this, every repo already carrying one keeps its
-- stale 'failed' until its next kitchen run — on an estate that mostly fails
-- environmentally, indefinitely. Readiness rebuilds from these columns on the
-- next analysis run.
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
        COUNT(*) FILTER (WHERE passed = false AND failure_kind NOT IN
            ('create_failed', 'destroy_failed', 'network_timeout', 'timeout', 'no_converge')) AS failed_count,
        COUNT(*) FILTER (WHERE passed = true OR (passed = false AND failure_kind NOT IN
            ('create_failed', 'destroy_failed', 'network_timeout', 'timeout', 'no_converge'))) AS total_count
    FROM git_kitchen_results_active
    GROUP BY git_repo_name, git_repo_url
) counts
WHERE gr.name = counts.git_repo_name
  AND gr.git_repo_url = counts.git_repo_url;
