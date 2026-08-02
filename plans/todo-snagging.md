# Snagging — found by using it

Defects found by the product owner working through the shipped app, rather than by
tests. Kept separate from the feature backlogs because these are *faults in what is
built*, not work not yet started.

**How to work this list.** Reproduce first, then write the failing test, then fix. Every
item here got past a green suite, so a fix with no new test is a fix that will be undone.

---

## Open

*(nothing outstanding)*

---

## Fixed

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
