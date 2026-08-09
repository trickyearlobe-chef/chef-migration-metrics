# Snagging — found by using it

Defects found by the product owner working through the shipped app, rather than by
tests. Kept separate from the feature backlogs because these are *faults in what is
built*, not work not yet started.

**How to work this list.** Reproduce first, then write the failing test, then fix. Every
item here got past a green suite, so a fix with no new test is a fix that will be undone.

---

## Open

- **When the session expires, screens keep showing the old data instead of asking me to sign in
  again.** Reported by the product owner 2026-08-09 from the shipped app: screens silently stop
  updating, and some render what looks like data belonging to a different view. Graphs that
  cannot draw appear instead of any indication that the session has ended.

  **Three things established in code on 2026-08-09; the third is a guess and needs reproducing.**

  There is no handling of an expired session anywhere in the frontend. `401` appears exactly once
  in the whole of `frontend/src`, and it is a comment in `AuthContext.tsx:68` about the initial
  sign-in check. Every other call turns a `401` into an ordinary error thrown at whichever screen
  made the request.

  Screen state is hand-rolled — there is no query library — so a failed refresh leaves whatever
  was already rendered in place. That is the silent non-update: nothing clears, nothing says why.

  The "other view's data" part is **not** explained yet. There is no persistent client-side cache
  of view data (only `AdminExplainPage` and `AdminPerformancePage` touch web storage), so the
  likely causes are component state surviving navigation or a stale in-flight response arriving
  after a route change. Reproduce before designing anything around it.

  **The pattern to copy already exists in the same file.** `client.ts:62-68` detects a
  maintenance response and dispatches an app-wide event on an `EventTarget` that
  `MaintenanceContext` consumes, lifting the condition out of the calling screen. An expired
  session is the same shape and wants the same treatment.

  **A trap for whoever implements it.** `client.ts:74` does `if (parsed.code) code = parsed.code`,
  so `ApiError.status` is overwritten by a `code` field in the response body. Detection keyed on
  the error's status can therefore be defeated by the body, and the fix would look done while
  failing for some endpoints. Assert on this directly.

  **Why this was never built, which is the useful part.** The retired auth specification had a
  "Session Management" section. It specified that expiry is configurable and defaults to 8h, that
  sessions are server-side rows keyed by a UUID token, the cookie flags, and why `SameSite` is Lax
  rather than Strict. It never said what the person sees when the 8h elapses. The mechanism was
  documented thoroughly enough that the section read as complete, so the omission was invisible —
  and with an 8h default, every user meets this daily. Recoverable from the tag
  `specifications-retired-2026-08-04` if the detail is wanted.

  Related principle, already stated in two journeys: a selection the server cannot parse must fail
  loudly ([named cohorts](../journeys/named-cohorts.md)), and a machine we cannot see must not read
  as fine ([node readiness](../journeys/node-readiness.md)). A session that has ended must not
  render as data. The user-facing half belongs in [getting in](../journeys/service-access.md); the
  "no screen shows stale data as current" half binds every journey and currently has no home — see
  the note in [the journeys plan](specs-as-journeys.md).

- **One role we cannot read loses the whole dependency chain.** Found on 2026-08-08 while
  writing the role-impact journey, not by anybody using the app. Expanding a run list fails
  outright when a referenced role is not found, rather than resolving what it can and naming
  the part it could not. `internal/nodekitchen/runlist_test.go:260` asserts the error, so
  this is deliberate, not an oversight — the question is whether it is still the right call.

  It matters because the whole value of the role view is seeing the inherited chain, and an
  estate with one stale role reference gets no answer instead of a partial one. Cycles and
  nesting are both handled properly, so this is the only gap of its kind.

  Not reproduced against real data, and the impact on the screens has not been traced —
  establish how often a missing role actually occurs before changing behaviour. If it is
  changed, the partial answer has to say which role was unreadable, or it becomes a silent
  undercount, which is worse than the error.

- **An owner's cookbook summary reports every cookbook untested, silently.** Found while
  mining the abandoned ownership plan on 2026-08-04, not by anybody using the app.
  `internal/datastore/owners.go:710-711` and `:776-777` query `cookbook_complexity` and
  join `cookbooks` — neither table has ever existed. The tree has `server_cookbooks` and
  `server_cookbook_complexity`; the names in the query are the real ones with the
  qualifier dropped.

  The error is discarded at `:717` (`if err == nil && complexityLabel.Valid`), so the query
  fails on every call and the value simply stays empty. No error, no log line. Blocking
  cookbooks show a blank complexity label, and per the comment above
  `GetOwnerCookbookSummary` the compatible/incompatible/untested verdict is derived from
  that same absent table.

  `28fc997` fixed the sibling git repo summary. These two were diagnosed correctly on
  2026-08-01 in a plan that was then abandoned, and the diagnosis went with it.

  Impact on the screens in production has not been traced. A test asserting an *error*
  will never go red here — the functions return wrong values rather than failing — so
  establish each one's actual behaviour from the code before writing its test.

---

## Fixed

- **"Browse tables" on the database import returned "Method not allowed."** Reported by the
  product owner 2026-08-05: Ownership → Import → File or database, a SQL Server connection
  chosen, and the button answered with a red error banner. The screen offers table browsing
  precisely because whoever sets an import up usually cannot inspect the database, so the
  fallback was writing a query blind against a schema you cannot see.

  **Two faults, and the second is the one worth carrying.** The endpoint was never
  registered on the mux — every case in the import dispatch switch had a route except
  `tables` — so the request never reached the handler written for it, and the query
  underneath had never been run by anything.

  The reason it said "method not allowed" rather than "no such endpoint" is separate. The
  single-page-app fallback catches everything unmatched, and it checked the **method before
  it checked whether the path was an API path at all**. So every unrouted non-GET API
  request reported a verb error: an endpoint that exists and refuses POST, rather than one
  that was never wired up. That is what made a wiring fault read as a permissions problem,
  and it applied estate-wide, not just here. The two checks are now the other way round,
  with the order commented as load-bearing. Page routes still answer 405 to a POST, because
  there the method really is the complaint.

  It had already produced a visible inconsistency nobody had connected: with performance
  monitoring disabled, the same unregistered endpoint answered 404 to a GET and 405 to a
  DELETE. Those two tests asserted the old behaviour and now assert 404 like their GET
  counterparts.

  **Guarding the drift, since a registration list kept in step with a dispatch switch by
  hand is what failed.** A test reads the paths the dispatch switch compares against
  straight out of its own source and asserts the mux carries each one, so the next
  divergence fails a test rather than waiting for somebody to press the button. A sweep of
  every API path literal in the package found no other unrouted case.

  A functional test now drives the button end to end against the seeded SQL Server, since
  the endpoint being unreachable meant its query had never executed against a real database.

- **A repo you had given an owner still read as unowned.** Reported by the product owner
  2026-08-02: "none of my repos are owned according to the UI", with a repo showing two
  owners on the owners page and sitting under "no owner" on the repo list.

  The product disagreed with itself about what a `git_repo` assignment's `entity_key`
  holds. The committers panel wrote the git **URL**; the repo list, the unowned filter
  and the export all read by repo **name**. So an owner assigned in the app was recorded
  where nothing that lists repos could read it, and the repo stayed on the list of work
  nobody has been made responsible for.

  It failed the other way too: the owner's own page and the cookbook's inherited owner
  both matched on the URL, so the **name**-keyed assignments the ownership import wrote
  were invisible to them. And the owner list's repo count counts assignment rows whatever
  form the key is in — so the count agreed with neither list, which is why nothing caught
  it.

  The name is canonical (repo URLs are volatile — `handle_git_repos.go:129-133`), and it
  is what the import already wrote. Fixed in the one writer that chose a form and the
  three readers that assumed the other one, with migration 0063 rewriting existing
  URL-keyed rows. Rows whose key matches no known repo are left alone: they name a repo
  this instance has never collected, and guessing would turn a visible oddity into a
  silent wrong answer.

- **Every repo an owner owns was reported as incompatible.** Found by the test written for
  the entry above, not by anybody using it. The owner page's git repo summary joined
  `git_repo_complexity.git_repo_id` to `git_repos.id` — neither column exists, so the
  query errored on every call, and the error was counted as "incompatible". A repo with a
  clean CookStyle record read exactly like a repo with a failing one.

  The join now matches on the name and URL the complexity table is actually keyed by, and
  a query error is returned rather than counted as a verdict. That swallowing is what let
  it survive: an owner page that said "this owner's repos are all incompatible" was
  indistinguishable from a real answer.

- **The rejected-row list disagreed with the rejected-row count.** Capping the per-row
  detail in the response took a flat prefix, so rejected rows sitting late in the file
  were dropped from the list while the outcome tally still counted them — 41 shown
  against 156 counted, with nothing on the page to say which was right. Self-inflicted,
  by the truncation added the same afternoon.

  Rejected rows are now kept in preference to accepted ones: they are the only kind
  anybody has to act on and there are few of them, while accepted rows are bulk. Source
  order is preserved. The heading also reads from the server's tally rather than from the
  length of the list, so the two cannot drift again.

  **The import was never affected** — the commit runs from the full report, and only the
  display was short. But a page showing two different numbers for one thing is worse than
  a page showing one wrong number, because it costs the reader their trust in both.

- **A rejection could not be undone.** Dismissing hid the pair from the list, so a
  mis-click suppressed it permanently *and* invisibly — nothing left to click. Rejected
  pairs are now listed separately with who rejected them and why, and each can be undone.
  Undoing only removes the rejection; it does not assert the pair is a duplicate, so if
  the scan no longer finds the two similar they stay absent, which is the honest outcome.

- **No way to say "not a duplicate".** The view offered a merge and nothing else, so a
  pair somebody had already rejected returned on every scan and the list could never be
  worked down to nothing — the only state that makes it worth opening.

  Dismissals live in their own table (migration 0062) rather than in
  `owner_duplicate_candidates`, because the scan deletes and rebuilds that on every run
  and a rejection stored there would be swept away. Keyed on the ordered pair so it
  matches whichever way round the caller names the two, and cascading with the owners so
  merging one away takes the dismissal with it. A reason is optional: demanding one is
  how dismissals stop being recorded, and this only ever removes a suggestion.

  Dismissing is an **operator** action rather than admin — it removes a suggestion, not a
  person — so the action column now shows for operators, with merge still admin-only. An
  empty list reports how many pairs were rejected, so "worked down to nothing" reads
  differently from "nobody has looked".

- **The duplicates view paired three unrelated people with each other.** Owners added by
  email address all carry their address as an alias (migration 0059 seeds it), and the
  scan compared whole addresses. Everyone at one company shares a domain, and a shared
  domain is most of a shared string: three names with nothing in common scored 38%, 33%
  and 30% against a 30% floor, entirely on the domain. Every pair matched.

  Fixed by scoring the alias comparison on the localpart. The measured effect on the
  reported case: 45%/40%/38% → 10%/3%/0%, all dropped. The nearest-neighbour ordering
  still uses the whole value, so the GiST index still bounds the scan, and the reported
  values are still the full addresses because a reader needs them to judge the pair.

  **It also strengthens the signal it exists for.** One person under two domains —
  the case the plan calls a strong lead — went from 44% to 100%, because the domain was
  previously diluting the part that actually identified them.

  **Why it would have got worse, not better, at scale:** the noise floor rises with the
  number of owners sharing a domain, so the view degrades fastest on exactly the estate
  it was built for. Worth remembering when judging any similarity signal here.

- **The subject picker did not narrow — typing `selinux` offered `aett_bats_core_aws`.**
  The picker sent its term as `search`, and the git repo and cookbook list endpoints read
  `name` and ignore everything else. An ignored filter does not return nothing; it returns
  the first page of the whole catalogue, so the first match alphabetically came back and it
  read as a broken match rather than a dropped parameter.

  Fixed by sending `name`. `search?` was also removed from `GitRepoFilterQuery`,
  `CookbookFilterQuery` and `NodeFilterQuery` — all three advertised a parameter their
  endpoint ignores, which is what made the mistake available. `/owners` genuinely honours
  `search`, so the owner picker was never affected. Regression test asserts the term
  reaches the API under the key the API reads.

  **The general lesson, worth carrying:** a filter parameter the server silently ignores
  fails *open*, and failing open looks like data rather than like breakage. The same shape
  caused the team-verdict filter to show every repo while displaying a filter chip. When a
  filter looks wrong, check the parameter reaches the query before suspecting the data.

- **A saved filter did not carry the owner.** Found by use, 2026-08-03. It failed silently
  in both directions: the page built its saved selection without ownership, so nothing was
  sent, and the server's allowlist would have rejected it anyway. Fixed for `owner`,
  `unowned` and `human_verdict`; the decision it records is in `plans/todo-ownership.md`.

  **The general lesson, and it is the second time on this branch:** a filter that is dropped
  on the way to the request fails *open* and reads as data. The subject picker did the same
  with `search` vs `name`. A control that cannot be saved, or that returns the whole
  catalogue, is worth suspecting before the data is.

- **The owner search box is not obvious, and the list quietly stops at 50.** Found by use,
  2026-08-03. The control opens on a list of owners with a search field above it that nothing
  draws the eye to, so it reads as "scroll this list" — which does not work on an estate with
  1,862 owners, where the list is the first 50 with no sign there are more.

  Fixed together: the cursor starts in the search box, and a capped list says "Showing 50 of
  1,862 — type to narrow". **The general shape, for the third time on this branch:** a view
  that shows part of a set without saying so reads as the whole set. The unloadable owner
  catalogue and the ignored `search` parameter were the same fault wearing different clothes.

- **The import entry points still said "file".** Found by use, 2026-08-03: the button on the
  Owners page read "Import CSV / JSON", the page said "from a file", and the tab was called
  "Map columns" — so nothing anywhere told anybody the database option existed. A feature
  nobody can find is not shipped. Now "Import owners", "from a file, or from a database", and
  a tab called "File or database".

- **Filtering by a person matched git repos but no cookbooks.** Reported by the customer
  2026-08-03. Ownership had only ever been recorded against repos, and the cookbook list
  resolved ownership against `entity_type = 'cookbook'`, so it correctly found nothing.

  **The fix is derivation, not more data.** A cookbook's owner is whoever owns the git repo it
  is built from: git is the code, a server cookbook is the deployed artefact, and a fix is made
  in the repo — which is why people say "cookbook" in standup and mean the repo. Recording it
  on both sides would be two truths that can disagree, so `resolveOwnershipFilter` derives it,
  once, for the list, the export and the dashboard alike.

  One way only: owning a cookbook does not make somebody the owner of a repo. Names are the
  join, on the same one-cookbook-per-repo assumption the readiness evaluator already uses to
  look up a human verdict.

- **A URL in the ticket field is now a link.** Asked for by the customer 2026-08-03 (a request,
  not a fault): they paste ServiceNow and Jira addresses into the failure register's ticket
  field, and a link you have to select and copy is a link nobody follows.

  **Only `http` and `https` are linked.** The register is free text any operator can write and
  everyone else reads, so a `javascript:` or `data:` address rendered as a link would be a
  script one colleague runs in another's session. Anything else stays as text, which is also
  the right answer for a bare ticket number — guessing a scheme would turn `INC0012345` into a
  link to nowhere.

  Shows the last part of the address, with the whole thing on hover, because a full URL pushes
  a narrow column about.

- **The console logs a 404 for platform coverage when opening remediation.** Found by use with
  devtools open, 2026-08-03. Nothing is broken: `/cookbooks/:name/platform-coverage` answers
  404 when coverage has never been computed, and the page treats it as optional and hides the
  card. The browser logs any non-2xx, so the line appears regardless.

  **What is worth fixing is not the console line.** The handler cannot tell "this cookbook has
  no coverage yet" from "there is no such cookbook", and the page cannot tell either from "the
  request failed" — see the tech-debt item on silent catches. The shape of the fix: 200 with an
  explicit "not evaluated" for a cookbook that exists, 404 only for one that does not, and a
  page that says "no coverage yet" rather than showing nothing.

  **Expect more of these while navigating with the console open.** They are mostly this same
  pattern rather than twelve separate faults.
