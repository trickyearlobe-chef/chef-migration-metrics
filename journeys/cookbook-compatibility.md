# Knowing whether a cookbook survives the new version

**As the engineer running the upgrade, I need one honest verdict per cookbook — will this
break on the target version — so that the list of things to fix is a list I can trust.**

The estate's cookbooks come from two places and I care about both the same way. Some are on
the Chef servers, uploaded and in use. Some are in git repositories, which is where they are
actually written and where a fix has to land. Asking "is this safe" should not depend on
which side I happen to be looking from, and a cookbook that looks fine on one side and
broken on the other is a question I need answered, not a discrepancy I have to notice.

## What I need to see

Per cookbook, whether it is safe, needs looking at, or is blocked — and what that verdict
was based on, because I will be asked to justify it and because I do not trust a verdict I
cannot see the reasoning for.

Two independent signals feed it. Static analysis reads the code and tells me about things
that were removed or deprecated. Actually converging the cookbook on a real machine tells me
whether it works. They disagree sometimes, and when they do I want to see both rather than
have one quietly overrule the other.

Which version, because the same cookbook at two versions is two different answers.

## What has to be true

**An untested cookbook is not a passing cookbook.** "We have no result" and "we have a
result and it is fine" must never render the same way. Most of the damage this tool can do
is telling me something is safe when nobody has actually looked.

**A verdict must not be poisoned by our own infrastructure.** When a converge test fails
because the lab could not build a machine, or could not get it an address, or could not log
in, that is our problem and not the cookbook's. Counting it against the cookbook produces a
blocked list full of things that are not broken, and a list I stop believing is worse than
no list — I will work around it, and then I will miss the real one.

**Nor by files that are not the cookbook.** A repository carries pipelines, helper tasks and test
suites that never run during a converge. A finding in one of those is not this cookbook breaking,
and when the same helper task sits in nearly every repository it makes nearly the whole estate
look broken. Same principle as the paragraph above, one level out — see [trusting what the scan
says](scan-trust.md), which holds the decisions.

**A person can overrule the machine.** If I have watched a cookbook converge successfully,
my verdict wins, and it is recorded as mine. The reverse too: if it passed here and broke in
production, I say so and that sticks.

## How I know it worked

The blocked list is short enough to work through, and when I pick something off it and go
look, it really is broken.

## What proves it

The three properties above are each pinned by a contract next to the code that decides the
verdict:

- An untested cookbook is not a passing cookbook — [no verdicts means
  unknown](internal/analysis/semantic_contracts_test.go#TestContract_CookstyleStatus_NoVerdictsIsUnknown),
  and a result too old to trust also means unknown rather than
  fine ([stale is unknown](internal/analysis/semantic_contracts_test.go#TestContract_CookstyleStatus_StaleIsUnknown)).
- A verdict is not poisoned by our own infrastructure — a converge failure on its own
  [does not make the cookbook
  failed](internal/analysis/semantic_contracts_test.go#TestContract_CookstyleStatus_OnlyTKFailureIsPassed).
- A person can overrule the machine, and it shows whose verdict it is — [the overrule
  marker](frontend/src/components/OverruledMarker.test.tsx).

**Nothing proves the closing claim** — that the blocked list is short enough to work
through and that what is on it really is broken. That is only answered by picking things
off it and going to look, which is why the infrastructure-failure rule above is the one to
protect: it is what keeps the list believable.
