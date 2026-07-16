# Git repo URL migration — collector doesn't follow a moved repo while the old remote stays reachable

Handoff plan for a fresh thread. Analysis done 2026-07-16; no code written yet.

## Problem

An active **Stash → GitLab** migration is underway; the two systems are mutually
unaware and **neither redirects** (SSH transport has no redirect anyway — see below).
The collector pins each cookbook to the URL it was **originally cloned from** and never
migrates it to the new location as long as the old remote answers. So every cookbook
already cloned from Stash keeps being scanned from Stash — silently serving stale
content — even after its authoritative copy moves to GitLab, until someone manually
resets it one repo at a time. This does not scale for the estate and is silent (no
error, just wrong data).

## Why it happens (code evidence, `internal/collector/git.go`)

- `CloneOrPull` (~L151): for an existing clone it reads the **on-disk `origin`**
  (`readRemoteURL`) and fetches from it, **deliberately ignoring the caller's candidate
  URL** ("On subsequent runs the pull path ignores the passed-in repoURL").
- Self-heal (~L193): a re-clone from the candidate URL only fires when
  `git fetch origin` **fails** (dead origin). A moved-but-reachable Stash origin fetches
  fine, so self-heal never triggers.
- `DeleteStaleGitRepos` (~L561) is only DB-row hygiene after a successful clone/pull —
  it does not migrate a reachable clone.
- The only remedy today is manual, per-cookbook: `POST /api/v1/cookbooks/:name/reset-git`
  (`internal/webapi/handle_cookbook_reset_git.go` → `datastore.ResetGitRepoCloneStatus`
  + local-clone-dir deletion). Its own doc comment states the intent: "useful when a
  repository has moved (e.g. from Stash to GitLab) and the stored git_repo_url is stale."

SSH has no redirect: `git fetch` over SSH runs `git-upload-pack <path>` on the named
host — no 3xx/`Location`. Only HTTPS follows HTTP 301s; GitHub's SSH "moved" courtesy is
server-specific and Stash/self-managed GitLab don't do it. `url.insteadOf` is a blunt
global rewrite that would break not-yet-migrated repos. So a URL trick is not the fix.

## Config the fix builds on

`git_base_urls` is an **ordered** config list (order = preference). `BuildGitCookbookURLs`
/ `ResolveGitCookbookURL` (`git.go` ~L397–L448) build candidate URLs from it in order;
`fetchGitCookbooks` (~L448) drives `CloneOrPull` per cookbook.

## Fix A (durable) — origin reconciliation

In `fetchGitCookbooks` (it holds the ordered candidate list; `CloneOrPull` only sees one
URL), before reusing an existing clone, compare the clone's on-disk `origin` base against
the configured `git_base_urls`:

- If a **higher-priority** candidate than the on-disk origin's base is configured
  (GitLab ahead of Stash), attempt a re-clone from the preferred candidate(s), **falling
  back to the current origin if the repo isn't on the new host yet** (clone fails → next
  candidate → eventually the still-valid Stash URL).
- Result: a **self-completing** migration — each cookbook auto-migrates to GitLab the
  moment it appears there, stays on Stash until then, no flag day, no manual resets.

Design points to settle first:
- Confirm `git_base_urls` ordering semantics encode preference (they appear to).
- Decide the trigger: reconcile only when the on-disk origin's base is **absent from**
  `git_base_urls`, or also when a **higher-priority** base exists (the latter migrates
  proactively; the former only after Stash is removed from config — safer, less churn).
- Avoid surprise mass re-clones: gate behind the config actually changing; log every
  migration; keep `DeleteStaleGitRepos` row-hygiene working with the new origin.
- Preserve existing status semantics (`Changed`/`WasCloned`, HEAD-SHA change detection,
  `UpsertGitRepo`, no duplicate rows — cf. `93e294b`).

## Fix B (stopgap) — bulk reset by URL pattern

A one-shot admin action to migrate the in-flight estate now: reset **all** git repos
whose stored `git_repo_url` matches a base pattern (e.g. the Stash base) — for each,
delete the local clone dir **and** reset `clone_status` to pending (both, else
`isGitRepo(dir)` stays true and it re-fetches Stash). Next cycle re-clones from the
preferred GitLab candidate. Model on `handle_cookbook_reset_git.go`; add a
pattern-scoped datastore method alongside `ResetGitRepoCloneStatus`.

## Acceptance

- TDD (tests first, per repo conventions).
- Fix A: a repo whose on-disk origin is a de-prioritised/removed base URL re-clones from
  the preferred candidate when it exists there; falls back to the old origin when it does
  not; unchanged repos are a cheap fetch (no needless re-clone); no duplicate DB rows;
  migration is logged.
- Fix B: pattern reset removes local clones + resets status for exactly the matching
  repos; next collection re-clones them from the preferred base.
- Update `specifications/data-collection.md` (git fetch/clone-or-pull + migration
  behaviour) and record any spec divergence.

## Files

`internal/collector/git.go` (fetchGitCookbooks, CloneOrPull, readRemoteURL, candidate
build), `internal/datastore/git_repos.go` (ResetGitRepoCloneStatus + new pattern reset;
DeleteStaleGitRepos), `internal/webapi/handle_cookbook_reset_git.go` (+ bulk endpoint),
`specifications/data-collection.md`. Related commits: `bb3bfe3` (fetch-failure
self-heal), `93e294b` (no duplicate row on failed fetch), `218067e` (server cache reuse).
