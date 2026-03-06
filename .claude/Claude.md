# Claude.md — Development Guidelines

This file contains the rules and conventions that must be followed at all times when working on this project. Read this file before doing anything else.

---

## Token Economy

- **Do NOT read all specification files upfront.** Only read the spec(s) relevant to the current task.
- Use `./Structure.md` to orient yourself and find spec file names. **Do not read `specifications/Specification.md`** unless you need the project overview for a new contributor.
- **Prefer reading specific line ranges** over entire files. Use file outlines first, then read only the sections you need.
- Large spec files have a **TL;DR** at the top. Read that first to decide whether you need the full file.
- `specifications/ToDo.md` is a progress table only. Authoritative tasks live in `specifications/todo/*.md` — read only the one relevant to your work.
- After completing tasks, update the done/total counts in `ToDo.md` (use the regeneration script in that file).
- Use the **task-to-spec lookup** below to find which specs and todo files to load. Do not load specs not listed for your task. `(§ Section)` means read only that section.

### Task-to-Spec Lookup

| Task area | Read spec | Todo file |
|-----------|-----------|-----------|
| Chef API client | `chef-api/` | `todo/data-collection.md` |
| Node collection, cookbook fetching, role graph | `data-collection/` | `todo/data-collection.md` |
| Usage analysis, CookStyle, Test Kitchen, readiness, remediation | `analysis/` (relevant §) | `todo/analysis.md` |
| Database schema / migrations | `datastore/` | `todo/project-setup.md` |
| Configuration / TLS | `configuration/`, `tls/` | `todo/configuration.md` |
| Web API endpoints | `web-api/` | `todo/visualisation.md` |
| Dashboard frontend | `visualisation/` | `todo/visualisation.md` |
| Auth (local/LDAP/SAML) | `auth/` | `todo/auth.md` |
| Logging | `logging/` | `todo/logging.md` |
| Elasticsearch / NDJSON | `elasticsearch/` | `todo/visualisation.md` |
| Packaging (RPM/DEB/container/Helm/Compose/CI) | `packaging/` (relevant §) | `todo/packaging.md` |
| Notifications / exports | `web-api/` (relevant §), `configuration/` (§ Notifications) | `todo/visualisation.md` |
| Secrets / credentials | `secrets-storage/` (relevant §) | `todo/secrets-storage.md` |

---

## Orientation

- Read `./Structure.md` first to understand the layout of the project before exploring files or making changes.
- `./Structure.md` must be updated in the same change whenever a file or directory is added, moved, renamed, or removed.

---

## Task Summaries

- At the end of every task (feature, bug fix, test addition, refactor, etc.), write a summary file in `.claude/summaries/`.
- **Naming convention:** `YYYY-<component>-<short-description>.md` (e.g. `2025-secrets-rotation-tests.md`).
- **Purpose:** Give future threads enough context to continue work without re-reading code or re-running tests. Include what was done, what the final state is (test counts, coverage, passing/failing), any known gaps, and which files were modified.
- **Minimum contents:**
  1. **Context** — what component/area was involved and why.
  2. **What was done** — specific changes, with enough detail to understand scope.
  3. **Final state** — test counts, coverage numbers, pass/fail status, any warnings.
  4. **Known gaps** — anything deliberately left uncovered or deferred.
  5. **Files modified** — list of files touched (production code and tests separately).
  6. **Recommended next steps** — prioritised list of what to work on next, with reasoning. Include which specs and todo files to read, and the approximate scope of each step. This avoids future threads spending tokens rediscovering what to do.
- Keep summaries concise but complete — a new thread should be able to read the summary and pick up where you left off without re-exploring the codebase.
- **At the start of a new thread**, before exploring the codebase, read **only the most recent summary** in `.claude/summaries/` (sorted by filename date prefix). If it has a "Recommended next steps" section, use that as your starting point instead of re-analysing the project from scratch. **Do not read other summaries unless you need context on a specific component you are about to modify.**
- Before starting work on a component, check `.claude/summaries/` for existing summaries related to that component. This avoids duplicating investigation effort.
- Update `./Structure.md` when adding a new summary file (add the entry to the `summaries/` listing).

### Single Source of Truth for Next Steps

- The **most recent summary file's "Recommended next steps" section** is the single source of truth for what to work on next.
- **Do NOT duplicate next-step plans into `specifications/ToDo.md`.** That file is a progress-table index only — no narratives, milestones, token estimates, or session plans.
- When finishing a task, put the full next-steps plan in the new summary file and nowhere else.

### Archiving Old Summaries

- Only recent summaries belong in `.claude/summaries/`. Older summaries live in `.claude/summaries/archive/`.
- **When to archive:** After writing a new summary, if there are more than **8 files** in `.claude/summaries/` (excluding `archive/`), move the oldest summaries to `archive/` to keep the count at 8 or fewer.
- **Do not read archived summaries** at thread start. They exist for historical reference only. If you need context on an old component, you may read a specific archived summary on demand.
- Update `./Structure.md` when archiving summaries.

### ToDo.md Hygiene

- `specifications/ToDo.md` must contain **only** the progress table and the count-regeneration script. No milestones, no narratives, no token estimates, no session plans, no phase breakdowns.
- After completing tasks, update the done/total counts in the progress table. Do not add any other content.

### Context Budget and Incremental Saves

Large tasks risk exhausting the context window before a summary can be written. Follow these rules to prevent losing work:

- **Write the summary early and update it incrementally.** Create the summary file as soon as meaningful progress has been made (e.g. after the first milestone, not at the very end). Update it as you go. A partial summary saved is infinitely better than a perfect summary lost to a context limit.
- **Break large tasks into checkpoints.** If a task involves multiple distinct subtasks (e.g. "add tests for function A, then B, then C"), save the summary after completing each subtask. The summary should always reflect the current state, not just the planned end state.
- **Prefer multiple small commits over one large commit.** When a task touches many files or adds many tests, pause at natural boundaries to update the summary. This also makes it easier for a new thread to pick up mid-task if the current thread ends unexpectedly.
- **If you sense the conversation is getting long**, proactively save the summary with what you have so far, noting any remaining work under "Known gaps" or a "Remaining work" section. Do not wait for the user to ask.
- **If the user's request is large**, tell them your plan, note how you intend to checkpoint, and save the first summary after the first checkpoint — before continuing to the next phase.

---

## Ignore Files

- The project maintains ignore files for Git (`.gitignore`), Docker (`.dockerignore`), and Helm (`.helmignore`). These must be kept up to date.
- When a new file type, directory, build artifact, or secret pattern is introduced, all relevant ignore files must be reviewed and updated in the same change.
- `.gitignore` — excludes build output, secrets, IDE files, OS metadata, runtime data, and test artifacts from version control.
- `.dockerignore` — excludes everything not needed by the Dockerfile build context (secrets, deploy artifacts, documentation, IDE files, stale build output). Keeping this tight reduces build context size and prevents secrets leaking to the Docker daemon.
- `.helmignore` (at `deploy/helm/chef-migration-metrics/.helmignore`) — excludes files that should not be packaged into the Helm chart archive.
- Secrets and credentials (`*.pem`, `*.key`, `.env`, `keys/`) must appear in **all** ignore files. Never rely on a single ignore file to prevent accidental exposure.

---

## Specifications

- Specs live under `.claude/specifications/<component>/Specification.md`.
- Before implementing any feature, check whether a specification exists. If not, write one first.
- When completing tasks, update the relevant `todo/*.md` file and refresh the counts in `ToDo.md`.

---

## Chef Infra Server API

- All Chef Infra Server API calls must conform to the project API specification at `./specifications/chef-api/Specification.md`.
- The upstream API reference is at https://docs.chef.io/server/api_chef_server.
- `./specifications/chef-api/Specification.md` must be updated when API bugs or unexpected behaviours are discovered.
- Do not use external libraries (e.g. `mixlib-authentication`) for Chef API signing. Signing must be implemented natively.

---

## External Tool Output

- When the application shells out to external tools (git, CookStyle, Test Kitchen, Docker, etc.) as part of batch processes, the invocation **must** use flags that produce JSON or machine-parseable output wherever the tool supports it. This makes parsing robust, locale-independent, and resilient to future output format changes.
- Preferred output formats, in order: **JSON** (`--format json`, `--format '{{json .}}'`), **porcelain** (`--porcelain`), **NUL-delimited** (`-z`), **explicit format strings** (`--format='...'`). Fall back to line-oriented text parsing only when no structured option exists.
- Never parse a tool's human-readable/colourised terminal output. Always suppress colour (`--no-color`, `NO_COLOR=1`) and progress indicators (`--quiet`) when capturing output programmatically.
- Specific per-tool guidance lives in the component specifications: `analysis/Specification.md` (CookStyle, Test Kitchen, Docker) and `data-collection/Specification.md` (git). Those specs are authoritative — this section states the general principle.

---

## Database

- All database schema changes must be managed through migration files. Migrations must be sequential, numbered, and checked into source control.
- The application must run pending migrations automatically on startup.
- Migrations must never be edited after they have been committed. Instead, create a new migration to make further changes.

---

## Language and Concurrency

- All backend components must be implemented in **Go**.
- Use **goroutines** to parallelise work wherever independent units of work can proceed concurrently. Key examples:
  - Collecting node data from multiple Chef server organisations in parallel
  - Pulling multiple cookbook git repositories in parallel
  - Running CookStyle scans across multiple cookbook versions in parallel
  - Running Test Kitchen tests across multiple cookbooks and/or target Chef Client versions in parallel
- Use **channels** or **sync primitives** (e.g. `sync.WaitGroup`, `errgroup`) to coordinate goroutines and collect results or errors.
- Goroutine concurrency must be **bounded** using worker pools. Each task type (organisation collection, node page fetching, git pulls, CookStyle scans, Test Kitchen runs, readiness evaluation) has its own independently configurable worker pool size. See `./specifications/configuration/Specification.md` for the concurrency configuration schema and default values.
- Each concurrent work unit must propagate errors back to the caller rather than silently discarding them.

---

## Testing

- Tests must be written before implementing code (test-driven development).

---

## Go Package Layout

See `./Structure.md` for the full directory tree. Key conventions:
- All Go code is a single module. Application packages live under `internal/`.
- Shared domain types live in `internal/models/`.
- Database queries are centralised in `internal/datastore/` — other packages must not import `database/sql` directly.
- HTTP handlers in `internal/webapi/` are thin wrappers — business logic lives in domain packages.
- Config structs live in `internal/config/` and are passed by value or interface — packages must not read config files or env vars directly.
- Test files sit alongside code (`foo_test.go` next to `foo.go`).
- Integration tests use build tags (`//go:build functional`) and are excluded from `go test ./...`.

---

## Frontend Conventions

- The React frontend lives in `frontend/` and is built with `npm run build` into `frontend/build/` (or `frontend/dist/`).
- The Go binary embeds the built frontend assets using `go:embed` and serves them from the web server.
- The frontend communicates exclusively through the Web API (`/api/v1/...`). It never accesses the database directly.

---

## Error Handling

- All exported functions that can fail must return `error` as the last return value. Do not panic for recoverable errors.
- Wrap errors with context using `fmt.Errorf("operation: %w", err)` so that callers can trace the failure path.
- Use sentinel errors (e.g. `var ErrNotFound = errors.New("not found")`) for conditions that callers need to check with `errors.Is()`.
- HTTP handlers must map domain errors to appropriate HTTP status codes in `internal/webapi/` — domain packages must not import `net/http`.
- Background jobs (collection, analysis, export) must log errors and continue processing remaining items. A single failing organisation, cookbook, or node must not abort the entire job.
- External process execution (CookStyle, Test Kitchen, git) must enforce timeouts, capture stderr, and return structured error information — not raw exec failures.

---

## Naming Conventions

- **Go packages**: lowercase, single-word where possible (`chefapi`, `webapi`, `datastore`), matching the directory name.
- **Go files**: `snake_case.go` (e.g. `partial_search.go`, `readiness_evaluator.go`). Test files: `*_test.go`.
- **Go types**: `PascalCase` — use domain nouns (`NodeSnapshot`, `CookbookVersion`, `ReadinessResult`), not generic names (`Data`, `Item`, `Record`).
- **Go interfaces**: name by capability, not by `I` prefix (`Collector`, `Authenticator`, `Exporter`). Single-method interfaces use the `-er` suffix.
- **Database tables**: `snake_case`, plural (`node_snapshots`, `cookbook_versions`, `readiness_results`).
- **Database columns**: `snake_case` (`chef_version`, `policy_name`, `is_stale`, `created_at`).
- **Migration files**: `NNNN_short_description.up.sql` / `NNNN_short_description.down.sql` (e.g. `0001_create_node_snapshots.up.sql`).
- **API endpoints**: kebab-case paths under `/api/v1/` (e.g. `/api/v1/dependency-graph`, `/api/v1/cookbook-compatibility`).
- **Configuration keys**: `snake_case` in YAML (e.g. `stale_node_threshold_days`, `embedded_bin_dir`).
- **Environment variable overrides**: `SCREAMING_SNAKE_CASE` with `CMM_` prefix (e.g. `CMM_DATABASE_URL`, `CMM_SMTP_PASSWORD`).

---

## Licensing

- All components must be licensed under Apache 2.0.