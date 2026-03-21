# Claude.md — Development Guidelines

This file contains the rules and conventions that must be followed at all times when working on this project. Read this file before doing anything else.

## Git Branching

- All tasks must be performed on a branch, never on `main`
- Branch names must be of the pattern `<type>/<short-description>` where `<type>` is one of `feature`, `fix`, `refactor`, `chore`, `docs`, `specification`, or `test`.
- All banching and merging happens locally, no PR's
- **Do not merge the feature branch into `main` without explicit permission from the user.**
- After significant work has been completed and verified (tests pass, linting clean, summary written), present a summary of the branch's changes and **ask the user for permission to merge**.
- When permission is granted, merge using `git merge --no-ff` to preserve the branch history, then delete the feature branch.
- If the user declines or wants changes first, continue working on the same branch.
- Do not squash commits when merging.

## Commits

- **Each completed todo or meaningful unit of work must result in its own commit.** Do not batch unrelated changes into a single commit.
- Commit only one logical unit of work at a time
- Split unrelated changes into separate commits.
- The commit message must follow `<type>(<scope>): <summary>` format
- Write clear, descriptive commit messages following conventional commit style
  - First line `<type>(<scope>): <summary>`
  - Include a body (separated by a blank line) when the "why" is not obvious from the summary.
- Commit early and often.



## Ignore Files

- The project maintains ignore files for Git (`.gitignore`), Docker (`.dockerignore`), and Helm (`.helmignore`). These must be kept up to date.
- When a new file type, directory, build artifact, or secret pattern is introduced, all relevant ignore files must be reviewed and updated in the same change.
- Secrets and credentials (`*.pem`, `*.key`, `.env`, `keys/`) must appear in **all** ignore files. Never rely on a single ignore file to prevent accidental exposure.

---

## Specifications

- Specs live under `.claude/specifications/<component>.md` (flat layout, no subdirectories).
- Before implementing any feature, check whether a specification exists. If not, write one first.
- When completing tasks, update the relevant `todo-<component>.md` file.

## Database

- All database schema changes must be managed through migration files. Migrations must be sequential, numbered, and checked into source control.
- The application must run pending migrations automatically on startup.
- Migrations must never be edited after they have been committed. Instead, create a new migration to make further changes.

## Language and Concurrency

- All backend components must be implemented in **Go**.
- Use **goroutines** to parallelise work wherever independent units of work can proceed concurrently.
- Use **channels** or **sync primitives** (e.g. `sync.WaitGroup`, `errgroup`) to coordinate goroutines and collect results or errors.
- Goroutine concurrency must be **bounded** using worker pools. Each task type (organisation collection, node page fetching, git pulls, CookStyle scans, Test Kitchen runs, readiness evaluation) has its own independently configurable worker pool size. See `.claude/specifications/configuration.md` for the concurrency configuration schema and default values.
- Each concurrent work unit must propagate errors back to the caller rather than silently discarding them.

## Testing

- Tests must be written before implementing code (test-driven development).
- Tests must be run after each code change.

## Go Package Layout

Key conventions:
- All Go code is a single module. Application packages live under `internal/`.
- Shared domain types live in `internal/models/`.
- Database queries are centralised in `internal/datastore/` — other packages must not import `database/sql` directly.
- HTTP handlers in `internal/webapi/` are thin wrappers — business logic lives in domain packages.
- Config structs live in `internal/config/` and are passed by value or interface — packages must not read config files or env vars directly.
- Test files sit alongside code (`foo_test.go` next to `foo.go`).
- Integration tests use build tags (`//go:build functional`) and are excluded from `go test ./...`.


## Frontend Conventions

- The React frontend lives in `frontend/` and is built with `npm run build` into `frontend/build/` (or `frontend/dist/`).
- The Go binary embeds the built frontend assets using `go:embed` and serves them from the web server.
- The frontend communicates exclusively through the Web API (`/api/v1/...`). It never accesses the database directly.

## Error Handling

- All exported functions that can fail must return `error` as the last return value. Do not panic for recoverable errors.
- Wrap errors with context using `fmt.Errorf("operation: %w", err)` so that callers can trace the failure path.
- Use sentinel errors (e.g. `var ErrNotFound = errors.New("not found")`) for conditions that callers need to check with `errors.Is()`.
- HTTP handlers must map domain errors to appropriate HTTP status codes in `internal/webapi/` — domain packages must not import `net/http`.
- Background jobs (collection, analysis, export) must log errors and continue processing remaining items. A single failing organisation, cookbook, or node must not abort the entire job.
- External process execution (CookStyle, Test Kitchen, git) must enforce timeouts, capture stderr, and return structured error information — not raw exec failures.

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

## Licensing

- All components must be licensed under Apache 2.0.
