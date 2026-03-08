# Testing — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

> **1,965 tests across 14 packages:** analysis (263), auth (129), chefapi (90), collector (235), config (117), datastore (79), embedded (21), export (36), frontend (11), logging (93), remediation (114), secrets (331), tls (79), webapi (364). All passing, 0 failures. Additional test items below track coverage of specific functional areas.

---

- [x] Unit tests for Chef API client and authentication — 90 tests in `internal/chefapi/client_test.go` covering RSA signing, HTTP client, API errors, retry logic, and all endpoints
- [x] Unit tests for partial search query builder — covered in `client_test.go` partial search tests (correct method/path/query/body verification)
- [x] Unit tests for cookbook usage analysis — 51 tests in `internal/analysis/usage_test.go` covering tuple extraction, aggregation, active set building, parallel extraction, Policyfile nodes, end-to-end pipeline
- [x] Unit tests for cookbook usage analysis with Policyfile nodes — covered in `usage_test.go` (`TestExtractNodeTuples_PolicyfileNode`, `TestEndToEnd_FullAnalysisPipeline` with mixed classic and Policyfile nodes)
- [x] Unit tests for node readiness calculation — 77 tests in `internal/analysis/readiness_test.go` covering per-node evaluation, cookbook compatibility checks, disk space evaluation, stale node handling, blocking reasons, parallel evaluation
- [x] Unit tests for stale node detection logic — covered in `chefapi/client_test.go` NodeData helper tests (`IsStale`, `OhaiTimeAsTime`, missing ohai_time treated as stale)
- [ ] Unit tests for stale cookbook detection logic
- [x] Unit tests for auto-correct preview generation (diff computation, statistics) — 52 tests in `internal/remediation/autocorrect_test.go` covering directory copy, diff generation, LCS-based edit computation, hunk grouping, statistics, temp cleanup
- [x] Unit tests for cop-to-documentation mapping enrichment — 21 tests in `internal/remediation/copmapping_test.go` covering LookupCop, AllCopMappings, CopMappingCount, embedded mapping index
- [x] Unit tests for cookbook complexity score calculation (weighted scoring, label classification) — 41 tests in `internal/remediation/complexity_test.go` covering ComputeComplexityScore with all weight factors, ScoreToLabel boundary conditions, blast radius loading
- [ ] Unit tests for blast radius computation (node count, role count via dependency graph, policy count)
- [ ] Unit tests for CookStyle version profile selection per target Chef Client version
- [x] Unit tests for role dependency graph building (role → role, role → cookbook parsing) — 33 tests in `internal/collector/runlist_test.go` covering `ParseRunListEntry`, `ParseRunList`, `BuildRoleDependencies` with role→role, role→cookbook edges, env_run_lists, deduplication
- [ ] Unit tests for dependency graph traversal (transitive dependencies)
- [ ] Unit tests for notification trigger evaluation (status change detection, milestone crossing)
- [ ] Unit tests for webhook notification payload construction and delivery
- [ ] Unit tests for email notification construction
- [x] Unit tests for export generation (CSV, JSON, Chef search query formats) — 36 tests in `internal/export/export_test.go` covering ready node export (CSV, JSON, Chef search query), blocked node export (CSV, JSON, complexity scores, blocking reasons), cookbook remediation export (CSV, JSON, target version filter, complexity label filter), filter application (environment, platform, role, stale, no-filter passthrough), max rows, write-to-disk, invalid format rejection
- [ ] Unit tests for export async/sync threshold decision
- [x] Unit tests for embedded tool resolution (embedded_bin_dir lookup, PATH fallback, missing directory handling) — 21 tests in `internal/embedded/embedded_test.go` covering ResolvePath, ValidateCookstyle, ValidateKitchen, ValidateDocker, ValidateGit, missing directory, PATH fallback
- [x] Unit tests for local authentication — 129 tests in `internal/auth/` covering `LocalAuthenticator` (success, failure, lockout, non-local provider rejection, store errors), `SessionManager` (create, validate, invalidate, cleanup, lifetime), `Middleware` (RequireAuth, RequireAdmin, RequireRole, token extraction), `Password` (hashing, validation rules, bcrypt cost)
- [x] Unit tests for auth web API handlers — covered in `internal/webapi/` tests for login, logout, me, admin user CRUD (list, create, update, delete, password reset), method checks, auth-not-configured fallback
- [ ] Unit tests for Elasticsearch NDJSON export (document format, doc_id generation, .tmp suffix handling)
- [ ] Unit tests for Elasticsearch high-water-mark tracking (incremental export, first-run full export)
- [ ] Integration tests for data collection against a test Chef server
- [ ] Integration tests for data collection of Policyfile nodes
- [ ] Integration tests for dashboard API endpoints
- [ ] Integration tests for remediation API endpoints
- [ ] Integration tests for dependency graph API endpoints
- [ ] Integration tests for export API endpoints
- [ ] Integration tests for notification delivery (webhook mock, SMTP mock)
- [ ] Integration tests for Elasticsearch export pipeline (write NDJSON → Logstash → Elasticsearch → Kibana query)
- [ ] End-to-end test covering collection → analysis → remediation → dashboard display
- [ ] Verify embedded Ruby environment builds successfully for amd64 and arm64
- [ ] Verify embedded `cookstyle --version` executes without system Ruby
- [ ] Verify embedded `kitchen version` executes without system Ruby
- [ ] Verify embedded tools do not conflict with a pre-existing Chef Workstation installation
- [ ] Verify RPM installs, starts, and runs on a fresh RHEL/Rocky/Alma system (with embedded tools)
- [ ] Verify DEB installs, starts, and runs on a fresh Debian/Ubuntu system (with embedded tools)
- [ ] Verify Docker Compose stack starts and passes health checks
- [ ] Verify ELK testing stack starts and Logstash indexes test data into Elasticsearch
- [ ] Verify Helm chart deploys and passes `helm test`
