# Journey suites — the todo list made of tests

Every journey is supposed to have one: a test per thing the journey says has to be in place,
run outside CI, where green means built and red means still to do. A todo list made of tests
cannot go stale, because running it recomputes it.

**It had drifted to one out of twenty before anybody measured it**, and it drifted silently
because the only thing enforced was that a journey *links* a test. Linking proves a specific
property. It never enumerates what is outstanding — so "what is left" lived in prose, and prose
is the thing this exists to replace.

Run `make journeys` to see the state, `make journey` to run the suites, `make journey-coverage`
for just the gaps. The drift now reports itself.

## The shape, from the one that exists

[ownership-intake](../journeys/ownership-intake.md) is the worked example — its suite is
`internal/webapi/ownership_intake_journey_test.go`.

- Build tag `journey`, out of the gating suite. Red is the normal state for most of a journey's
  life, and a red that blocks a release gets deleted.
- One test per line of the journey, quoting that line, so the reason outlives its author.
- Assert the real thing, so building the feature turns the test green with no edit to the test.
- A suite names its journey in a comment; that is how `make journey-coverage` finds it.
- `t.Skip` with a reason for anything that cannot be answered honestly from here yet — it says
  what is missing rather than pretending either way.
- **Never a parking space for regressions.** Something that used to work and now fails is a
  broken build. Parked here it becomes indistinguishable from an honest gap.

## Doing the other nineteen

One journey per sitting. It is a reading job, not a writing job: read the journey, turn each
"what I need" line into an assertion, run it, and let the result say what is true. Resist
adjusting the journey to match what the code does — that is the drift running backwards.

Expect two things on every one. Some lines will turn out already proven by tests in the ordinary
suite, and the journey suite should assert the behaviour at the seam rather than duplicate the
unit test. Others will turn out to be prose that nobody can test as written, which usually means
the journey line is vague rather than the code is missing — worth fixing the line, with the
owner.

Order suggested by how much is riding on them, not by size:

- scan-trust, cookbook-compatibility, remediation-priority — the verdicts everything else reads
- node-readiness, role-impact, estate-progress — the numbers quoted to a programme board
- ownership-attribution, ownership-identity, human-verdict, named-cohorts
- service-access, service-secrets, service-continuity, service-configuration, service-collection,
  service-diagnosis — the ones where a gap is an outage rather than a wrong number
- converge-testing, run-history, working-all-day

## Not decided

Whether a journey with no suite should eventually fail the pre-commit hook, the way a journey
naming no test already does. It would stop the drift returning, and it would also block every
commit touching the other nineteen journeys until each is done — so it is a decision for when
the number is small, not now.
