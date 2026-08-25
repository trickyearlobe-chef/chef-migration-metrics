# What the machines already told us

**As the engineer running the upgrade, I need to know whether a cookbook actually ran on a
real machine at the target version, because a rule and a lab are both predictions and this
is the event.**

A static rule is a guess written before the fact, and a lab run is a guess in a place we
built. A converge that already happened on a real machine is the thing itself.

## What I need

To have a cookbook raise itself as a blocker when it converges cleanly at the version the
estate is on now and fails at the target version. That pairing is the whole case — the
cookbook works today and stops working at the target, seen on real machines — and somebody
transcribing it by hand is the part I want back.

To overrule it myself, exactly as I overrule anything else in the register — and when I
mark it good, for that to stand over the scan and the lab both, with the screen saying a
person decided rather than quietly showing a clean result.

To be told when one of these is worth another look: across the whole estate, every converge
at the target version we saw for it in the last stretch succeeded. That is a prompt to go and
check, not a verdict. Nothing clears itself.

To get the events wherever they come from. Some estates push them to us as they happen.
Others already send them somewhere that keeps them — Observe by Snowflake, here — and we go
and ask it on a schedule. I should not be able to tell which by looking at the answer.

## The decisions behind it

**Only the pairing raises a blocker.** A failure at the target version on its own could be a
cookbook that has been broken for a year. Clean at the current version and failing at the
target is what makes it evidence about the version.

**An observed failure cannot be blamed on our lab.** There is no lab. That is why this is
worth more than a converge we ran ourselves, and why it needs no judgement about whether the
lab can be trusted today.

**How long a stretch is depends on how often the estate converges**, so it is set, not fixed.
What must hold is that it covers at least one full cycle: a window shorter than that sees
whichever handful of machines happened to run, and a clean sweep of three nodes out of hundreds
is not worth interrupting anybody for.

**Nothing unblocks itself.** A quiet day is not a fixed cookbook — a cookbook nobody ran
looks exactly like one that started working. So a clean day raises a flag for somebody to
look at, and a person still decides. Getting this wrong would unblock an estate by
forgetting about it.

**A person's answer is the last word, over every automated one.** Marking a cookbook good
clears it whatever the scan said and whatever the lab said. This is not new here — it is the
same override that already exists, reached by a different route.

**Whether observed success may ever clear a blocker on its own is a switch, and it starts
off.** The other blocking rules ship on because their signals were trusted from the outset.
This one is not, yet: nobody has watched it against a real estate long enough to say a clean
run means what it appears to mean. Turning it on is somebody stating they have. Until then a
clean day is a prompt for a person and nothing more, and the path is left open rather than
closed — a signal that proves itself in practice should not need a redesign to be believed.

**A person's overrule is not reconsidered.** Once somebody has looked and said otherwise,
seeing the same failure again is not new information. Raising it a second time teaches
people to ignore the register.

**Where the events are kept is not the requirement.** Observe by Snowflake is where they are
today. What has to hold is that somewhere already holding them can be asked on a schedule —
a site that keeps them elsewhere is the same journey, not a new one. The same run must not
count twice because both routes saw it.

**Asking for them must not need a change to every machine.** The reason to fetch from
somewhere that already collects them is that it needs nothing deployed and no change control
on thousands of nodes.

## What proves it

That a failure survives being stored with enough of itself to attribute — which cookbook,
which version, what went wrong — is pinned by [a failure that goes in and comes back
whole](internal/datastore/converge_runs_functional_test.go#TestFunctional_ConvergeRuns_FailureRoundTrip),
and separating the runs that failed at a given version from the rest is pinned by [the
filtered
read](internal/datastore/converge_runs_functional_test.go#TestFunctional_ConvergeRuns_ListFiltered).

That an odd record does not cost us the whole delivery is pinned by [decoding
defensively](internal/ingest/normalise_test.go#TestNormalise_ResilientToWeirdFieldShapes),
which matters more for a source we query than one we own, because its shape is not ours.

That a person's "good" stands over a failing scan is pinned by [not-broken overriding a
failing scan](internal/analysis/readiness_human_verdict_test.go#TestHumanVerdict_NotBrokenOverridesAFailingScan),
and that it reaches the machines is pinned by [the node being
unblocked](internal/analysis/readiness_human_verdict_test.go#TestHumanVerdict_NotBrokenUnblocksTheNode).
That the screen says a person decided rather than showing a bare clean result is pinned by
[the repo being marked as
overruled](internal/webapi/handle_git_repos_test.go#TestGitRepos_MarksARepoAPersonHasOverruled).

**Nothing proves a quiet day means anything.** Every run succeeding in a day is only as
strong as what ran that day. It is why this is a prompt and not a verdict, and why the switch
above starts off: the evidence that would justify turning it on is a record of the prompt
being right, which can only be gathered by using it.

**Nothing proves the estate is a fair sample.** The machines that reached the target version
first are rarely the difficult ones.

**The load-bearing assumption:** that a failed run names the cookbook that failed, often
enough to attribute. A dependency that will not resolve fails before any cookbook runs, so
there is nothing to blame. Check what proportion arrive unattributable before building
anything that counts them — if it is large this is a much weaker instrument than it looks.
