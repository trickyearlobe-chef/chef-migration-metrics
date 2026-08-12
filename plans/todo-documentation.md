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

---

## Renderer research for the API document page (measured 2026-08-12)

Kept so a change of mind does not re-run the measurement. Dry-run installs
(`npm install --dry-run --ignore-scripts --no-save`) against the Harness proxy, from a
frontend tree of 371 packages. Nothing was installed.

| Package | Version measured | New transitive packages |
|---|---|---|
| `swagger-ui-react` | 5.32.13 | **+167** (+45% on the tree) |
| `rapidoc` | 9.3.8 | **+112** |
| `redoc` | 2.5.3 | **+97** |

**Chosen: hand-rolled React, zero new dependencies.** The document has no tags, no
components and `parameters: null` on every operation, so the detail a real renderer exists
to display is not there to display — the cost buys layout, not information.

If we circle back:

- **`redoc`** is the cheapest real renderer and has no try-it-out by default, which matches
  the decision already taken. Pulls mobx and styled-components.
- **`rapidoc`** is a web component, so it drops in without React bindings, but costs more
  than redoc and its theming is attribute-driven.
- **`swagger-ui-react`** is the familiar look and the most expensive. Try-it-out is on by
  default and must be switched off explicitly — the wrong default for a page served to a
  signed-in person.
- Any of them only pays off **after** the generator emits parameters, request bodies and
  response schemas. Until then they render the same three fields the hand-rolled page does.
- Re-measure before adopting: the counts move, and the Harness proxy quarantines very
  recent versions, so pin a slightly older release.

**The generator gap this exposed.** Every operation carries a summary and `x-required-role`,
and nothing else: `parameters` is null and the only response is a generic 200. That is thin
for the client generators the agent-access journey names as the reason for having a
description at all. Enriching the generator is the prerequisite for both a better page and a
usable generated client.

**Named types earn their keep beyond the description.** Lifting the 22 anonymous
`var body struct` declarations to named types is the prerequisite for reflecting request
schemas, but the same change unblocks rot detection generally — a named type can be snapshotted
and compared, which is exactly what
`TestJourney_TheShapeCannotChangeUnderACaller` (`internal/webapi/api_integration_journey_test.go`)
is skipped for today: nothing records what an answer looked like at a release, so nothing fails
when a field changes meaning. Do the lift once; it pays the description, the generated client and
the shape contract.
Do the lift with `sg` (`var body struct { $$$ }` is a shape, not a regex) and confirm call sites
with LSP `findReferences` rather than grep — the structs are anonymous, so text search cannot tell
one handler's body from another's.
