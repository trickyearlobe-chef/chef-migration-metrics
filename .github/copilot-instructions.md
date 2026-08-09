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
- Do NOT add a `Co-authored-by: Copilot` trailer (or any AI authorship line) to
  commits or PRs — see the Commits rule in `/CLAUDE.md`. A `commit-msg` hook
  strips it as a backstop, but don't add it in the first place.
- To reduce context cost, generated/vendored paths (`node_modules`, `dist`,
  `build`, `embedded`, lockfiles, `migrations`) are excluded from editor
  indexing: VS Code via `.vscode/settings.json`, Zed via `.zed/settings.json`
  (`file_scan_exclusions`). The Copilot **CLI** enforces no local ignore file —
  it relies on `.gitignore` (which already covers the generated dirs), so do not
  read into those paths or the tracked-but-noisy ones (`migrations/`,
  `frontend/package-lock.json`) unless the task requires it.
- To find the right journey, start at `journeys/overview.md` (the routing
  index). Large specs are split into a thin `<spec>.md` index plus
  `<spec>-<section>.md` parts — open only the part you need, not a whole spec.
