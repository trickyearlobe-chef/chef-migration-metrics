-- SPDX-License-Identifier: Apache-2.0

-- A Test Kitchen run that timed out is a statement about the lab, not about the
-- cookbook. It used to be counted as a cookbook failure, which rolled up to
-- tk_status = 'failed' and made readiness call the cookbook incompatible even
-- when CookStyle passed — blocking every node running it.
--
-- The rollup queries now count `passed = false` only. Those queries run when a
-- kitchen result is written or an exclusion changes, so without this backfill
-- every repo already carrying a timeout keeps its stale verdict until the next
-- kitchen run — on an estate that mostly times out, indefinitely.
--
-- Readiness is not touched here: node_readiness.blocking_cookbooks is rebuilt
-- by the next analysis run, which reads these columns.
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
        COUNT(*) FILTER (WHERE passed = true)     AS passed_count,
        COUNT(*) FILTER (WHERE passed = false)    AS failed_count,
        COUNT(*) FILTER (WHERE passed IS NOT NULL) AS total_count
    FROM git_kitchen_results_active
    GROUP BY git_repo_name, git_repo_url
) counts
WHERE gr.name = counts.git_repo_name
  AND gr.git_repo_url = counts.git_repo_url;
