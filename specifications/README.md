# Specifications — user journeys only

A specification here answers four questions, in the words the person would use:

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

## Why

Two rules explain the rest:

- **A contract is a test.** "X must equal Y", "the canonical function is", "these are the
  fields" — that belongs in an executable test next to the code that owns it, where it
  fails when it stops being true. If it cannot be written as a failing test, it was never
  a contract.
- **Code is the source of truth.** Anything written here that restates the code is a copy
  that starts rotting immediately, and a rotten copy is worse than no copy because it reads
  as authoritative.

The previous specifications are in `archive/specifications/` — historical, not
authoritative. Go/DB/frontend conventions are in `docs/project-conventions.md`.
