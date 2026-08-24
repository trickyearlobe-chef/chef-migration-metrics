# Project Conventions

Project-specific technical conventions for the Chef Migration Metrics dashboard.

## Deployment Context

- **Two deployment shapes**: a RHEL server (RPM package) and a developer workstation.
- **Assume no shell access on the deployed host** — every operation must be triggerable from the UI.
- **Scale**: large fleets. Assume node and git-repo counts big enough that any unbounded query is a bug, and that hypervisor capacity for Test Kitchen is limited.
- The UI must be usable by non-technical stakeholders (project managers, executives).

## Target Chef Version

- The application uses a **single active target Chef version** at any time (e.g. `19.1.164`).
- Target changes are infrequent (1–2 per year).
- When target changes: all cookstyle and kitchen results are **invalidated and re-run** for the new target.
- Materialised status columns (TK status, compatibility) are always relative to THE active target.
- Do not design for multiple concurrent targets — this adds complexity with no real-world benefit.

## Configuration

- **All application configuration is stored in the encrypted config store (database)**, not YAML files.
- The bootstrap YAML (`deploy/pkg/config.yml`) contains ONLY values required before the database is available: `database_url`, `listen_address`, `listen_port`.
- The master encryption key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) is an environment variable, never stored in config.
- New configuration sections go into the config store with a corresponding admin UI page.
- Do not add new settings to the YAML file unless they are genuinely required before database connectivity is established.
- **Configuration changes must take effect without a restart** unless there is a genuine technical reason live reload cannot be supported (e.g. listen address/port, TLS certificates). Document the reason if a restart is required.

## Database

- All database schema changes must be managed through migration files. Migrations must be sequential, numbered, and checked into source control.
- The application must run pending migrations automatically on startup.
- Migrations must never be edited after they have been committed. Instead, create a new migration to make further changes.
- This dashboard runs against **large fleets**. JSONB operations that scan or aggregate all rows must be bounded or paginated.

### Node Snapshot Invariant

- Each node has **exactly one row** in `node_snapshots`, identified by `(organisation_name, node_name)`.
- Nodes are **only upserted, never deleted and recreated** — delete-and-reinsert corrupts summaries and counts.
- Nodes are removed **only** when the Chef Server reports them gone (via `DeleteOrphanedNodeSnapshots`).
- Because of upsert semantics, node data is valid once written — a failed collection run does NOT invalidate previously written rows.

### Primary Key Strategy

- **Domain entities** use natural composite keys (e.g. `organisation_name`, `node_name`, `cookbook_name + version`, `git_repo_name`). Do not introduce synthetic UUIDs for these — they add a fragile layer of indirection that breaks silently during re-collection, upsert, and deduplication. Migrations 0001–0009 establish this pattern.
- **Ephemeral operational records** — tables that model transient processes with no stable natural identifier — may use UUID primary keys (`DEFAULT gen_random_uuid()`). Examples: `vm_tracking`, `node_kitchen_runs`, `kitchen_batches`, `git_kitchen_results` (migrations 0013–0016). These rows represent one-off runs or tracked VMs, not long-lived domain concepts.
- When adding a new table, default to natural keys. Use UUIDs only when the entity genuinely has no stable natural identifier and document the reasoning in the migration file header comment.

## Language and Concurrency

- All backend components must be implemented in **Go**.
- Use **goroutines** to parallelise work wherever independent units of work can proceed concurrently.
- Use **channels** or **sync primitives** (e.g. `sync.WaitGroup`, `errgroup`) to coordinate goroutines and collect results or errors.
- Goroutine concurrency must be **bounded** using worker pools. Each task type (organisation collection, node page fetching, git pulls, CookStyle scans, Test Kitchen runs, readiness evaluation) has its own independently configurable worker pool size. See `configuration.md` for the concurrency configuration schema and default values.
- Each concurrent work unit must propagate errors back to the caller rather than silently discarding them.

## Go Package Layout

- All Go code is a single module. Application packages live under `internal/`.
- Shared domain types live alongside their domain packages (e.g. `internal/datastore/`, `internal/analysis/`). There is no separate `internal/models/` package.
- Database queries are centralised in `internal/datastore/` — other packages must not import `database/sql` directly.
- HTTP handlers in `internal/webapi/` are thin wrappers — business logic lives in domain packages.
- Config structs live in `internal/config/` and are passed by value or interface — packages must not read config files or env vars directly.
- Test files sit alongside code (`foo_test.go` next to `foo.go`).
- Integration tests use build tags (`//go:build functional`) and are excluded from `go test ./...`.

## Frontend Conventions

- The React frontend lives in `frontend/` and is built with `npm run build` into `frontend/build/` (or `frontend/dist/`).
- The Go binary embeds the built frontend assets using `go:embed` and serves them from the web server.
- The frontend communicates exclusively through the Web API (`/api/v1/...`). It never accesses the database directly.

## API Conventions

- The frontend is the **sole consumer** of the API. There are no third-party integrations or backward-compatibility obligations.
- API contracts can be changed freely — no versioning or deprecation cycle needed.
- All list endpoints must support server-side pagination, filtering, and sorting via SQL pushdown. In-memory pagination of full result sets is not acceptable at fleet scale.

- All exported functions that can fail must return `error` as the last return value. Do not panic for recoverable errors.
- Wrap errors with context using `fmt.Errorf("operation: %w", err)` so that callers can trace the failure path.
- Use sentinel errors (e.g. `var ErrNotFound = errors.New("not found")`) for conditions that callers need to check with `errors.Is()`.
- HTTP handlers must map domain errors to appropriate HTTP status codes in `internal/webapi/` — domain packages must not import `net/http`.
- Background jobs (collection, analysis, export) must log errors and continue processing remaining items. A single failing organisation, cookbook, or node must not abort the entire job.
- External process execution (CookStyle, Test Kitchen, git) must enforce timeouts, capture stderr, and return structured error information — not raw exec failures.

## Cross-View Consistency

- When the same entity attribute appears in more than one view (list, detail, filter, export), all views must agree on **both** (a) the derivation function **and** (b) which underlying record represents the entity. Sharing only the derivation is not enough — divergent record selection still produces inconsistent results.
- If an attribute is invariant over some dimension (e.g. the disk verdict is invariant over target Chef version), resolve it independently of that dimension in every view. Do not scope a lookup by a key the value does not depend on.
- Never let a `LEFT JOIN ... IS NULL` (or equivalent) conflate "no record" with "indeterminate value" — they are distinct states and must be filtered and displayed distinctly.

## Naming Conventions

- **Go packages**: lowercase, single-word where possible (`chefapi`, `webapi`, `datastore`), matching the directory name.
- **Go files**: `snake_case.go` (e.g. `partial_search.go`, `readiness_evaluator.go`). Test files: `*_test.go`.
- **Go types**: `PascalCase` — use domain nouns (`NodeSnapshot`, `CookbookVersion`, `ReadinessResult`), not generic names (`Data`, `Item`, `Record`).
- **Go interfaces**: name by capability, not by `I` prefix (`Collector`, `Authenticator`, `Exporter`). Single-method interfaces use the `-er` suffix.
- **Database tables**: `snake_case`, plural (`node_snapshots`, `cookbook_versions`, `readiness_results`).
- **Database columns**: `snake_case` (`chef_version`, `policy_name`, `is_stale`, `created_at`).
- **Migration files**: `NNNN_short_description.up.sql` / `NNNN_short_description.down.sql` (e.g. `0001_create_node_snapshots.up.sql`).
- **API endpoints**: kebab-case paths under `/api/v1/` (e.g. `/api/v1/dependency-graph`, `/api/v1/cookbook-compatibility`).
- **Configuration keys**: `snake_case` in YAML (e.g. `stale_node_threshold_days`, `cookstyle_timeout_minutes`).
- **Environment variable overrides**: `SCREAMING_SNAKE_CASE` with `CM_` prefix (e.g. `CM_DATABASE_URL`).