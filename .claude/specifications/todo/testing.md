# Testing — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

> 756 tests across 6 packages: secrets (331), config (117), logging (93), collector (79), chefapi (74), datastore (62). All passing, 0 failures. Additional test items below track coverage of specific functional areas.

---

- [x] Unit tests for Chef API client and authentication — 74 tests in `internal/chefapi/client_test.go` covering RSA signing, HTTP client, API errors, retry logic, and all endpoints
- [x] Unit tests for partial search query builder — covered in `client_test.go` partial search tests (correct method/path/query/body verification)
- [ ] Unit tests for cookbook usage analysis
- [ ] Unit tests for cookbook usage analysis with Policyfile nodes
- [ ] Unit tests for node readiness calculation
- [x] Unit tests for stale node detection logic — covered in `client_test.go` NodeData helper tests (`IsStale`, `OhaiTimeAsTime`, missing ohai_time treated as stale)
- [ ] Unit tests for stale cookbook detection logic
- [ ] Unit tests for auto-correct preview generation (diff computation, statistics)
- [ ] Unit tests for cop-to-documentation mapping enrichment
- [ ] Unit tests for cookbook complexity score calculation (weighted scoring, label classification)
- [ ] Unit tests for blast radius computation (node count, role count via dependency graph, policy count)
- [ ] Unit tests for CookStyle version profile selection per target Chef Client version
- [x] Unit tests for role dependency graph building (role → role, role → cookbook parsing) — partially covered: `client_test.go` tests `GetRoles()`, `GetRole()` with `RunList`/`EnvRunLists`; run_list parsing and graph traversal not yet implemented
- [ ] Unit tests for dependency graph traversal (transitive dependencies)
- [ ] Unit tests for notification trigger evaluation (status change detection, milestone crossing)
- [ ] Unit tests for webhook notification payload construction and delivery
- [ ] Unit tests for email notification construction
- [ ] Unit tests for export generation (CSV, JSON, Chef search query formats)
- [ ] Unit tests for export async/sync threshold decision
- [ ] Unit tests for embedded tool resolution (embedded_bin_dir lookup, PATH fallback, missing directory handling)
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
