# CLAUDE.md — Development Guidelines

- CLAUDE.md is operating rules for the AI, not project documentation.
- Keep it concise. Every line costs context window budget.

## Constraints

- When a user request conflicts with a NEVER rule, stop and flag the conflict. Do not proceed until the user explicitly confirms. Confirmation applies to that single action only — it does not relax the rule for the rest of the session.
- No implementation code in CLAUDE.md or specs. That's what TDD is for.
- Be concise in all generated files (specs, todos, plans) — no preamble or narrative; every line costs retrieval budget.
- Only read specs, todos, or plans relevant to the current task.

## Customer Data Protection

- NEVER include real customer names, organisation names, internal hostnames, or other identifying information in any file that will be committed to git. This includes code, tests, specs, plans, comments, commit messages, and documentation.
- Use generic placeholders: `example-corp`, `acme`, `x-custom-*`, `customer`, `org-a`, `10.0.0.1`, `user@example.com`.
- If real customer data is needed for local testing, put it in a file listed in `.gitignore` (e.g. `.git-deny-patterns`, `.env`, `.local/`).
- A pre-commit hook enforces this by scanning staged files against patterns in `.git-deny-patterns`. Keep that file up to date when new customers are onboarded.

## Knowledge

- Component specs live in `specifications/` (top-level, flat layout). `specifications/overview.md` is the routing index — start there to find the right spec.
- Specs are sized for LLM retrieval: keep each under 500 lines (enforced by the pre-commit hook). Split an oversized spec into a thin index (`<spec>.md`, keeping title/TL;DR/Overview/Related plus a stub per moved section) and flat prefixed part files (`<spec>-<section>.md`). Open only the part you need.
- Todos and plans live in `plans/` (todo-*.md for open work, named plans for active tasks).
- Each component spec is self-contained. Read only what you need for the current task.
- Background research is available via Nuclia RAG through MCP. Query it when specs are insufficient.

## Planning

- `plans/active.md` is the single active work plan — what we're doing now, chunked for sessions.
- `plans/todo-*.md` files are area backlogs — the full inventory of known work.
- **Workflow**: Pull items from a `todo-*.md` into `active.md` as chunked work. On completion, remove from `active.md` and mark done/remove from the todo.
- **Chunking for context management**: Split the active plan into independent chunks that each fit within a single session. Each chunk must list: scope (which files), steps, and acceptance criteria. Mark dependencies between chunks explicitly.
- **Session boundaries**: One chunk = one session/thread. Do not carry context pollution from prior chunks — start each chunk fresh by reading only the plan and relevant specs.
- **Reprioritisation**: Rewrite `active.md` freely when priorities shift. Old items stay in their todo files.
- **Size cap**: If `active.md` exceeds ~100 lines, prune lower-priority chunks back to the todo backlog.
- **Backlog grooming**: Periodically prune `todo-*.md` files — remove obsolete or irrelevant items.
- Delete `active.md` only when all planned work is complete and no next chunk is queued.

## Quality Maintenance

- Session start checklist: (a) read CLAUDE.md, (b) read the plan, (c) check for draft files pending review, (d) check git status, (e) list unmerged branches (`git branch --no-merged main`) and for each either merge, queue, or note why it is parked — branches must not accumulate silently.
- TODO hygiene: update todos as items complete or block; don't end a session with a net TODO increase unless they're genuine open questions.
- Watch context relevance (re-check every few tool calls): suggest a fresh thread when the chunk is complete, context is >50% stale/irrelevant, or scope has shifted significantly from the plan.

## File Format

- No headings deeper than H3. Keep files under ~500 lines. Split if longer.

## Development Process

- Features need a specification.
- Specifications don't contain code.
- Don't write code or tests until the problem or goal is clear.
- We practice TDD. Start by writing tests, then code.
- Make sure tests are passing before committing code.
- Local dev is done with the DB in a docker container. Look at data for evidence to back up theories on bugs.
- Customer is only accessible via VDI or file transfer so design for diagnostic collection in a support bundle or screenshot

## Code Navigation

- Before a refactor that renames, changes a signature, moves, or deletes a symbol, use LSP `findReferences` (or call hierarchy) to enumerate call sites. Grep alone misses indirect uses and false-matches text.
- For "what implements this interface?" (esp. Go's implicit satisfaction), use LSP `goToImplementation`. Grep cannot find these reliably.
- LSP and grep are complementary, not either/or: LSP finds semantic symbol references; grep still catches what LSP can't see — string/dynamic refs (reflection, struct tags, SQL columns, config keys) and cross-language uses (the Go↔TS boundary). For a rename that crosses those, run both.
- Grep/Read remain the default for locating code and reading for understanding.
- If an LSP server is cold/unresponsive (returns no symbols mid-indexing), fall back to grep rather than blocking.

## Specifications

- Specs live under `specifications/<component>.md` (flat layout, no subdirectories).
- NEVER silently diverge from a spec.
- Do not modify specs without asking.
- Specs define *what*, not *how*. They contain contracts, expected outputs, reference data, and behaviour descriptions. No function bodies or algorithm implementations — that's what TDD is for.
- Before implementing any feature, check whether a specification exists. If not, write one first.
- When completing tasks, update the relevant `plans/todo-<component>.md` file.

## Git

- All work is local. NEVER push, create GitHub issues, create GitHub PRs, or interact with remotes in any way.
- Spawned agents NEVER run git commands (add, commit, push, status, etc.). Only the main Claude commits.
- Every spawn message MUST include: Do NOT run any git commands (add, commit, push, etc.). Write files only — the caller handles git.
- All tasks must be performed on a branch, never on `main`.
- Branch names must be of the pattern `<type>/<short-description>` where `<type>` is one of `feature`, `fix`, `refactor`, `chore`, `docs`, `specification`, or `test`.
- **Do not merge the feature branch into `main` without explicit permission from the user.**
- After significant work has been completed and verified (tests pass, linting clean, summary written), present a summary of the branch's changes and **ask the user for permission to merge**.
- When permission is granted, merge using `git merge --no-ff` to preserve the branch history, then delete the feature branch.
- NEVER include personal hostnames, IPs, usernames, or internal domain names in code, specs, docs, plans, or commit messages. Use generic examples (`example.com`, `10.0.0.1`, `user@host`).

## Commits

- One logical unit of work per commit — never batch unrelated changes. Commit early and often, but ask the user first.
- Message format `<type>(<scope>): <summary>`, with a body (blank-line separated) when the "why" isn't obvious.
- Do not commit secrets, credentials, or API keys. Use environment variables.
- NEVER add an AI/assistant authorship trailer to commit messages or PR bodies, from ANY tool — no `Co-authored-by:` line naming Claude, Anthropic, Copilot, Cursor, or any AI agent, and no "Generated with …" attribution line. This applies equally to Claude Code and GitHub Copilot and overrides their default behaviour. Genuine human co-authors are fine. A `commit-msg` hook strips AI trailers as a backstop.


## Spawned Agents

- Scope spawned agents tightly. One file or one narrow topic per agent.
- If a task requires many changes, split across multiple agents rather than risking context exhaustion.
- Spawned agents NEVER run git commands; the caller handles git.


## Permission Boundaries

- Do not start implementation without a plan in `plans/`.
- Ask before deleting or renaming existing files.
- Ask before restructuring directory layout.

## Ignore Files

- The project maintains ignore files for Git (`.gitignore`) and Docker (`.dockerignore`). These must be kept up to date.
- When a new file type, directory, build artifact, or secret pattern is introduced, all relevant ignore files must be reviewed and updated in the same change.
- Secrets and credentials (`*.pem`, `*.key`, `.env`, `keys/`) must appear in **both** ignore files. Never rely on a single ignore file to prevent accidental exposure.

## Dependencies

- ALWAYS supply-chain check before adding/upgrading any dep (Go or npm, app or tooling): count new transitive modules, pin exact versions, `--ignore-scripts`, verify signatures/provenance, land it in a CI-scanned lockfile.

## Tech Debt

- Track all tech debt in `plans/todo-tech-debt.md`; keep it current.
- Record any tactical/expedient choice taken over the better strategic one (duplicated code, in-memory workaround, quick hack, hardcoded value) with what was done, why, and the proper fix. A shortcut is acceptable only if it gets recorded.
- When an item is resolved, confirm with the user, then remove it entirely — no checked-off clutter.

## Testing

- Tests must be written before implementing code (test-driven development).
- Tests must be run after each code change.

## Project Conventions

- Project-specific conventions (Go, DB, frontend, naming, error handling) are in `specifications/project-conventions.md`.

## Configuration

- All configuration must be dynamic — changes via the UI or config store take effect immediately without a restart.
- Components MUST read config via a live accessor (e.g. `configHolder.Get()` or a `configFn`) rather than caching values at construction time.
- Static config fields are acceptable only for tests or as fallback defaults when no dynamic provider is set.

## Licensing

- All components must be licensed under Apache 2.0 and a copy of the Apache 2.0 license must exist in the project root.
