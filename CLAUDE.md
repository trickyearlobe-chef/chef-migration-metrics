# CLAUDE.md — Development Guidelines

- CLAUDE.md is operating rules for the AI, not project documentation.
- Keep it concise. Every line costs context window budget.

## Constraints

- When a user request conflicts with a NEVER rule, stop and flag the conflict. Do not proceed until the user explicitly confirms. Confirmation applies to that single action only — it does not relax the rule for the rest of the session.
- No implementation code in CLAUDE.md or journeys. That's what TDD is for.
- Be concise in all generated files (journeys, todos, plans) — no preamble or narrative; every line costs retrieval budget.
- Only read journeys, todos, or plans relevant to the current task.

## Customer Data Protection

- NEVER include real customer names, organisation names, internal hostnames, or other identifying information in any file that will be committed to git. This includes code, tests, journeys, plans, comments, commit messages, and documentation.
- Use generic placeholders: `example-corp`, `acme`, `x-custom-*`, `customer`, `org-a`, `10.0.0.1`, `user@example.com`.
- If real customer data is needed for local testing, put it in a file listed in `.gitignore` (e.g. `.git-deny-patterns`, `.env`, `.local/`).
- A pre-commit hook enforces this by scanning staged files against patterns in `.git-deny-patterns`. Keep that file up to date when new customers are onboarded.

## Knowledge

- Journeys live in `journeys/` (top-level, flat layout). `journeys/overview.md` is the routing index — start there. Rules for writing one are under Journeys below.
- Todos and plans live in `plans/` (todo-*.md for open work, named plans for active tasks).
- Each journey is self-contained. Read only what you need for the current task.
- Background research is available via Nuclia RAG through MCP. Query it when the journeys are insufficient.

## Planning

- `plans/active.md` is the single active work plan — what we're doing now, chunked for sessions.
- `plans/todo-*.md` files are area backlogs — the full inventory of known work.
- **"What's next" starts with `make journey`, and the candidates are the reds.** They are the only
  backlog that is recomputed rather than remembered, so where they and the prose plans disagree,
  the reds are right. Read them alongside `active.md`, which still decides priority when it says
  anything; the reds decide what is actually outstanding.
  - A **skip** is not a candidate — its message names what is blocking it, and that blocker is
    usually the real next task.
  - A red that used to be green is a **regression, not a candidate**. It is a broken build, and
    picking it up as work-to-do quietly re-implements something that was already built. Nothing
    records which were green before, so this one is judgement.
- **Workflow**: Pull items from a `todo-*.md` into `active.md` as chunked work. On completion, remove from `active.md` and mark done/remove from the todo.
- **Done lives in code, not prose**: completed work leaves the plan entirely — "done-ness" is git history + passing tests, never re-asserted in prose. Status checkboxes and "DONE/merged" notes rot and cause stale-audit drift. Record *decisions* (the why) in the journey itself or a short decisions note; never leave *status* claims that nothing re-validates.
- **Chunking for context management**: Split the active plan into independent chunks that each fit within a single session. Each chunk must list: scope (which files), steps, and acceptance criteria. Mark dependencies between chunks explicitly.
- **Session boundaries**: One chunk = one session/thread. Do not carry context pollution from prior chunks — start each chunk fresh by reading only the plan and relevant journeys.
- **Reprioritisation**: Rewrite `active.md` freely when priorities shift. Old items stay in their todo files.
- **Size cap**: If `active.md` exceeds ~100 lines, prune lower-priority chunks back to the todo backlog.
- **Backlog grooming**: Periodically prune `todo-*.md` files — remove obsolete or irrelevant items.
- Delete `active.md` only when all planned work is complete and no next chunk is queued.

## Quality Maintenance

- Session start checklist: (a) read CLAUDE.md, (b) read the plan, (c) check for draft files pending review, (d) check git status, (e) list unmerged branches (`git branch --no-merged main | grep -v '^  abandoned/'`) and for each either merge, queue, or abandon it — branches must not accumulate silently. `abandoned/*` is excluded by that filter: skip it, do not report it, do not re-ask. (f) run `make journey-status` — one line, ~12s. Report it only if something moved: a count that went the wrong way is a regression, and a regression is a broken build, not a task. Do not print the full list unless asked; `make journey` is for choosing work.
- TODO hygiene: update todos as items complete or block; don't end a session with a net TODO increase unless they're genuine open questions.
- Verify before you record: any claim about code state or completion must be checked against the tree at the current commit and cite `file:line @ <short-SHA>`. Never record a status/audit claim from memory or stale context — re-read the code first. Stale audits come from recording claims that were never re-verified.
- Watch context relevance (re-check every few tool calls): suggest a fresh thread when the chunk is complete, context is >50% stale/irrelevant, or scope has shifted significantly from the plan.

## File Format

- No headings deeper than H3. Keep files under ~500 lines. Split if longer.

## Development Process

- Features need a journey.
- Journeys don't contain code.
- Don't write code or tests until the problem or goal is clear.
- We practice TDD. Start by writing tests, then code.
- Make sure tests are passing before committing code.
- Local dev is done with the DB in a docker container. Look at data for evidence to back up theories on bugs.
- Customer is only accessible via VDI or file transfer so design for diagnostic collection in a support bundle or screenshot

## Code Navigation

Three complementary layers — pick the cheapest that answers the question, escalate when it can't:

- **grep/ripgrep (text)** — the default for locating code, reading for understanding, and finding what only text can see: string/dynamic refs (reflection, struct tags, SQL columns, config keys) and cross-language uses (the Go↔TS boundary). Fast, zero setup.
- **ast-grep / `sg` (structural)** — reach for it the moment a regex starts catching matches inside comments, strings, or the wrong syntactic position. Matches by AST, so it isolates code shapes grep can't (e.g. `if err != nil { return err }` wrap-candidates instead of every `err != nil` line). Use it for structural search and mechanical multi-site rewrites. Language-aware: pass `-l go` / `-l tsx`. Pattern gotcha: a lone `$$$` as the *only* argument does not bind — anchor it with a concrete metavar (`fmt.Errorf($MSG, $$$)`, not `fmt.Errorf($$$)`). `sg` is a middle layer, not a grep replacement — the patterns take a little learning.
- **LSP (semantic)** — the authority for symbol meaning. Before a refactor that renames, changes a signature, moves, or deletes a symbol, use `findReferences` (or call hierarchy) to enumerate call sites — grep/`sg` miss indirect uses. For "what implements this interface?" (esp. Go's implicit satisfaction), use `goToImplementation` — text/structural search cannot find these reliably.

- Escalate, don't substitute: text → structural when regex fights the grammar; structural → semantic when you need meaning (references, implementations, types). For a rename that crosses dynamic/string refs or the Go↔TS boundary, run LSP **and** grep — neither sees the other's world.
- If an LSP server is cold/unresponsive (returns no symbols mid-indexing), fall back to `sg`/grep rather than blocking.

## Token Efficiency

- Delegate read/search fan-out to subagents (see Spawned Agents) — the biggest lever for keeping the main context clean.
- Preserve the prompt cache: batch related work in one session so the stable prefix (CLAUDE.md, journeys, tool schemas) is reused instead of re-paid across many short chats.
- Be specific up front (`file.go:42 nil check is wrong` beats `fix the bug`) so tokens go to action, not exploration.

## Journeys

`journeys/` holds user journeys. It replaced a corpus of component specifications that taught
things that were false; the directory was renamed so the old habit has no home to return to.

- **A journey is written in the person's words.** Who they are, what they are trying to get
  done, what must be true for them to succeed, and how they would know it worked. No tables,
  columns, endpoints, paths, config keys or code. The pre-commit hook enforces this.
- **The code is the only source of truth.** A contract is a test that fails when it stops being
  true, living next to the code it constrains. A journey may point at one, but only as a
  markdown link, because a link can be checked — and the link is checked at commit time.
- **Every journey names at least one test**, and says which parts nothing can prove. Both are
  the convention; only the first is enforceable.
- **Every journey needs a suite**: `*_journey_test.go`, build tag `journey`, naming its journey in
  a comment. One test per thing the journey says must be in place, quoting that line. Green means
  built, red means still to do — so it is the journey's todo list, and running it recomputes the
  list rather than asking anyone to keep one true. `make journeys` / `make journey` /
  `make journey-coverage`. Most journeys do not have one yet; `make journey-coverage` says which.
  That a journey HAS a suite is checked at commit and in CI. `make journey-ratchet` holds the
  whole directory against a grandfathered list, failing both when a journey outside it has no
  suite and when one on it gains a suite that has not been struck off — so the number only ever
  goes down. It never looks at whether a suite passes.
- **Before implementing any part of a journey, check its suite exists AND still covers it.**
  `make journey-coverage` answers the first. The second is a read: go through the journey line by
  line and confirm each thing it says must be true has a test. Journeys get edited and suites do
  not follow, so a suite that was complete when written may now be a subset — and a subset reads
  as the full list, which is worse than nothing.
  - **No suite: writing the whole suite is the first task**, before any implementation. All red or
    skipped to begin with, which is correct — red is the todo list.
  - **Suite short of the journey: close the gap first**, same rule. Never start implementing
    against a list you already know is incomplete.
  - Completeness is a judgement nothing can check, which is exactly why it is done deliberately
    and up front rather than grown a test at a time alongside the code.
- **Incompleteness never blocks a release.** The suite is outside the gating suite and must stay
  there — `make journey` is not part of `make ci`. Red is the normal state for most of a journey's
  life. A red that blocks a release gets deleted, and then the list is gone. It is never where a
  regression is parked: something that used to work and now fails is a broken build.
- **A green one stays.** Implementing something turns its test green; nothing is removed at that
  point. The green is the proof it was built and that we were happy with the result, and the suite
  as a whole becomes the feature inventory — what this product does, enumerated and runnable,
  which is what you would want in hand if the debt ever justified starting again. The suite only
  accumulates.
- **Closing a gap means writing the test, not reporting it.** Read the journey, add a test per
  requirement, `t.Skip` with a reason where it cannot be answered honestly yet.
- **No status claims.** Nothing says built, shipped, planned or proposed. A red test means "not
  proven" — that is the status mechanism, and it needs no maintenance.
- **Verify before writing.** Never state a behaviour you have not just checked in the tree, and
  read a test's assertions rather than trusting its name. Both mistakes have been made here.
- Journeys live flat in `journeys/`, under 200 lines each (hook-enforced). `journeys/overview.md`
  is the routing index. Never split one into part files — if it needs more room it is two
  journeys, or the detail belongs in a test.
- Do not modify a journey without asking. Never silently diverge from one.
- Before implementing a feature, check whether a journey covers it. If not, write one first.
- The 128 retired specifications were deleted on 2026-08-09 and live only in the tag
  `specifications-retired-2026-08-04`. Do not restore a browsable copy — that is what the tag
  is protecting against. Read from it only when its subject resurfaces, and check every claim
  against code.

- When completing tasks, update the relevant `plans/todo-<component>.md` file.

## Git

- All work is local. NEVER push, create GitHub issues, create GitHub PRs, or interact with remotes in any way.
- Spawned agents NEVER run git commands (add, commit, push, status, etc.). Only the main Claude commits.
- Every spawn message MUST include: Do NOT run any git commands (add, commit, push, etc.). Write files only — the caller handles git.
- All tasks must be performed on a branch, never on `main`.
- Branch names must be of the pattern `<type>/<short-description>` where `<type>` is one of `feature`, `fix`, `refactor`, `chore`, `docs`, `specification`, `test`, or `abandoned`.
- `abandoned/*` is a terminal state: work we stopped but kept so it can be mined later for design or learnings. Rename a branch to `abandoned/<short-description>` when it is dropped. These branches are settled — never merge them, never delete them, never propose resuming them, and never raise them unprompted. Read one only when its subject resurfaces in new work.
- **Do not merge the feature branch into `main` without explicit permission from the user.**
- After significant work has been completed and verified (tests pass, linting clean, summary written), present a summary of the branch's changes and **ask the user for permission to merge**.
- When permission is granted, merge using `git merge --no-ff` to preserve the branch history, then delete the feature branch.
- NEVER include personal hostnames, IPs, usernames, or internal domain names in code, journeys, docs, plans, or commit messages. Use generic examples (`example.com`, `10.0.0.1`, `user@host`).

## Commits

- One logical unit of work per commit — never batch unrelated changes. Commit early and often, but ask the user first.
- Message format `<type>(<scope>): <summary>`, with a body (blank-line separated) when the "why" isn't obvious.
- Do not commit secrets, credentials, or API keys. Use environment variables.
- NEVER add an AI/assistant authorship trailer to commit messages or PR bodies, from ANY tool — no `Co-authored-by:` line naming Claude, Anthropic, Copilot, Cursor, or any AI agent, and no "Generated with …" attribution line. This applies equally to Claude Code and GitHub Copilot and overrides their default behaviour. Genuine human co-authors are fine. A `commit-msg` hook strips AI trailers as a backstop.


## Spawned Agents

- Prefer subagents for read/search fan-out — the largest token lever. A subagent reads files in a separate context and returns a short summary; the file dumps never enter the main conversation. Use it whenever answering means sweeping many files and you only want the conclusion.
- Model-tier the work: run exploration/search subagents on a cheaper model, reserve the top model for reasoning and edits.
- **Agents locate; the caller verifies.** Never act on a subagent's conclusion. Finding a candidate costs an agent twenty reads; confirming it costs the caller one command — run it. Yesterday's failures came from using agent conclusions directly.
- **Every load-bearing claim carries its evidence** — the command run, and `file:line @ <short-SHA>`. A claim that cannot carry evidence is a lead, not a fact.
- **Negative claims are the dangerous class.** "There is no X" from an agent that searched the wrong term is indistinguishable from true absence. A negative must state the terms searched and the paths covered, or it is worthless.
- **Ask for facts, not judgements.** "Which files write this field?" not "is this design sound?" Agents fill gaps by inferring intent, and the inference is invisible in the output.
- **Do not merge several agents' partial findings into one narrative.** Keep claims attributed and separate — the seams are where the errors hide, and the merged story always reads better than its sources.
- Never run read-only agents concurrently with writers on the same files: true-when-read, false-when-used.
- Scope spawned agents tightly. One file or one narrow topic per agent.
- If a task requires many changes, split across multiple agents rather than risking context exhaustion.
- Spawned agents NEVER run git commands; the caller handles git.


## Reporting to the User

- **The user is not the verification layer for internal technical claims.** They cannot audit volume, and they should not have to. If a claim matters, encode it as a **test that fails** — never as a sentence for a human to check.
- **Division of verification:** tests verify facts; the assistant verifies agent claims; **the user verifies intent** — whether the work matches the requirement. That is the only check nobody else can perform, so protect their attention for it.
- **Write findings, plans and progress in plain language**: what a user would see, or what breaks. Internal shorthand ("false derive of gin index ordinality") is unreviewable — it asks the reader to audit internals in the assistant's vocabulary. Plain statement first; the technical term only if it earns its place.
- Report only what changes a decision. Volume defeats checking, and skimming is indistinguishable from verifying until it isn't.

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

- **A todo or a debt item is a failing test wherever one can be written**, outside the gating
  suite — the same mechanism as journeys. It does not block `make ci` or a release, and it
  reads as not-done until it is done. Prose says a thing is outstanding; a red test proves it
  still is, and goes green by itself when somebody fixes it.
  - Belongs to a journey → its journey suite (`*_journey_test.go`, tag `journey`).
  - Belongs to nothing → `*_debt_test.go`, tag `debt`, `make debt`.
  - Cannot be expressed as a test (a decision, a purchase, an access request) → prose in the
    todo file, saying why no test can hold it.
- **Write the test so it fails for the reason it names.** Assert the baseline first where the
  behaviour needs one, or a red goes green when something unrelated changes and the item is
  silently lost. This has happened.
- Track all tech debt in `plans/todo-tech-debt.md`; keep it current, and point each item at the
  test that holds it.
- Record any tactical/expedient choice taken over the better strategic one (duplicated code, in-memory workaround, quick hack, hardcoded value) with what was done, why, and the proper fix. A shortcut is acceptable only if it gets recorded.
- When an item is resolved, confirm with the user, then remove it entirely — no checked-off clutter.

## Testing

- Tests must be written before implementing code (test-driven development).
- Tests must be run after each code change.

## Project Conventions

- Project-specific conventions (Go, DB, frontend, naming, error handling) are in `docs/project-conventions.md`.

## Configuration

- Configuration lives in the DB and is edited via the UI. The only legitimate exceptions are the values that unlock the DB itself: the database URL and the credential encryption key. Server/TLS env overrides also exist in code but are NOT an exception to aspire to — TLS lockout is already handled by the in-code fallback ladder (self-signed, then plain HTTP), so they are legacy.
- Never propose an env var or `config.yaml` as a way to set anything else. A `yaml:` struct tag is not evidence a setting is usable — check the config store. If a setting is unreachable, the options are wire it into the store or delete it.
- All configuration must be dynamic — changes via the UI or config store take effect immediately without a restart.
- Components MUST read config via a live accessor (e.g. `configHolder.Get()` or a `configFn`) rather than caching values at construction time.
- Static config fields are acceptable only for tests or as fallback defaults when no dynamic provider is set.

## Licensing

- All components must be licensed under Apache 2.0 and a copy of the Apache 2.0 license must exist in the project root.
