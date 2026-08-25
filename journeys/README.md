# User journeys

A journey here answers four questions, in the words the person would use:

- Who is this person and what are they trying to get done?
- What must be true for them to succeed?
- What do they see or do?
- How would they know it worked?

That is all. **No tables, columns, endpoints, packages, function names, file paths, config
keys, or code.** A pre-commit check enforces this.

## Pointing at a contract

A journey may say where a rule is pinned, but only as a markdown link, because a link can
be checked:

    the verdict is pinned by [the derivation contract](internal/analysis/semantic_contracts_test.go#TestContract_CookstyleStatus_StaleIsUnknown)

The file must exist and the optional `#fragment` must appear in it, or the commit is
blocked. So a reference that goes stale fails the commit that made it stale, and gets
fixed by the person doing that work, while they still have the context — rather than by an
audit nobody runs. Bare paths and symbols in prose stay blocked: nothing can resolve them,
so nothing can tell you when they stop being true.

Links into `archive/` are blocked. That material is historical.

## Naming a test is required

Every journey must link at least one test, and the commit is blocked otherwise. This is
what replaces a status line. Nothing here says built or shipped — the journey names the test
that proves it, and if that test is red the journey is not proven, which is visible without
anybody maintaining a sentence that claims otherwise.

**Name a test that already exists, next to the code it pins, wherever one does.** A contract
belongs beside its code, and moving it somewhere labelled "journeys" would be a second home for
the same assertion.

A property with **no** test needs the other answer: a journey names a red one, and red means
"not proven yet" rather than a broken build. That is what a journey suite is for — one per
journey, under its own build tag, outside the gating suite, one test per thing the journey says
must be in place. Running it recomputes the todo list instead of asking anybody to keep one
true. Writing a journey whose central property nothing asserts is the point to add a red test
there, not to link the closest passing one and let it read as proof.

**One resolving link satisfies the rule.** It is deliberately not a coverage rule: much of
what a journey promises cannot carry an assertion, and a rule demanding a test per claim gets
met with tests that assert nothing — worse than no test, because it looks proven.

**So each journey also says which parts nothing can prove.** That part is not enforced,
because no check can tell an honest admission from a silent omission. It is the half of the
convention that depends on the writer.

## Why

Two rules explain the rest:

- **A contract is a test.** "X must equal Y", "the canonical function is", "these are the
  fields" — that belongs in an executable test next to the code that owns it, where it
  fails when it stops being true. If it cannot be written as a failing test, it was never
  a contract.
- **Code is the source of truth.** Anything written here that restates the code is a copy
  that starts rotting immediately, and a rotten copy is worse than no copy because it reads
  as authoritative.

The specifications these replaced were deleted deliberately, and are recoverable only from the
tag `specifications-retired-2026-08-04`. A browsable copy gets read and believed, which is what
the tag is protecting against: they asserted tables, endpoints and config flags that did not
exist, and carried status claims nothing re-validated.

Go, database and frontend conventions are in `docs/project-conventions.md`.
