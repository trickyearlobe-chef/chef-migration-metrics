# Specifications — user journeys only

A specification here answers four questions, in the words the person would use:

- Who is this person and what are they trying to get done?
- What must be true for them to succeed?
- What do they see or do?
- How would they know it worked?

That is all. **No tables, columns, endpoints, packages, function names, file paths, config
keys, or code.** A pre-commit check enforces this.

Two rules explain why:

- **A contract is a test.** "X must equal Y", "the canonical function is", "these are the
  fields" — that belongs in an executable test next to the code that owns it, where it
  fails when it stops being true. If it cannot be written as a failing test, it was never
  a contract.
- **Code is the source of truth.** Anything written here that restates the code is a copy
  that starts rotting immediately, and a rotten copy is worse than no copy because it reads
  as authoritative.

The previous specifications are in `archive/specifications/` — historical, not
authoritative. Go/DB/frontend conventions are in `docs/project-conventions.md`.
