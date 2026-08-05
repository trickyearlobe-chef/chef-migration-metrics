# Specifications become user journeys

Why: the specifications had stopped being merely out of date and had started teaching
things that were false, and planning against them cost real time repeatedly. Traced
example — `configuration.md` described a YAML-file configuration model, which is the
opposite of the design, and that is why a file-based model kept being proposed. A spec
that is wrong is not passive.

Labelling them untrustworthy does not work. A "NOT TO BE TRUSTED" banner sat at the top of
the index and appears twice in CLAUDE.md, and they were planned from anyway. So the
mechanism has to be structural.

## The rules now in force

- **A specification is a user journey.** Who the person is, what they are trying to get
  done, what must be true for them to succeed, and how they would know it worked, in their
  words. No tables, columns, endpoints, paths, config keys or code.
- **A contract is a test.** Anything of the form "X must equal Y" belongs in a test that
  fails when it stops being true. If it cannot be written as a failing test it was never a
  contract.
- **`specifications/` records what was built.** Intent and backlog live in `plans/`. A
  journey only qualifies if the capability is reachable in the running app.
- **No status claims in prose.** Nothing says built, shipped, planned or proposed. The
  stalest line in the best document we had was its status line.
- **A journey may point at code or a test, but only as a markdown link**, because a link
  can be resolved. The target must exist and the optional `#fragment` must appear in it, so
  a reference that goes stale fails the commit that made it stale — whoever is doing that
  work fixes it while they still have the context.
- Each journey also states **the decisions that bind**, so they are not silently
  re-litigated, and **flags its own load-bearing assumption** for checking rather than
  reading as uniformly confident.

Enforced by `.githooks/pre-commit`, which has 13 tests of its own (`make test-hooks`).

## Where the old corpus went

Tag **`specifications-retired-2026-08-04`** on `a0abd66` holds all 128 files and 20,422
lines, verified retrievable. The working copy is at `archive/specifications/` and is
**due to be deleted** — it is browsable, and a future session will read it and believe it.
The tag is the only recovery path that requires deliberate effort.

Conventions moved to `docs/project-conventions.md` — they describe how we write code rather
than claiming what exists.

## Done

Chunk A — six journeys, 353 lines, replacing 4,453 across ~25 source specs: where the
estate stands, which machines can move, the one fix that unblocks thousands, whether a
cookbook survives, what to fix first, naming a slice of the estate. Written from the app's
routes and navigation, not from the archive, because drafting from the archive re-imports
the fiction.

## Next

**Chunks B, C, D — one session each.** Same method: check the capability exists, read the
archived spec only for the *why*, write the journey.

- **B — whether a cookbook is safe.** CookStyle and cop classification, Test Kitchen, the
  failure register, run events. 8 routes.
- **C — who does the work.** Ownership, import, aliases, duplicates, audit, committers.
  7 routes.
- **D — running the service.** Authentication and SSO, configuration, collection,
  credentials, backups, logging, system health, TLS, packaging, exports, diagnostics.
  20 routes, and most of the requirements nobody wrote down.

**Then:** delete `archive/specifications/`, and repair the 58 code comments citing spec
paths that the deletion breaks (20 cite `cop-classification.md`, 7 `event-ingest.md`,
6 `web-api-exports.md`, 5 each `failure-register.md` and `enriched-metric-snapshots.md`).

## Decided, and needs building

**A journey must name a test.** Agreed 2026-08-05. Enforced at commit time: the journey
must carry a resolving link to a test. A **red** test does not block a build — it reports
loudly, so redness means "this journey is not proven" and status becomes observable without
anyone writing it down. Follow the existing `//go:build functional` pattern with a separate
tag and its own make target, kept out of the gating suite.

Not everything is testable, and that is the risk to design around. Roughly two thirds of
the binding decisions in chunk A can carry a real assertion — a stale machine never reads
as passing, untested is not passing, a rule's classification, a saved selection the server
does not understand failing loudly. Some can only be checked through a stand-in (a chart
configured with a zero baseline; an icon carrying a shape as well as a colour). Some cannot
be tested at all — "the list is short enough to work through", and the decisions about what
we deliberately do not build.

**So the rule is: name a test for the parts that can be tested, let it be red if the
behaviour is not there, and say in the journey which parts nothing can prove.** One real
link satisfies the requirement. A journey with a test that passes without checking anything
would be worse than one with no test, because it would look proven.

## Open, not decided

- **The check catches form, not substance.** It stops copied code; it does nothing about a
  claim written in plain English. Naming a test closes part of this.
- **Coverage is not mechanical.** Answered by hand on 2026-08-04: 13 of 48 routes covered,
  and six capabilities with no route at all — certificates renewing themselves, collection
  on a schedule, receiving pushed telemetry, retention and purge, install and upgrade,
  anti-lockout server control. Those are the requirements held self-evident, and no
  navigation-driven sweep would ever find them. A check could compute this instead.
- **References are one-directional.** The hook resolves spec→code. Nothing checks code→spec.
- **`plans/` and the backlog feed planning the same way the specs did** — 6,954 lines
  including 577 of tech debt. The known-good rule from the abandoned branch: plans carry
  journeys and non-derivable gotchas, not enumerations. A cheap mechanical version is to
  require every file reference in a plan to resolve, so a stale enumeration fails the
  commit.
- **CLAUDE.md and the memories** carry the same disease, including three internal
  contradictions. The evidence gives a principle: the rules that hold here are the ones
  with hooks behind them, so a rule that has failed repeatedly and cannot be enforced
  should be deleted rather than restated again.
- **Field knowledge about systems we do not control** — roughly 60–80 diffuse callouts
  (Chef Server pagination, Ohai's filesystem shape on Windows, Test Kitchen lifecycle hook
  constraints, Windows build-number collisions, IdP metadata behaviour). Accepted loss for
  now, recoverable only from the tag. Most of it converts into a contract on our side of
  the boundary — "Automate drops a destination that does not answer 200–204" becomes a test
  that our endpoint always does. The interpretive residue has no home.
- **The three-layer split** (requirements / contracts / implementation) is parked. It fixes
  category error but not truth, and the layer count looked more like five: plans and
  operating rules are neither requirements, contracts nor implementation.

## Two habits worth keeping regardless of the above

**A plan must not become implementation written in prose.** The abandoned ownership branch
was 1,397 lines, all planning documents, no code. 45% of what was written was deleted again
within the same day, and 29% of the deleted text carried `file:line` citations. The
implementing agent re-derives that layer in minutes; meanwhile a markdown file has no
compiler, so a wrong table name in it is immortal while the same name in Go dies on the
first query. Precision without execution is what makes a false claim credible.

**An unanswered question must survive.** Four decisions on this work were taken by default
because a question went unanswered and nothing recorded that it was open — how aggressive
the check should be, complete versus as-we-go coverage, whether a journey must name a test,
and whether `plans/` was in scope. Three of those are above; the third was resolved on
2026-08-05. State the default taken, in writing, where it outlives the conversation.
