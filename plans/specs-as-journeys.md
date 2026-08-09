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

Enforced by `.githooks/pre-commit`, which has 21 tests of its own (`make test-hooks`).

## Where the old corpus went

Deleted. Tag **`specifications-retired-2026-08-04`** holds all 128 files and 20,338 lines, and
retrievability was re-verified against the tag immediately before deleting the working copy.
That tag is now the only route back, deliberately: a browsable copy gets read and believed.

Conventions moved to `docs/project-conventions.md` — they describe how we write code rather
than claiming what exists.

## Method, for whoever writes the next one

Check the capability exists in the tree, write the journey, then find the test — and **read the assertion, not the test's name.** Four claims
drafted from plausible-sounding test names turned out to be wrong when the bodies were read,
one of them exactly backwards. Finding the test is where a journey gets checked against
reality, which is the main reason the rule earns its keep.

Two more things the writing taught:

- **The archive lied about status as well as behaviour**, which is why reading it for the *why*
  is no longer part of the method and the tree is the only starting point. The retired
  failure-register spec carried "specified, not built" while the feature had shipped — page,
  handler, tests, migrations. Recover it from the tag only when its subject resurfaces, and
  check every claim in it against code.
- **Route counts mislead.** Chunk B's "8 routes" were 5: three were redirects to tabs.

## Next

**Plan files still cite retired specs — 22 paths that no longer resolve.** Every reference
outside `plans/` was repaired when the archive was deleted; the backlog was left, because
those files are being pruned anyway and fixing a reference in a plan that is about to be
deleted is wasted work. Repair them as each plan is next touched.

The count in this plan was wrong, which is worth knowing for the next estimate: it said 58
code comments, and 58 was the number in Go and TypeScript. The real figure was 58 plus the
front door — the README carried a ten-row index of retired specs, `CLAUDE.md` pointed at a
file this branch had already moved, and there were citations in migrations, packaging
scripts, the systemd unit, `nfpm.yaml`, `.gitignore` and the pre-commit hook's own comments.
A grep scoped to source files reported the tidy number.

## Then, worth considering

- **Nothing checks code→spec.** The hook resolves a journey's references to code; a code change
  that invalidates an untouched journey passes. That gap was demonstrated during this work —
  a corpus-wide check appeared to pass when it had in fact examined nothing, because staging an
  unmodified file stages nothing. A check that reads `specifications/` directly rather than the
  staged set would close it.
- **`plans/` still feeds planning the way the specs did** — 6,954 lines including 577 of tech
  debt. The cheap mechanical version is to require every file reference in a plan to resolve, so
  a stale enumeration fails the commit.

## Open, not decided

- **The check catches form, not substance.** It stops copied code and it now forces a
  resolving test link, but it does nothing about a claim written in plain English — including
  the "what nothing can prove" paragraph the convention asks for, which no check can tell
  from a silent omission.
- **A red test as a status signal has not been exercised.** Every chunk-A journey found an
  existing green test to name, so the separate build tag and make target for deliberately
  red journey tests was not needed and was not built. The first journey whose central
  property nothing asserts is where that gets built — the failure mode to avoid is linking
  the nearest passing test instead, because it then reads as proof.
- **Coverage is not mechanical, and route coverage is a floor rather than the measure.** All 48
  routes now map to a journey, checked by listing them against the index. The six capabilities
  with no route are covered only in prose inside journeys, which is the best that can be done —
  no navigation-driven sweep would ever have found them. A check could compute the route half of
  this; nothing can compute the other half.
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
