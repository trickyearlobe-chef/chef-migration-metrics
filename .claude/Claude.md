# Claude.md — Development Guidelines

This file contains the rules and conventions that must be followed at all times when working on this project. Read this file before doing anything else.

---

## Token Economy

- **Do NOT read all specification files upfront.** Only read the specific spec(s) relevant to the current task.
- Use `./Structure.md` to orient yourself within the project layout. Use the Specifications Index table in `specifications/Specification.md` to identify which spec file(s) you need — but do not read the full top-level Specification.md unless you need the project overview.
- **Prefer reading specific line ranges** over entire files when possible. Use file outlines first, then read only the sections you need.
- `specifications/ToDo.md` is a large index file. Each component also has its own `todo/` file (e.g. `specifications/todo/analysis.md`). **Read only the component-level todo file** relevant to your current work instead of the full `ToDo.md`.
- Large spec files have a **TL;DR** section at the top. Read that first to decide whether you need the full file.
- When updating `./Structure.md`, keep entries concise — one-line descriptions only. The spec files themselves contain the detail.
- Use the **task-to-spec lookup table** below to find exactly which specs and todo files to load for a given task. Do not load specs not listed for your task.

### Task-to-Spec Lookup

| Task area | Required specs (read fully) | Reference only (read TL;DR) | Todo file |
|-----------|----------------------------|----------------------------|-----------|
| Chef API client / auth signing | `chef-api/` | `configuration/` | `todo/data-collection.md` |
| Node data collection | `data-collection/`, `chef-api/` | `configuration/`, `datastore/` | `todo/data-collection.md` |
| Cookbook fetching (git + server) | `data-collection/` | `chef-api/`, `configuration/`, `datastore/` | `todo/data-collection.md` |
| Cookbook usage analysis | `analysis/` (§ Usage Analysis) | `data-collection/`, `datastore/` | `todo/analysis.md` |
| CookStyle / compatibility testing | `analysis/` (§ Compatibility Testing) | `configuration/`, `packaging/` | `todo/analysis.md` |
| Remediation guidance | `analysis/` (§ Remediation) | `datastore/`, `visualisation/` | `todo/analysis.md` |
| Node upgrade readiness | `analysis/` (§ Readiness) | `data-collection/`, `datastore/` | `todo/analysis.md` |
| Role dependency graph | `data-collection/` (§ Role Graph) | `chef-api/`, `datastore/`, `visualisation/` | `todo/data-collection.md` |
| Database schema / migrations | `datastore/` | `configuration/` | `todo/project-setup.md` |
| Configuration parsing | `configuration/` | — | `todo/configuration.md` |
| TLS / certificate management | `tls/` | `configuration/`, `packaging/` | `todo/configuration.md` |
| Web API endpoints | `web-api/` | `auth/`, `datastore/` | `todo/visualisation.md` |
| Dashboard frontend | `visualisation/` | `web-api/` | `todo/visualisation.md` |
| Auth (local/LDAP/SAML) | `auth/` | `configuration/`, `web-api/` | `todo/auth.md` |
| Logging subsystem | `logging/` | `configuration/` | `todo/logging.md` |
| Elasticsearch / NDJSON export | `elasticsearch/` | `configuration/`, `datastore/` | `todo/visualisation.md` |
| RPM / DEB packaging | `packaging/` (§ RPM/DEB) | `configuration/` | `todo/packaging.md` |
| Container image / Dockerfile | `packaging/` (§ Container) | — | `todo/packaging.md` |
| Embedded Ruby environment | `packaging/` (§ Embedded Ruby) | `analysis/` | `todo/packaging.md` |
| Helm chart | `packaging/` (§ Helm) | `tls/`, `configuration/` | `todo/packaging.md` |
| Docker Compose / ELK stack | `packaging/` (§ Compose/ELK) | `elasticsearch/` | `todo/packaging.md` |
| CI/CD workflows | `packaging/` (§ CI/CD) | — | `todo/packaging.md` |
| Notifications (webhook/email) | `configuration/` (§ Notifications), `web-api/` (§ Notification Endpoints) | `logging/` | `todo/visualisation.md` |
| Data exports (CSV/JSON) | `web-api/` (§ Export Endpoints) | `configuration/`, `datastore/` | `todo/visualisation.md` |
| Secrets / credential storage | `secrets-storage/` | `configuration/`, `datastore/` | `todo/secrets-storage.md` |
| Credential encryption / rotation | `secrets-storage/` (§ Encryption Model, § Key Rotation) | `datastore/` | `todo/secrets-storage.md` |
| Credential resolution (DB/env/file) | `secrets-storage/` (§ Credential Resolution Precedence) | `configuration/`, `chef-api/` | `todo/secrets-storage.md` |
| Credential Web API (admin CRUD) | `secrets-storage/` (§ Web API Endpoints), `web-api/` (§ Credential Management) | `datastore/` | `todo/secrets-storage.md` |
| Kubernetes / Helm secrets | `secrets-storage/` (§ Kubernetes Secrets Integration), `packaging/` (§ Helm) | `configuration/` | `todo/secrets-storage.md` |

`(§ Section)` means read only that section of the spec, not the entire file.

---

## Orientation

- Read `./Structure.md` first to understand the layout of the project before exploring files or making changes.
- `./Structure.md` must be updated in the same change whenever a file or directory is added, moved, renamed, or removed.

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

- All specifications live under `.claude/specifications/`, organised into subdirectories by component.
- Before implementing any feature, check whether a specification exists for it. If not, write one first.
- The top-level specification is `.claude/specifications/Specification.md`. Start there for project context.
- The master to-do list is `.claude/specifications/ToDo.md`. Per-component to-do files live under `.claude/specifications/todo/` (e.g. `todo/analysis.md`, `todo/data-collection.md`). When completing tasks, update both the component file and the master `ToDo.md`.

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

All Go code lives in the repository root (single Go module). Follow this package structure:

```
cmd/
  chef-migration-metrics/       # main package — CLI entrypoint, flag parsing, startup
internal/
  chefapi/                      # Chef Infra Server API client, RSA signing, partial search
  collector/                    # Periodic collection job orchestration (nodes, cookbooks, roles)
  analysis/                     # Cookbook usage, CookStyle, Test Kitchen, readiness evaluation
  remediation/                  # Auto-correct preview, cop mapping, complexity scoring
  datastore/                    # Database access layer — queries, migrations, connection pool
  webapi/                       # HTTP handlers, router, middleware (auth, CORS, pagination)
  auth/                         # Authentication providers (local, LDAP, SAML) and RBAC
  config/                       # Configuration parsing, validation, env var overrides
  tls/                          # TLS listener setup, ACME integration, cert reload
  export/                       # CSV/JSON/NDJSON export generation, async job runner
  notify/                       # Webhook and email notification dispatch
  secrets/                      # Credential encryption, storage, resolution, rotation, zeroing
  logging/                      # Structured logger, log scopes, retention
  elasticsearch/                # NDJSON file writer, high-water-mark tracking
  embedded/                     # Embedded tool resolution (CookStyle, TK, Ruby lookup)
  models/                       # Shared domain types (Node, Cookbook, ReadinessResult, etc.)
frontend/                       # React application (separate npm project)
migrations/                     # Sequential numbered SQL migration files
```

Conventions:
- Use `internal/` to prevent external import of application packages.
- Each package should have a single clear responsibility matching its specification.
- Shared domain types used across packages live in `internal/models/`.
- Database queries are centralised in `internal/datastore/` — other packages must not import `database/sql` directly.
- HTTP handlers in `internal/webapi/` are thin — they validate input, call domain logic, and serialise output. Business logic lives in the domain packages (`analysis/`, `collector/`, etc.).
- Configuration structs live in `internal/config/` and are passed to other packages by value or interface — packages must not read config files or environment variables directly.
- Test files sit alongside the code they test (`foo_test.go` next to `foo.go`).
- Integration and functional tests use build tags (`//go:build functional`) so they are excluded from `go test ./...` by default.

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