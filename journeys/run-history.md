# Seeing what actually happened on the machines

**As the engineer running the upgrade, I need to see the real converge history of the estate —
which machines are failing, on what, and for how long — because a prediction about a cookbook
is not the same thing as a machine that stopped working last night.**

Everything else here reasons about what *should* happen. This is the record of what did. It is
also the only thing that covers the parts of the estate we cannot reach: there are servers we
are not allowed to connect to, so nothing we do can go and ask them how they are. They can
still tell us, if we let them push.

## What I need

Machines first, runs second. I think in terms of "which servers are unhappy", and only then
"show me what that server has been doing". A flat list of every converge in the estate is
noise; the same information grouped by machine is a worklist.

To filter it the way the problem arrives — this organisation, failures only, this version of
Chef, this cookbook, since yesterday — and have the machine list answer "which machines have
*any* run matching this", rather than only those whose most recent run happens to match. A
server that failed four times last night and succeeded this morning is exactly the server I
want to see when I ask about failures.

When something failed, the actual error and where it came from, not a summary. If I have to
go and log into the machine to find out what happened, this has saved me nothing.

**Machines we cannot pull from, treated as first-class.** For a large part of this estate,
pushed telemetry is the *only* source. Those servers must not be second-class citizens
visible in a corner — if a machine is reporting, it is part of the estate.

## The decisions behind it

**A malformed field must not cost us the record.** Telemetry comes from software we do not
control, across versions we do not choose, and fields turn up as the wrong type — a count
arriving as a word, a list where an object was expected. Refusing such a record loses the
whole run, including the parts that were fine. So unrecognisable fields degrade to nothing
and the run is kept. The identity and outcome of a run are the only fields worth failing on.

**But a record we cannot parse at all is stored nowhere, not half-stored.** Partial rows are
worse than absence, because they read as data.

**Kept for a bounded time, then purged.** This is high-volume and it is history, not the
current state of anything. Nothing here is the authority on how a machine is configured; it is
the authority on what happened to it recently.

**Off unless somebody turns it on.** Receiving pushed data means exposing an endpoint, and an
endpoint that exists by default is an endpoint nobody decided to have. Until it is enabled it
is not merely idle — it is absent.

## What proves it

The defensive decode is pinned by exactly the case that motivated it: a record whose count
arrives as a word, whose cookbook list arrives as a number and whose error arrives as a
string [is kept, with the odd fields degraded to
empty](internal/ingest/normalise_test.go#TestNormalise_ResilientToWeirdFieldShapes) and the
run's identity, outcome and time intact. A record that cannot be parsed at all [persists
nothing](internal/webapi/handle_ingest_test.go#TestHandleIngest_MalformedPersistsNothing), and
a shape we do not recognise as a converge [creates no
rows](internal/webapi/handle_ingest_test.go#TestHandleIngest_IgnoredShapeNoRows) rather than an
empty one. Until it is switched on the endpoint [is not
there](internal/webapi/handle_ingest_test.go#TestHandleIngest_DisabledReturns404).

The machines-we-cannot-pull-from decision is pinned in two places, and it is the one most
likely to be undone by accident: the run list [reads the converge history directly, including
an organisation that has no collected machines at
all](internal/datastore/converge_runs_functional_test.go#TestFunctional_ConvergeRuns_ListFiltered),
and the organisations you can filter by [come from the history rather than from the list of
organisations we
collect](internal/datastore/converge_runs_functional_test.go#TestFunctional_ConvergeRuns_ListOrganisations).
Join those to the collected estate in some future change and this whole population silently
disappears from the view built for it.

"Any matching run, showing that run" is pinned by [the machine
rollup](internal/datastore/converge_runs_functional_test.go#TestFunctional_ConvergeRuns_NodeRollupExistsSemantics),
which is explicitly the case that separates it from "the machine's latest run happens to
match". Failure detail [survives storage and comes back
whole](internal/datastore/converge_runs_functional_test.go#TestFunctional_ConvergeRuns_FailureRoundTrip),
and duplicate deliveries of the same run [collapse, with old history
purged](internal/datastore/converge_runs_functional_test.go#TestFunctional_ConvergeRuns_UpsertDedupAndRetention).
Those need a real database and run under their own build tag rather than in the gating suite.

**A measured gap nothing here can fix.** Runs that fail while working out which cookbooks to
use — an unresolvable dependency, a cookbook that is not there — arrive without the error
detail that a failed converge carries. For that class the history will show a machine failing
and not say why. This was established by watching real traffic, and it is a property of what
the sending system chooses to include, so no test on our side can assert it away. Expect the
"actual error" promise above to be unmet for exactly those cases.

**Nothing proves the volume is survivable.** A cap exists and drops beyond it, and retention
purges; whether the settings are right for an estate this size is an operational question that
only real traffic answers.

**The load-bearing assumption:** that the identifier the sending system puts on a run is
stable, and unique enough to recognise the same run arriving twice. De-duplication and the
whole history depend on it. Verify it before changing how runs are keyed — if it is not
stable, duplicates accumulate silently and every count on this view is wrong upwards.
