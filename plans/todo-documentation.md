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

## A generated API description — the first surface where drift is made impossible

Raised 2026-08-03: the product owner was asked whether we have an API. We do, and it cannot
currently be described to anybody. This is also the cheapest place to prove the enforcement
idea above, because the route table is enumerable and the answer is checkable by a test.

**Measured 2026-08-03** (routes from `router.go`, paths from `journeys/*.md`, diffed
both ways) — the baseline to improve on:

| | |
|---|---|
| route patterns registered under `/api/v1` | 155 |
| registered but named in no spec | **48** |
| documented paths not served | 8 |
| — of those, genuinely wrong claims | **3** |

The 48 are not edge cases: the ownership import/alias/duplicate surface, saved filters, run
events, the kitchen queue, the failure register, SAML admin, seven dashboard endpoints. The
3 wrong ones are `/api/v1/admin/test-kitchen/config` (real path
`/api/v1/admin/config/test-kitchen`, asserted in two specs), `/api/v1/kitchen/git-run` (real
path `/api/v1/kitchen/git/run`) and `/api/v1/cookstyle/violations`, headed "New Endpoint"
and never built. There is **no OpenAPI document anywhere**, so nothing machine-readable and
nothing that generates a client.

- [ ] **Record the route table as it is registered.** `protect`, `adminOnly`, `operatorOnly`
  and their siblings are the single funnel every route passes through, so the pattern and
  the required role can be captured there. No source parsing, no new dependency, and the
  list cannot fall behind the routes because it *is* the routes.
- [ ] **A test that fails on drift, in both directions.** Every recorded route must appear
  in the OpenAPI document and every documented path must be a recorded route. This is the
  whole point: descriptions stay hand-written, but the set cannot rot, and a renamed path
  breaks the build instead of a customer's client.
- [ ] **Mind the prefix routes — this is where a naive version will under-deliver.** The mux
  registers subtrees (`/api/v1/git-repos/`) whose sub-paths are dispatched inside the
  handler by segment (`:name/committers/assign`, `:name/rescan`, `:name/:version/
  remediation`). Those endpoints are invisible to the mux, so route-set equality alone
  describes the subtree and not the endpoints. Each prefix needs its sub-paths declared
  where they are dispatched, or the count will look complete while a third of the real
  surface is still undescribed — the same failure, one level down.
- [ ] **Then fix the 3 wrong paths and delete or mark the proposals**, so the specs stop
  asserting endpoints that were never built.

**The auth question is answered, in the journey, not here.** There are no API keys or service
tokens. `RequireAuth` takes a session token from `Authorization: Bearer` or the cookie, and
sessions come from a user login — so an integration holds a person's credentials and
re-authenticates when the session expires. The product decision that was outstanding is settled
in [asking my assistant why this is failing](../journeys/agent-access.md): a per-person credential
issued from one's own record, carrying that person's level of access, read-only — not a service
account with a second permissions model.

That journey is what this section now serves, and its suite
(`internal/webapi/agent_access_journey_test.go`, run with `make journey`) is the live list of
what is outstanding. The four boxes above are the description work; the credential and the
assistant-facing surface are red tests there.
