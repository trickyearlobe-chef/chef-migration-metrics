-- Repair drift in the materialised git_repos.cookstyle_status / compatibility_status
-- columns. These were maintained only incrementally (per scan, and per rescore
-- ONLY for results whose status changed) and blanked to 'untested' on a target
-- change — with no full re-materialisation step. So a rescore/reclassification
-- that did not change a result's stored status left the repo column stale, and
-- the Git Repos list (which filters the materialised column) then disagreed with
-- the dashboard summary (which reads git_repo_cookstyle_results directly).
--
-- Ongoing correctness is now handled by RecomputeAllGitRepoCookstyleStatus after
-- every rescore; this one-time backfill corrects existing rows on upgrade by
-- re-deriving each repo's status from its latest cookstyle result (same
-- derivation as RecomputeGitRepoCompatibilityStatus).

UPDATE git_repos gr
SET compatibility_status = COALESCE(l.compat, 'untested'),
    cookstyle_status     = COALESCE(l.cs_status, 'untested'),
    updated_at = now()
FROM (
    SELECT DISTINCT ON (git_repo_name, git_repo_url)
        git_repo_name,
        git_repo_url,
        CASE
            WHEN error_message != '' THEN 'error'
            WHEN passed = true THEN 'compatible'
            WHEN passed = false THEN 'incompatible'
        END AS compat,
        CASE
            WHEN error_message != '' THEN 'untested'
            ELSE NULLIF(cookstyle_status, '')
        END AS cs_status
    FROM git_repo_cookstyle_results
    ORDER BY git_repo_name, git_repo_url, scanned_at DESC
) l
WHERE gr.name = l.git_repo_name
  AND gr.git_repo_url = l.git_repo_url;
