# Journey suites — the todo list made of tests

Every journey is supposed to have one: a test per thing the journey says has to be in place,
run outside CI, where green means built and red means still to do. A todo list made of tests
cannot go stale, because running it recomputes it.

**How many are done is not written here.** `make journeys` says, `make journey-coverage` names
the ones with nothing, and `make journey` runs what exists. A count written into this file would
be wrong the first time somebody landed a suite and would be believed anyway — which is the exact
failure the suites exist to prevent, so it is not going to be committed in the plan for them.

It drifted for a long time because the only thing enforced was that a journey *links* a test.
Linking proves a specific property; it never enumerates what is outstanding. So "what is left"
lived in prose. That is now blocked at commit time and again in CI.

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

## Doing the rest

One journey per sitting. It is a reading job, not a writing job: read the journey, turn each
"what I need" line into an assertion, run it, and let the result say what is true. Resist
adjusting the journey to match what the code does — that is the drift running backwards.

Expect two things on every one. Some lines will turn out already proven by tests in the ordinary
suite, and the journey suite should assert the behaviour at the seam rather than duplicate the
unit test. Others will turn out to be prose that nobody can test as written, which usually means
the journey line is vague rather than the code is missing — worth fixing the line, with the
owner.

Order suggested by how much is riding on them, not by size. This is a priority, not a todo —
`make journey-coverage` is what says which are still outstanding:

- scan-trust, cookbook-compatibility, remediation-priority — the verdicts everything else reads
- node-readiness, role-impact, estate-progress — the numbers quoted to a programme board
- ownership-attribution, ownership-identity, human-verdict, named-cohorts
- service-access, service-secrets, service-continuity, service-configuration, service-collection,
  service-diagnosis — the ones where a gap is an outage rather than a wrong number
- converge-testing, run-history, working-all-day

## Decided

A journey with no suite fails the pre-commit hook, and fails CI if it changed in a push. Both
scoped to the journey being touched: that pressure is the point, whereas a gate over the whole
directory would have been red on the day it landed and switched off the week after — which is
how the rule was lost the first time.
