-- A git repo whose clone has failed can't be verified, so its materialised
-- cookstyle/compatibility status must be 'untested' — not a stale
-- ready/needs_review/blocked verdict left over from a previous successful scan.
-- The clone-failure write path (MarkGitRepoCloneFailed / UpsertGitRepoFailed) and
-- the recompute functions now enforce this invariant; this one-time backfill
-- corrects existing Missing repos on upgrade (they were showing e.g. needs_review
-- in the Git Repos list while their detail correctly showed Untested).

UPDATE git_repos
SET cookstyle_status = 'untested',
    compatibility_status = 'untested',
    updated_at = now()
WHERE clone_status = 'failed'
  AND (cookstyle_status <> 'untested' OR compatibility_status <> 'untested');
