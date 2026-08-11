# Unambiguous states over the API

From [building against this from the outside](../journeys/api-integration.md): a program reading a
state has to be able to tell the states apart. Pinned by
`TestJourney_StatesCanBeToldApart` in the journey suite.

## Two faults, only one of them everywhere

**A real state rewritten as blank** — the cookbook list turned "no git repository" into an empty
test state because the screen wanted a dash. Two distinct states arrived identical. One site, now
fixed; the dash is keyed on the state instead.

**A state that disappears when empty** — `omitempty` on a state field means "not known" is sent as
the field being absent, which a caller cannot tell from a version of the API that has no such
field. Fixed where the field is a state from a known set: cookbook, git-repo detail and role test
status, plus role compatibility, which was also being sent blank rather than as untested.

## Not the same fault — left alone deliberately

Absence meaning *not applicable* is legitimate and reads correctly:

- Dependency-graph nodes carry a test or compatibility state only where the node is that kind of
  thing. Sending an empty string for a role node would be less meaningful, not more.
- Disk encryption state is telemetry the platform may not report at all.
- Remediation breakdown items carry a state only for some kinds of item.

## Still open — each needs a decision, not a guess

An organisation that has never been collected has no collection state, and there is no defined
value for that. Same for a node's target-converge state where parallel deployment is not in use.
Making the field present without inventing a state only moves the ambiguity from "absent" to
"blank". Inventing one is a product decision.
