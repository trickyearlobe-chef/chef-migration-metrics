# Documentation — ToDo

Feature, deployment, and configuration docs are covered by README.md, confluence-docs/, journeys/, and deploy/**/README.md. The only open doc task:

- [ ] Document contributing guidelines (no CONTRIBUTING file exists)

## Specs are requirements; contracts live in code and tests

Decision (2026-08-02). Specs rot because they **copy** contracts — table columns, config
keys, endpoint shapes — and nothing re-validates the copy. `ownership.enabled` survived in
six specs precisely because no test could ever fail over it.

- **Specs hold requirements**: user journeys, intent, invariants, decisions and their why —
  what no test can express. They **point at** the authoritative type or test; they never
  restate it.
- **Contracts live in code and tests**, where they compile, pass or fail.

`CLAUDE.md` already states the reference-never-copy rule; the existing spec set predates or
ignores it, and nothing enforces it. So the cleanup is mostly **deletion plus a pointer**,
not rewriting: the requirements underneath are usually still sound — that is what we found
in the ownership set, where the journeys survived and the mechanism claims did not.

Sequencing: highest-traffic specs first (the ones agents actually reach for). Enforcement —
contract tests, or a checker that greps asserted config keys and endpoints against the code
— is what makes it stick; without it the set rots again. Related: `plans/spec-drift-control.md`.

## The API description

Built. Served at `/api/v1/openapi.json`, generated from the route table, with the set held
from both directions by tests in `internal/webapi/router_routes_test.go`. The three endpoints
the old spec set asserted and never served went with that spec set; nothing claims them now.

**One decision worth not re-litigating:** the sub-paths under each subtree pattern are read
from the handler's dispatch, never from the `// /api/v1/...` comments above it. Those comments
were tried and are not a source of truth — several omit the bare detail case, and one named a
sub-path that had been removed.

**Known gap, nobody has decided it.** A feature switched off at runtime (run events, today)
still appears in the description, because the route is registered either way and the gate
lives inside the handler. An assistant reading the description will ask for it and get "no
such address" — the journey's exact worry about a tool that insists rather than correcting
itself. Either mark gated operations in the document or generate against live gate state.

**What is left is not description work.** It is the credential and the assistant-facing
surface, and the live list is the red tests in `internal/webapi/agent_access_journey_test.go`
(`make journey`).

**The auth question is answered, in the journey, not here.** There are no API keys or service
tokens. `RequireAuth` takes a session token from `Authorization: Bearer` or the cookie, and
sessions come from a user login — so an integration holds a person's credentials and
re-authenticates when the session expires. The product decision that was outstanding is settled
in [asking my assistant why this is failing](../journeys/agent-access.md): a per-person credential
issued from one's own record, carrying that person's level of access, read-only — not a service
account with a second permissions model. Whether that survives an unattended job is open, and
[building against this from the outside](../journeys/api-integration.md) says not to settle it
by building it.
