# CLAUDE.md — Development Guidelines

- CLAUDE.md is operating rules for the AI, not project documentation.
- Keep it concise. Every line costs context window budget.

## Constraints

- When a user request conflicts with a NEVER rule, stop and flag the conflict. Do not proceed until the user explicitly confirms. Confirmation applies to that single action only — it does not relax the rule for the rest of the session.
- No implementation code in CLAUDE.md or specs. That's what TDD is for.
- Always be concise and NEVER include preamble or narrative in generated files.
- Only read specs, todos, or plans relevant to the current task.
- Be concise when creating or updating specs and todos so tokens are not wasted retrieving context.

## Knowledge

- Component specs live in `specifications/` (top-level, flat layout, one file per component).
- Todos and plans live in `plans/` (todo-*.md for open work, named plans for active tasks).
- Each component spec is self-contained. Read only what you need for the current task.
- Background research is available via Nuclia RAG through MCP. Query it when specs are insufficient.

## Planning

- Work plans live in `plans/`. One file per task or feature.
- Before starting work, create a plan in `plans/<task>.md`.
- Plans are short: goal, which specs to read, ordered steps, and acceptance criteria.
- Delete the plan when the work is done. Git is the history.
- **Chunking for context management**: Split plans into independent chunks that each fit within a single session. Each chunk must list: scope (which files), steps, and acceptance criteria. Mark dependencies between chunks explicitly.
- **Session boundaries**: One chunk = one session/thread. Do not carry context pollution from prior chunks — start each chunk fresh by reading only the plan and relevant specs.

## Quality Maintenance

- Session start checklist: (a) read CLAUDE.md, (b) read the plan, (c) check for draft files pending review, (d) check git status.
- **Context pollution checks**: After every 3-4 tool calls, assess whether accumulated context is still relevant. If the conversation has drifted into debugging a tangent or contains large blocks of superseded output, suggest the user start a fresh thread for the next chunk.
- **Signal a fresh thread** when: (a) the current chunk is complete, (b) context is >50% stale/irrelevant, or (c) the task has shifted scope significantly from the original plan.
- Always update todos when items are completed or blocked to avoid losing context.

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

- **Each completed todo or meaningful unit of work must result in its own commit.** Do not batch unrelated changes into a single commit.
- Commit only one logical unit of work at a time.
- Split unrelated changes into separate commits.
- Commit early and often, but ask the user first.
- The commit message must follow `<type>(<scope>): <summary>` format.
- Write clear, descriptive commit messages following conventional commit style:
  - First line `<type>(<scope>): <summary>`
  - Include a body (separated by a blank line) when the "why" is not obvious from the summary.
- Do not commit secrets, credentials, or API keys. Use environment variables.


## Spawned Agents

- Scope spawned agents tightly. One file or one narrow topic per agent.
- If a task requires many changes, split across multiple agents rather than risking context exhaustion.
- Every spawn message MUST include: Do NOT use the console for file operations.


## Permission Boundaries

- Do not start implementation without a plan in `plans/`.
- Ask before deleting or renaming existing files.
- Ask before restructuring directory layout.

## Ignore Files

- The project maintains ignore files for Git (`.gitignore`) and Docker (`.dockerignore`). These must be kept up to date.
- When a new file type, directory, build artifact, or secret pattern is introduced, all relevant ignore files must be reviewed and updated in the same change.
- Secrets and credentials (`*.pem`, `*.key`, `.env`, `keys/`) must appear in **both** ignore files. Never rely on a single ignore file to prevent accidental exposure.

## Tech Debt

- Technical debt is tracked in `plans/todo-tech-debt.md`. This file must be kept up to date.
- When a **tactical decision** is made where a different **strategic decision** would be better long-term (e.g. duplicating code instead of extracting a shared component, using an in-memory workaround instead of a proper SQL query), add an entry to the tech debt list explaining what was done, why, and what the strategic fix would be.
- When a **problem is fixed in an ugly or expedient way** that needs future refactoring (e.g. a quick hack to unblock progress, a workaround for a library limitation, a hardcoded value that should be configurable), add it to the tech debt list with enough context for someone to come back and do it properly.
- When a tech debt item is **resolved**, ask the user for confirmation, then **remove it from the list entirely**. Do not leave checked-off items cluttering the file.
- Do not let tech debt accumulate silently — if you notice something that smells wrong but fixing it properly is out of scope for the current task, the trade-off is acceptable **only if it gets recorded** in the tech debt list.

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
