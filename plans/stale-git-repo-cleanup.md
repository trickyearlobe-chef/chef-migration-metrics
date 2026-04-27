# Stale Git Repo Cleanup

## Goal

Fix bug where stale `git_repos` rows from previously-failed base URLs persist and cause scanners to process repos that were never successfully cloned.

## Specs to Read

- None required — this is a bug fix in `internal/collector/git.go` and `internal/datastore/git_repos.go`.

## Steps

1. Add `ListClonedGitRepos` to datastore (filters `clone_status = 'ok'`)
2. Add `DeleteStaleGitRepos(ctx, name, keepURL)` to datastore — deletes rows for same name but different URL, plus orphaned committers
3. Write tests for both new methods
4. Update `fetchGitCookbooks` to call `DeleteStaleGitRepos` after successful upsert
5. Replace all 6 `ListGitRepos` calls in collector.go with `ListClonedGitRepos`
6. Write collector test verifying stale cleanup on success
7. Verify all tests pass

## Acceptance Criteria

- When a cookbook succeeds on URL A, any rows for the same name with different URLs are deleted (including cascaded child data and committers)
- All scanners (cookstyle, kitchen analysis, autocorrect, complexity, platform coverage) only process repos with `clone_status = 'ok'`
