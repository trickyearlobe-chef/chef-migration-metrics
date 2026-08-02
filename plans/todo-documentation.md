# Documentation — ToDo

Feature, deployment, and configuration docs are covered by README.md, confluence-docs/, specifications/, and deploy/**/README.md. The only open doc task:

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
