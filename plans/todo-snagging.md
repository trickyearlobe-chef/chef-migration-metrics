# Snagging — found by using it

Defects found by the product owner working through the shipped app, rather than by
tests. Kept separate from the feature backlogs because these are *faults in what is
built*, not work not yet started.

**How to work this list.** Reproduce first, then write the failing test, then fix. Every
item here got past a green suite, so a fix with no new test is a fix that will be undone.

---

## Open

- [ ] **No way to say "not a duplicate".** The duplicates view offers a merge and nothing
  else, so a pair a person has looked at and rejected stays on the list — and the scan
  rebuilds the table each run, so it comes back every time. The consequence is that the
  list can only grow in the reader's mind: there is no way to work it down to nothing,
  which is the only state that makes it worth opening. Needs a dismissal that survives a
  rescan, so it has to live outside `owner_duplicate_candidates` (the scan deletes and
  rebuilds that table), keyed on the ordered pair and cascading with the owners.

---

## Fixed

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
