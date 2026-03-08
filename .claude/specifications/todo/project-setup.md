# ToDo — Project Setup

Status key: [ ] Not started | [~] In progress | [x] Done

- [x] Initialise Go repository structure (`go mod init`) — `go.mod` exists (`github.com/trickyearlobe-chef/chef-migration-metrics`, go 1.25.4)
- [x] Add LICENSE file (Apache 2.0)
- [x] Add README.md with project overview and getting started guide
- [x] Document technology stack (Go backend, React frontend)
- [x] Create `.gitignore` with patterns for build output, Go, Node.js, IDE, OS, secrets, runtime data, and test artifacts
- [x] Create `.dockerignore` to keep Docker build context small and exclude secrets from the daemon
- [x] Create `.helmignore` at `deploy/helm/chef-migration-metrics/.helmignore` for Helm chart packaging
- [x] Add ignore file maintenance rule to `Claude.md`
- [x] Set up Go dependency management (`go.mod`, `go.sum`) — `go.sum` exists with `golang.org/x/crypto`, `github.com/lib/pq`, and `gopkg.in/yaml.v3` dependencies
- [x] Set up CI pipeline
- [x] Set up database migration tooling — custom migration runner in `internal/datastore/datastore.go` (discovers `NNNN_*.up.sql` files, applies in order within transactions, records in `schema_migrations` table)
- [x] Create `migrations/` directory and establish migration file naming convention — `migrations/0001_initial_schema.up.sql` and `.down.sql` exist
- [x] Implement automatic migration execution on application startup — `cmd/chef-migration-metrics/main.go` calls `db.MigrateUp()` during startup
- [x] Verify pending migrations cause startup failure with a descriptive error — `db.MigrateUp()` returns wrapped errors with migration version and name (e.g. `"datastore: applying migration 0003 (cookbook_usage_analysis): ..."`) and `discoverMigrations()` reports missing directories with the path; `main.go` logs these at ERROR severity via the startup scope and exits with code 1; verified by `TestDiscoverMigrations_NonexistentDir_DescriptiveError` and `TestDiscoverMigrations_DuplicateVersion_DescriptiveError` in `datastore_test.go`
- [x] Create `Makefile` with build, test, lint, package, version bump, and functional test targets
- [x] Implement `--version` CLI flag — `main.go` supports `-version` flag with build-time version injection via `-ldflags`
- [ ] Fix 50+ `errcheck` linter violations and re-enable the linter — `errcheck` is temporarily disabled in `.golangci.yml`. Violations span `frontend/embed_test.go`, `internal/analysis/`, `internal/chefapi/`, `internal/collector/`, `internal/config/`, `internal/datastore/`, `internal/frontend/`, `internal/remediation/`, `internal/secrets/`, and `internal/tls/`. Common patterns: unchecked `defer f.Close()`, `defer rows.Close()`, `defer stmt.Close()`, `defer resp.Body.Close()`, logging calls (`log.Debug`, `log.Warn`, `log.Info`, `log.Error`), and test helper calls (`os.WriteFile`, `os.Remove`, `os.Mkdir`, `w.Write`).
