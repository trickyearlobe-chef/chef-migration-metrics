# Keeping the data flowing

**As the administrator running this service, I need it to keep collecting from the Chef servers
on its own, and to tell me when it has stopped, because every number in this product is only as
current as the last successful collection and a stale number looks exactly like a fresh one.**

This is the quiet failure that undermines everything else. If collection stops, nothing breaks
visibly — the screens still work, the charts still draw, the totals still add up. They are just
answers about last week. A migration lead can take a stale figure to a board and never know.

## What I need

Collection running on a schedule I set, against the Chef servers I have configured, without
anybody starting it.

To add or remove a Chef server, and to have work in flight against one that is going away not
break everything else.

To know when it last succeeded, how long it took, and what it found — per server, not as one
number for the whole estate, because one unreachable server out of four is a different problem
from all four being down.

For a server that is failing to stop being retried into the ground, and to be told rather than
having it retried silently for a week.

For collection not to take the machine down with it. This runs on somebody's infrastructure, and
a tool that exhausts the disk or the memory of its host has done more damage than the problem it
was solving.

To bound how much runs at once, because the Chef servers we read from belong to somebody else
and are not ours to overload.

## The decisions behind it

**A schedule that cannot be parsed is refused when it is set, not when it should have fired.**
The alternative is a schedule that silently never runs, which is indistinguishable from a service
that is working until somebody checks a date.

**Collection is per Chef server and its failures are per server.** One organisation being
unreachable must not stop the others, and must not present as a total failure.

**The service watches its own host and will stop collecting to protect it.** Disk, processor and
memory are checked against thresholds, and there is a level at which the right thing to do is
stop. A tool that keeps working while filling a disk is choosing its own completeness over the
machine it is a guest on.

**How much runs at once is a setting, and it applies globally rather than per job.** The
constraint being managed is the load placed on other people's systems, and that load is the total,
not any one batch of work.

## What proves it

Schedules are pinned across both the valid and invalid cases — [expressions that
parse](internal/collector/scheduler_test.go#TestParseSchedule_ValidExpressions), [expressions that
are refused](internal/collector/scheduler_test.go#TestParseSchedule_InvalidExpressions) — and the
firing times are pinned concretely, including the ones people get wrong: [hourly at the top of the
hour](internal/collector/scheduler_test.go#TestCronSchedule_Next_HourlyAtTopOfHour), [every fifteen
minutes](internal/collector/scheduler_test.go#TestCronSchedule_Next_Every15Minutes), [daily at
midnight](internal/collector/scheduler_test.go#TestCronSchedule_Next_DailyAtMidnight) and
[weekdays only](internal/collector/scheduler_test.go#TestCronSchedule_Next_WeekdaysOnly).

Protecting the host is pinned at both levels of severity, for each resource: disk [warning and
critical](internal/syshealth/syshealth_test.go#TestEvaluateAlerts_DiskCritical), processor
[warning](internal/syshealth/syshealth_test.go#TestEvaluateAlerts_CPUWarning) and
[critical](internal/syshealth/syshealth_test.go#TestEvaluateAlerts_CPUCritical), and memory
[warning](internal/syshealth/syshealth_test.go#TestEvaluateAlerts_MemoryWarning). That the
measurements themselves are real rather than placeholders is pinned by [the
snapshot](internal/syshealth/syshealth_test.go#TestSnapshot_ReturnsValidStats), with [disk
figures](internal/syshealth/syshealth_test.go#TestSnapshot_DiskMetrics_Populated) and [memory
figures](internal/syshealth/syshealth_test.go#TestSnapshot_MemoryMetrics_Populated) populated.

Bounding concurrent work is pinned where it reads other people's systems — [readiness evaluation
is bounded](internal/analysis/readiness_test.go#TestEvaluateOrganisation_ConcurrencyBounded) — and
the bound is [read live rather than fixed at
startup](internal/analysis/readiness_test.go#TestReadinessEvaluator_EffectiveConcurrency_LiveOverride),
as is [the scanner's](internal/analysis/cookstyle_test.go#TestCookstyleScanner_EffectiveConcurrency_LiveOverride).
Reading a Chef server's inventory across pages is pinned [over multiple
pages](internal/chefapi/client_test.go#TestCollectAllNodesConcurrent_MultiplePages), [when a later
page fails](internal/chefapi/client_test.go#TestCollectAllNodesConcurrent_ErrorOnSubsequentPage) —
so a partial read is not mistaken for a complete one — and [when the work is
cancelled](internal/chefapi/client_test.go#TestCollectAllNodesConcurrent_ContextCancelled).

**Nothing proves the thing this journey is most afraid of.** No test asserts that a stale estate is
presented as stale rather than as current. The staleness of individual machines is handled — see
[which machines can move](node-readiness.md) — but "collection last succeeded four days ago and
every figure on this screen is from then" is a property of the screens, and it is asserted nowhere.
That is the silent failure the journey opens with, and it is the least defended.

**Nothing proves the service stops before it damages the host.** The thresholds are evaluated and
alerts are produced; that collection actually halts as a result is not covered by anything here.
Verify the behaviour before relying on it in somebody else's estate.

**The load-bearing assumption:** that a collection which fails part way through does not leave
figures that read as complete. If a partial collection is stored the same way as a full one, every
guarantee in this journey is cosmetic — the numbers would be confidently wrong downwards, and
nothing would say so.
