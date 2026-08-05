# Snagging — found by using it

Defects found by the product owner working through the shipped app, rather than by
tests. Kept separate from the feature backlogs because these are *faults in what is
built*, not work not yet started.

**How to work this list.** Reproduce first, then write the failing test, then fix. Every
item here got past a green suite, so a fix with no new test is a fix that will be undone.

---

## Open

- **"Browse tables" on the database import returns "Method not allowed."** Reported by the
  product owner 2026-08-05 from the shipped app: Ownership → Import → File or database, a
  SQL Server connection chosen, and the button answers with a red error banner. The screen
  offers table browsing precisely because whoever sets an import up usually cannot inspect
  the database, so the fallback is writing a query blind against a schema you cannot see.

  **One missing line, as far as this was traced — verify each of these rather than trusting
  it.** The query exists (`ownershipsql.ListTables`,
  `internal/ownershipsql/source.go:243`), the handler exists (`handleIntakeListTables`,
  `internal/webapi/handle_ownership_intake.go:1127`), and `handleOwnershipIntake` already
  dispatches `/api/v1/ownership/import/tables` to it at `:83-84`. The frontend posts a
  multipart form to that path (`frontend/src/api/ownership.ts:328`).

  What is missing is the mux registration. `internal/webapi/router.go:861-865` registers
  `import/profile`, `import/preview`, `import/commit`, `import/mappings` and
  `import/mappings/` — every case in that dispatch switch **except** `tables`. So the
  request never reaches the handler written for it.

  **Not established, and it matters more than this endpoint: why the response is 405 rather
  than 404.** An unregistered path should match no pattern, so something else is answering —
  a catch-all, or the single-page-app fallback. Find out before fixing. It means other
  unrouted paths are reporting a method error instead of a missing one, which is what made
  this look like a permissions problem.

  Reproduce first, then the failing test, then fix. Worth having: a test that the path
  answers a POST, and one that an unregistered path under `import/` returns 404 — the second
  is the general fault and outlives this endpoint. A registration list kept in step with a
  dispatch switch by hand will drift again, so consider a test asserting every case in that
  switch has a route.

---

## Fixed

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
