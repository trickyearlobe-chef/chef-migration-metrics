# Snagging — found by using it

Defects found by working through the shipped app, rather than by tests.

**How to work this list.** Reproduce first, then write the failing test, then fix. Every
item here got past a green suite, so a fix with no new test is a fix that will be undone.

---

## Open

**An admin changes the target version and everyone else keeps the old one.** The version
list is fetched once when a page loads and never again, and twenty-two screens read it. There
is exactly one target version and it decides every verdict, so anybody with a tab already open
goes on reading answers computed for a version they are no longer looking at, with nothing on
screen saying so. It corrects itself on a full reload, which nobody knows to do.

- **An auto-correct preview outlives the scan it belongs to.** Generation is skipped
  whenever a preview already exists, and the check reads only its timestamp. But a preview
  hangs off a scan result id, and a rescan overwrites the result in place rather than making
  a new one — so the diff survives a scan of different source and nothing says so. A git repo
  hits this routinely: its result is keyed on repo and target version with no commit, so every
  time HEAD moves the preview describes code that is gone. A server cookbook version is
  usually left alone once scanned, so it hits this rarely — but when it does, somebody
  deliberately asked for a rescan, which is the strongest possible sign the old diff is
  unwanted. The functions that clear previews exist (`ResetPreviews`, `ResetAllPreviews`)
  but have no route and no control, so the only way out is a shell on the server.

  **Fix:** drop the preview whenever a scan writes over an existing result — the overwrite is
  the signal, and it needs no comparison of commits or versions. Expose a reset alongside
  `POST /api/v1/admin/rescan-all-cookstyle` for anything already stranded.

- **Two run-history addresses accept `page` and silently ignore it.**

  `GET /api/v1/nodes/runs/{organisation}/{node}` and
  `GET /api/v1/run-events/nodes/{organisation}/{node}` both call `ParsePagination` and pass only
  `params.Limit()` to `ListConvergeRunsForNode`, whose signature takes a limit and no offset
  (`internal/datastore/converge_runs.go:143`). So `per_page` works, `page` does nothing, and
  asking for page 2 returns page 1 again. Neither returns any pagination metadata, so a caller
  cannot tell from an answer that it was bounded at all.

  **Fix:** give `ListConvergeRunsForNode` an offset, have both handlers pass it, and switch the
  two declarations to `paginated()`. `cappedNotPaged` then has no users and should go with them.

- **Viewers can read the logs, from the interface and from the API.** Not caused by API
  credentials — a credential carries its account's level, so the
  level itself is wrong.

  **Established @ ee8585dd.** All three log addresses use `r.protect` = any authenticated
  session (`internal/webapi/router.go`). "Logs" is in the main nav array, and the
  sidebar filters only on `isAdmin` (`frontend/src/components/AppLayout.tsx`) — so a viewer
  sees the link and it works.

  **Wanted:** logs and diagnostic bundles for operators and admins only, and absent from a
  viewer's interface.

- **A repo stayed blocked by a file the exclusion list already excludes.** A repo read blocked
  on a `File.exists?` finding in an excluded test-harness file, while the cop pages showed
  nothing blocking it. Saving any scan-scope entry fixed it, and the dashboard moved — so the
  verdicts were stale, not mis-matched.

  **So the seeded list has no trigger of its own.** `DefaultScanScopeExclusions` ships in code
  (`internal/analysis/scan_scope.go:72-111`); adding a pattern to it changes what every verdict
  *would* derive to and re-derives nothing. The estate keeps answering with the old scope until
  somebody happens to touch an unrelated setting, and nothing anywhere says it is out of date.

  **Shape of the fix, not yet decided.** Record what scope a stored verdict was derived under and
  re-derive on startup when it no longer matches, so a release that changes the seed list heals
  itself. A cruder version — rescore unconditionally at startup — costs a pass over the results
  table on every restart and would do nothing visible most times. Either way the test is that a
  changed seed list moves a stored verdict with no operator action.

- **The Chef server side of a scan misses Ruby that Chef genuinely ships.**
  `CookbookVersionManifest`
  parses only the legacy cookbook segments and never `all_files`, and a segment view cannot
  express a file in an arbitrary directory. So a cookbook holding real code outside those
  directories is scanned without it, and the verdict is quietly based on less than what runs.

- **When the session expires, screens keep showing the old data instead of asking me to sign in
  again.** Screens silently stop updating, and some render what looks like data belonging to a
  different view. Graphs that cannot draw appear instead of any indication that the session has
  ended.

  There is no handling of an expired session anywhere in the frontend. `401` appears exactly once
  in the whole of `frontend/src`, and it is a comment in `AuthContext.tsx:68` about the initial
  sign-in check. Every other call turns a `401` into an ordinary error thrown at whichever screen
  made the request.

  Screen state is hand-rolled — there is no query library — so a failed refresh leaves whatever
  was already rendered in place. That is the silent non-update: nothing clears, nothing says why.

  **The pattern to copy already exists in the same file.** `client.ts:62-68` detects a
  maintenance response and dispatches an app-wide event on an `EventTarget` that
  `MaintenanceContext` consumes, lifting the condition out of the calling screen. An expired
  session is the same shape and wants the same treatment.

  **A trap for whoever implements it.** `client.ts:74` does `if (parsed.code) code = parsed.code`,
  so `ApiError.status` is overwritten by a `code` field in the response body. Detection keyed on
  the error's status can therefore be defeated by the body, and the fix would look done while
  failing for some endpoints. Assert on this directly.

- **An owner's cookbook summary reports every cookbook untested, silently.**
  `internal/datastore/owners.go:710-711` and `:776-777` query `cookbook_complexity` and
  join `cookbooks` — neither table has ever existed. The tree has `server_cookbooks` and
  `server_cookbook_complexity`; the names in the query are the real ones with the
  qualifier dropped.

  The error is discarded at `:717` (`if err == nil && complexityLabel.Valid`), so the query
  fails on every call and the value simply stays empty. No error, no log line. Blocking
  cookbooks show a blank complexity label, and per the comment above
  `GetOwnerCookbookSummary` the compatible/incompatible/untested verdict is derived from
  that same absent table.

  A test asserting an *error*
  will never go red here — the functions return wrong values rather than failing — so
  establish each one's actual behaviour from the code before writing its test.
