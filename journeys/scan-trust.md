# Trusting what the scan tells me

**As the engineer running the upgrade, I need the scan to tell me apart the findings that
will actually break us from the ones that are just tidying, so that a red mark means
something and I keep reading them.**

The scanner reports hundreds of things per cookbook and it does not know which of them
matter for a version change. Chase a few reds that turn out to be house-style preferences and
I stop opening the list.

## What I need

Three plain answers per kind of finding, and no fourth one that dresses a guess up as an
answer:

- **This will break** — we know the thing it names is gone in the version we are moving to.
- **Somebody has to decide** — it might matter and nobody has established that it does.
- **This is harmless** — and there is a positive reason for saying so, not just an absence
  of evidence that it is dangerous.

For anything marked as breaking or harmless, why — on what basis that call was made. I will
be asked to justify a red to the person whose cookbook it is, and "the tool says so" is not
an answer that survives that conversation.

To be able to disagree. When I have established what a finding really means, my decision
sticks and everything downstream follows it, so that the next person does not rediscover the
same thing.

## The decisions behind it

**Severity from the scanner is not a migration verdict, ever.** Two findings can carry the
same severity where one is a hard crash on the target version and the other is a tool nobody
maintains that still runs perfectly. Ranking by severity is the mistake this exists to
replace, not a shortcut to fall back on when nothing better is available.

**Being wrong is not symmetrical, so the bar is not symmetrical.** A wrong "this will break"
costs somebody a wasted afternoon, and they tell me, and I fix it. A wrong "this is
harmless" hides a real blocker until production finds it, and nobody tells me because
nothing looked wrong. So harmless needs the stronger evidence, and **anything uncertain
becomes somebody has to decide — never harmless.**

**A person's decision outranks everything the tool worked out**, including its own confident
reds.

**One target version at a time.** Findings are judged per finding, not per version, because
there is only one version we are moving to. A per-version matrix was tried and removed.

## The repository is not the cookbook

A repository holds more than the cookbook. It holds the pipeline definition, the helper tasks
somebody wrote to run the tests, the test suites themselves. Those files never run on a machine
during a converge — and when the same helper task appears in nearly every repository, one
finding inside it makes nearly every cookbook look broken. A headline figure can be almost
entirely this.

**So a cookbook's verdict is about the code that ships and runs.** A finding in a file the
converge never executes does not block the cookbook.

**But the work does not disappear, because it is real work.** Those helper tasks and pipelines
will break on the new Ruby exactly as predicted; they are simply somebody else's problem and a
different piece of work. So a finding outside the cookbook stays visible on the cookbook, marked
as not blocking, and it is counted across the estate — because the thing I most need to know
about it is how widespread it is. One fix repeated across three thousand repositories is a
different conversation from three thousand separate problems, and I cannot have that conversation
if the number is buried or missing.

**Which files are excluded is a decision somebody makes and can see, not a rule inferred.** Two
tempting shortcuts are both wrong. Judging by what the packaging tool uploads does not work —
it uploads very nearly everything, including directories nobody would call cookbook code.
Inferring the set of files the converge *could* reach does not work either, because code can
load code: any allowlist quietly discards whatever nobody thought of, and that is the direction
that hides a real blocker. An explicit list of files we assert do not run is a small, specific,
checkable claim, and it can be argued with.

**A file we ignore because a repository told us to is a file we ignore for the wrong reason.** A
repository's own declaration of what is not cookbook content is frequently wrong in a way nobody
notices, so reading it would import somebody else's mistake and present it as our verdict.

**And every exclusion needs a reason recorded against it**, for the same reason a finding called
harmless does. Without that, this becomes the mechanism by which the blocked list is made to
look good.

## What proves it

The asymmetry is the load-bearing one, and it is pinned: anything unproven — a finding the
tool has never seen, a generic one from the underlying linter, a known one with no recorded
removal — [resolves to "somebody has to
decide"](internal/analysis/cop_classification_test.go#TestResolverReviewDefault) and never to
harmless. Harmless is only reached [for a positive structural
reason](internal/analysis/cop_classification_test.go#TestResolverStructuralNoise) — a
cosmetic formatting category, or a finding about test and build tooling rather than about the
code that runs on a machine.

A person's decision winning is pinned twice, [over everything the tool
concluded](internal/analysis/cop_classification_test.go#TestResolverOperatorOverrideWinsOverEverything)
including a finding the tool would have called breaking on recorded evidence, and
specifically [over a harmless
verdict](internal/analysis/cop_classification_test.go#TestResolverOperatorOverrideBeatsStructuralNoise).
Findings we have written ourselves, for things the underlying tool does not know about, [are
breaking by intent](internal/analysis/cop_classification_test.go#TestResolverCustomCop) —
there would be no reason to write one otherwise.

That a finding judged breaking actually blocks the cookbook rather than being recorded and
ignored is pinned by [the derivation
contract](internal/analysis/cookstyle_fullruleset_test.go#TestDeriveStatus_BlockerOutsideDepartments_Blocked).

That a finding outside cookbook code does not block, **and is still readable afterwards**, is
pinned by [one repository carrying the same breaking finding
twice](internal/analysis/cookstyle_scan_scope_test.go#TestRepositoryIsNotTheCookbook) — once in
cookbook code, once in a helper task, differing only in where they sit. The second half of that
test is what stops this being implemented as deletion, which would satisfy the first half and
lose the work. The same thing holds [when a verdict is worked out again
later](internal/analysis/cookstyle_scan_scope_test.go#TestRepositoryIsNotTheCookbook_SurvivesTheFingerprint)
from what was kept rather than from the findings themselves, and everything recorded before this
existed [still reads as it did](internal/analysis/cookstyle_scan_scope_test.go#TestFingerprintsWrittenBeforeScopeReDeriveUnchanged),
rather than a whole estate turning green on the day it shipped.

That the estate-wide count separates what blocks a cookbook from what is merely everywhere is
pinned [where the number is actually
read](internal/webapi/handle_cookstyle_cops_scope_test.go#TestCopAggregation_SplitsBlockingFromOutsideCookbookCode).
A repository is counted once, under the copy that decides its verdict.

That I can **disagree** with the list of ignored files is pinned the same way my decision over a
finding is. I can [name a file the shipped list never
could](internal/analysis/scan_scope_overrides_test.go#TestOperatorCanExcludeAFileTheSeedListCannotName)
— the script that only runs because a build job starts it, which sits somewhere different in every
estate and says nothing about itself — and I can [overturn a shipped
one](internal/analysis/scan_scope_overrides_test.go#TestOperatorCanDisagreeWithACuratedExclusion)
where it is simply wrong for us. Nothing takes effect [without a reason recorded against
it](internal/webapi/handle_cookstyle_scan_scope_test.go#TestScanScopePut_RequiresAReason), and [the
whole list is
readable](internal/webapi/handle_cookstyle_scan_scope_test.go#TestScanScopeList_ShowsCuratedAndOperatorEntriesTogether),
shipped entries beside local ones.

**Nothing proves the list of files we ignore is the right list.** Every entry [carries a recorded
reason](internal/analysis/cookstyle_scan_scope_test.go#TestScanScopeExclusionsAllCarryReasons) so
it can be argued with, but whether a reason is *true* is the same kind of curated claim as calling
a finding harmless, and it fails in the direction nobody reports: a file wrongly declared not to
run hides a real blocker, and nothing looks wrong.

**And the Chef server side still misses code that does run** but sits outside the places we look,
so its counts stay low in a way a reader cannot see.

**Nothing proves the knowledge itself is right.** The rules above decide what happens once a
finding is classified; whether the recorded evidence about a given finding is accurate is a
question about curated data, and a confidently wrong entry produces a confidently wrong red.
That is the failure this journey is most exposed to and no test here touches it.

**Nothing proves the worklist gets worked.** "Somebody has to decide" is only honest if
somebody does. If that pile is never triaged the tool has moved the problem rather than
solved it, and the list simply grows — visible to anyone who looks at it, asserted by
nothing.

**The load-bearing assumption:** that every finding reaching a person carries the reason for
its classification with it. Strip that provenance in some future change and this journey
collapses back to "the tool says so".
