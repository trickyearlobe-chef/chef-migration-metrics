# Saying so when the machine is wrong

**As the engineer who has actually watched a cookbook run, I need somewhere to record what I
know, so that my knowledge outlives the conversation and the tool stops arguing with me.**

The automated signals are wrong in both directions and I am the only thing that can correct
them. A cookbook is marked as blocking and it has been running in production for eight
months. Another one passes everything we have and then breaks on a real machine for a reason
no rule and no test predicted. Today the first costs somebody a wasted day, and the second
gets rediscovered by the next person, because there is nowhere to write either of them down.

## What I need

To record a verdict on a cookbook in either direction — this is not actually broken, or this
is broken and you missed it — with the reason, and have it stick.

For my verdict to outrank whatever the tool concluded, and to carry my name, so it is a
judgement somebody made and not an anonymous override that nobody can question later.

For it to reach the estate, not just sit in a list. If I say something is broken, the machines
running it stop reading as ready. That is the point: I am not annotating a report, I am
correcting the answer.

To read the whole set back in one go every morning — what is broken, why, who is on it, and
whether the pile is growing — because that is a standup, and if it takes twelve clicks it will
not happen.

To close one out when it is fixed, and have that recorded too, so the list is a live picture
rather than an archive of everything anybody ever complained about.

## The decisions behind it

**A verdict without a reason is not accepted.** Not a warning, a refusal. An unexplained
override is indistinguishable from a mistake three weeks later, and the person who has to
judge it will not be me.

**A verdict is about a cookbook, not about one version of it.** Naming a version is refused
rather than quietly ignored, and the refusal says why. Versions come and go weekly; a verdict
pinned to one would evaporate on the next release and quietly stop protecting anything, which
is worse than never having recorded it.

**Recording the verdict is the transaction; propagating it is not.** If updating the wider
picture fails, my verdict is still saved. Losing what a person took the trouble to tell us
because a downstream recalculation timed out would teach everyone to stop bothering.

**This is the correction layer, and it is deliberately at the end.** Everything else in the
product predicts. This records what somebody has seen. Where they disagree, what somebody has
seen wins.

## What proves it

Both directions are pinned: a person's "broken" [overrides a clean
scan](internal/analysis/readiness_human_verdict_test.go#TestHumanVerdict_BrokenOverridesACleanScan),
a person's "not broken" [overrides a failing
one](internal/analysis/readiness_human_verdict_test.go#TestHumanVerdict_NotBrokenOverridesAFailingScan),
and a verdict can be recorded [on a cookbook nothing has ever
tested](internal/analysis/readiness_human_verdict_test.go#TestHumanVerdict_BrokenOnAnUntestedCookbook) —
which is the case that matters most, because a cookbook nobody has scanned is exactly where a
person's knowledge is all there is.

That it reaches the estate rather than staying in a list is pinned in both directions too: a
broken verdict [blocks the machines running
it](internal/analysis/readiness_human_verdict_test.go#TestHumanVerdict_BlocksTheNodesRunningIt),
and a not-broken verdict [unblocks
them](internal/analysis/readiness_human_verdict_test.go#TestHumanVerdict_NotBrokenUnblocksTheNode).
An empty register [changes
nothing](internal/analysis/readiness_human_verdict_test.go#TestHumanVerdict_NoRegisterAtAll), so
the feature cannot alter answers until somebody actually uses it.

The refusals are pinned: [no reason, no
verdict](internal/webapi/handle_failure_register_test.go#TestRecordFailure_RefusesAVerdictWithNoReason),
and [naming a version is
refused](internal/webapi/handle_failure_register_test.go#TestRecordFailure_RefusesAVersion) with
a message that says why rather than a bare rejection. The verdict [survives a failed
recalculation](internal/webapi/handle_failure_register_test.go#TestRecordFailure_SurvivesAFailedRecompute)
and is [recorded with an audit
entry](internal/webapi/handle_failure_register_test.go#TestRecordFailure_RecordsTheVerdictAndAudits),
as is [closing one
out](internal/webapi/handle_failure_register_test.go#TestResolveFailure_RecordsWhoAndAudits). The
morning read is [one request, not one per
entry](internal/webapi/handle_failure_register_test.go#TestListFailureRegister_TheStandupView),
and [an empty register is still a
list](internal/webapi/handle_failure_register_test.go#TestListFailureRegister_EmptyIsAList) rather
than an error, because a good morning should not look like a fault.

**Nothing proves the verdicts are true.** This journey records human judgement and gives it
the last word; whether that judgement was right is not a testable property. The protection is
procedural — a name and a reason attached to every one, so a wrong call can be found and
argued with.

**Nothing proves the pile gets worked through.** A register that only grows is a list of
things nobody fixed, and it would look identical to a healthy one at every point except size.

**The load-bearing assumption:** that a verdict is keyed on something stable enough to still
match the cookbook months later. It is deliberately not keyed on a version for that reason,
but it is keyed on a name — and if repository or cookbook naming is ever re-keyed, standing
verdicts silently stop applying. That failure is invisible: the estate simply goes back to
believing the machine. Verify the keying before changing anything about how these are
identified.
