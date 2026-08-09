# Trusting what the scan tells me

**As the engineer running the upgrade, I need the scan to tell me apart the findings that
will actually break us from the ones that are just tidying, so that a red mark means
something and I keep reading them.**

The scanner reports hundreds of things per cookbook and it does not know which of them
matter for a version change. I have been burned by this before: I chased three reds that
turned out to be house-style preferences, and after that I stopped opening the list. The
tool did not become wrong at that point — it became useless, which is the same outcome
arrived at more slowly.

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
becomes somebody has to decide — never harmless.** Every unproven case sitting in a worklist
is the honest outcome, even though it makes the list longer.

**A person's decision outranks everything the tool worked out**, including its own confident
reds. The alternative is arguing with a machine that cannot hear me.

**One target version at a time.** Findings are judged per finding, not per version, because
there is only one version we are moving to. A per-version matrix was tried and removed.

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
collapses back to "the tool says so", which is where it started.
