# GitHub Copilot Instructions

The full project rules live in [`/CLAUDE.md`](../CLAUDE.md). It is the single
source of truth for both GitHub Copilot and Claude. **Read `/CLAUDE.md` and
follow every rule in it.** Do not duplicate those rules here — update `/CLAUDE.md`
instead, so both tools stay in sync.

Read `/CLAUDE.md` as "operating rules for the AI" — wherever it says "Claude",
the same applies to GitHub Copilot.

## Copilot-specific notes

- These are operating rules, not project documentation. Keep this file tiny — it
  is injected into every request.
- To reduce context cost, the workspace excludes generated/vendored paths
  (`node_modules`, `dist`, `build`, lockfiles, migrations) via
  `.vscode/settings.json`. Don't pull those into context unless asked.
- Specs in `specifications/` are large — open only the relevant section, never a
  whole spec file, unless the task needs it.
