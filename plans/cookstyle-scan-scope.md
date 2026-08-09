# Cookstyle scan scope — the repository is not the cookbook

The journey is [trusting what the scan says](../journeys/scan-trust.md). The verdict, the
prevalence counts and the non-blocking display are built; what is left is below.

## Decisions taken during the build

**The server-side under-count went to [snagging](todo-snagging.md), not here.** It is the
opposite fault to the one being corrected, and fixing it makes the server side scan *more*
files — every Rakefile and CI definition. That is only safe once the exclusion list exists,
which is the ordering the two now have.

**Two counts per cop survived contact with the code, with one addition.** The estate view
carries `cookbooks_affected` (where the cop can block) alongside `cookbooks_excluded_only`, as
recorded. What the code forced out into the open is that the *same* split has to be applied to
"unblocks" and to the headline blocker-cookbook count, or fixing a Rakefile would be credited
with releasing cookbooks it does not release. A filter would have done that silently and
wrongly; two counts made it a decision.

A cookbook carrying a cop in both a recipe and a Rakefile is counted once, as affected, so the
two columns can be added together without double-counting.

**Complexity is scoped too.** A cookbook that is Ready because its only blocker sits in a
Rakefile must not also score as expensive work — the two numbers are read side by side.

**The deliberately-red mechanism was not built.** It was planned for a journey property with no
test behind it; the test is green, so a build tag and make target to hold a red would have been
infrastructure with nothing in it. If another unproven property needs one, build it then.

## Remaining

- **The exclusion list is curated in code, not yet operator-editable, and that is the gap that
  matters most.** It follows the existing cop-classification shape (curated defaults in code,
  operator overrides in the DB) but only the first half exists.

  The seed list reaches files with predictable names. It cannot reach **an ordinary script that
  only runs because a build job invokes it** — that can sit at any path under any name, and
  nothing in the file says what runs it. Only somebody who knows the job can exclude it. So the
  editable half is not a nicety on top of a working feature; for that case it *is* the feature.
  Wiring overrides through the config store and an admin screen is the next chunk.

- **There is no way to list the cookbooks carrying a cop only outside cookbook code.** The count
  is on the cop row; the drill-down beneath it deliberately lists only cookbooks the cop can
  block, so it still totals `cookbooks_affected`. A second drill-down, or a scope toggle on the
  existing one, would close it.

- **Nothing has been run against real customer-shaped data.** The seed patterns were chosen from
  Chef's generator, not measured against the estate. The number worth checking first is how far
  the ~95% blocked figure actually moves.
