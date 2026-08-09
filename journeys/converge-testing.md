# Proving a cookbook really runs, not just that it reads clean

**As the engineer running the upgrade, I need cookbooks actually built and converged on real
machines running the target version, because reading the code only tells me what somebody
thought of while writing a rule.**

Static analysis finds what it has a rule for. The failures that hurt are the ones nobody
wrote a rule for — a dependency that will not resolve any more, a resource that changed
behaviour rather than disappearing, something that only shows up on Windows. The only way to
find those is to run the thing.

## What I need

To take a set of cookbooks and have them converged on the target version without standing
over it. Thousands of them, over days, picking up where it left off.

To choose the set the way I think about the estate — these repositories, these platforms,
the ones that block the most machines, the ones I own — rather than working through
everything alphabetically.

To see what happened: which converged, which did not, and for the ones that did not, enough
output to tell whether the cookbook broke or the lab did.

To know which cookbooks **cannot** be tested this way at all, because they have no test
setup. That is not a pass and it is not a failure; it is a gap, and I need it named as one
so I know what my coverage actually is.

Before a bulk run, to know what I am about to launch against: which platforms the estate's
test configurations actually name, and where a repository carries its own local
configuration that would fight ours.

## The decisions behind it

**A converge result on its own is not a verdict.** It is one signal beside the static
analysis, and it is the weaker one for trust purposes, because it depends on a lab.

**Our lab's failures must never be charged to the cookbook.** This is the whole reason the
previous approach lost credibility. When credentials change, or the address pool empties, or
hardware goes away, every run fails for reasons that have nothing to do with any cookbook —
and each of those failures then blocks every machine running it. Measured on the customer
estate on 2026-08-03, **89% of converge failures were of that kind**: nine out of ten reds
were about us, not about the code.

**So whether a converge failure can block at all is a switch an administrator controls.** On
by default, because when the lab is sound a real converge failure is the best evidence there
is. Turned off, results are still collected and still shown — they simply stop counting
towards anything. That is the honest response to a lab we do not currently trust: keep
looking, stop concluding. It is deliberately not automatic, because the tool cannot tell a
broken lab from a genuinely broken estate.

**A repository's own test configuration is respected, not replaced.** Whatever the team put
there — including the steps they run before a converge — has to survive, or we are testing
something that is not what they ship.

## What proves it

That a converge result alone never produces a verdict is pinned: with no static analysis to
go on, the cookbook comes back [as untested rather than
judged](internal/analysis/readiness_test.go#TestCheckCookbookCompatibility_TKConvergeFailIsUntested).
A cookbook with no test setup at all is [judged on the static analysis
alone](internal/analysis/readiness_test.go#TestCheckCookbookCompatibility_CSPass_NoTestSuite)
rather than being marked failed for the absence.

The switch is pinned in all three of its behaviours, which matters because the middle one is
the property that keeps a distrusted lab from silently deleting evidence:

- With it on, a converge failure [outranks a clean
  scan](internal/analysis/readiness_test.go#TestCheckCookbookCompatibility_TKFailureBlocksWhenSwitchIsOn).
- With it off, a converge failure [does not block, and is still
  reported](internal/analysis/readiness_test.go#TestCheckCookbookCompatibility_TKFailureDoesNotBlockWhenSwitchIsOff).
- With it off, a converge **pass** [stops counting
  too](internal/analysis/readiness_test.go#TestCheckCookbookCompatibility_TKPassIsNotCountedWhenSwitchIsOff),
  so the switch cannot be used to keep the good news and drop the bad.

Reading a repository's test configuration the way the tool itself would — [overrides merged
rather than replacing wholesale](internal/analysis/kitchen_analyser_test.go#TestMergeKitchenConfigs_DeepMerge),
with [lists replaced rather than
concatenated](internal/analysis/kitchen_analyser_test.go#TestMergeKitchenConfigs_ArrayReplace) —
is what makes "what am I about to launch against" a real answer rather than a guess, and
[locally invented platform attributes
survive](internal/analysis/kitchen_analyser_test.go#TestExtractKitchenConfig_PlatformExtensions)
the reading rather than being normalised away. Selecting work by whether a repository has a
test setup at all is pinned by [the batch
resolver](internal/batch/resolver_test.go#TestResolveBatch_HasTestSuiteFalse).

**Nothing proves the thing the journey is actually for.** No test establishes that a real
converge on a real machine finds failures the static analysis missed. That is the entire
premise, and it is confirmed only by having done it. If converge testing never catches
anything static analysis did not, the 89% figure above says to switch it off and save the
lab.

**Nothing proves the lab-versus-cookbook distinction is drawn correctly.** The switch makes
the distinction *available* — it does not classify any individual failure. Deciding that a
given red was the lab's fault is a human act, recorded elsewhere.

**The load-bearing assumption:** that a converge that never reached the converge step is
distinguishable, in what we store, from one that converged and failed. Everything above
depends on being able to tell those apart. Verify that before designing anything that
reports on failure causes — if the two look the same in the data, the 89% cannot be measured
again and the switch is the only defence left.
