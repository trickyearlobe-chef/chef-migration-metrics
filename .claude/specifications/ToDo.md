# Chef Migration Metrics - ToDo

This is the **master** to-do list. It is large (~600 lines). To save tokens, prefer the per-component files under `todo/` instead — they contain the same tasks split by area:

| File | Area |
|------|------|
| `todo/specification.md` | Specification writing tasks |
| `todo/project-setup.md` | Project setup and tooling |
| `todo/data-collection.md` | Node collection, cookbook fetching, role graph |
| `todo/analysis.md` | Usage analysis, compatibility testing, remediation, readiness |
| `todo/visualisation.md` | Dashboard, dependency graph, exports, notifications, log viewer |
| `todo/logging.md` | Logging infrastructure |
| `todo/auth.md` | Authentication and authorisation |
| `todo/configuration.md` | Configuration and TLS |
| `todo/secrets-storage.md` | Secrets and credential management (encryption, storage, rotation, resolution) |
| `todo/packaging.md` | Build tooling, embedded Ruby, RPM/DEB, container, Compose, ELK, Helm, CI/CD |
| `todo/testing.md` | Unit, integration, and end-to-end tests |
| `todo/documentation.md` | User and developer documentation |

When completing tasks, update **both** the component file and this master file.

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Specification

- [x] Write top-level project specification
- [x] Write component specification: Data Collection (node collection, cookbook fetching)
- [x] Write component specification: Analysis (cookbook usage, compatibility testing, readiness)
- [x] Write component specification: Data Visualisation (dashboard, trending)
- [x] Write component specification: Configuration
- [x] Write component specification: Authentication and Authorisation (SAML, LDAP, local accounts)
- [x] Write component specification: Logging
- [x] Write component specification: Datastore schema
- [x] Write component specification: Web API (HTTP layer between backend and frontend)
- [x] Write component specification: Packaging and Deployment (RPM, DEB, container, Docker Compose, Helm)
- [x] Write component specification: Elasticsearch Export (NDJSON export, Logstash pipeline, ELK testing stack)
- [ ] Write database migration files for initial schema
- [x] Document background job scheduling and recovery behaviour
- [x] Populate specifications/chef-api/Specification.md with relevant API endpoint references
- [x] Flesh out analysis specification with design decisions (Test Kitchen invocation, CookStyle parsing, disk space evaluation)
- [x] Add Policyfile support to data collection, analysis, visualisation, web API, datastore, and configuration specifications
- [x] Add stale node detection to data collection, analysis, visualisation, web API, datastore, and configuration specifications
- [x] Add stale cookbook detection to data collection, visualisation, datastore, and configuration specifications
- [x] Add remediation guidance (auto-correct preview, migration docs, complexity scoring) to analysis, visualisation, web API, and datastore specifications
- [x] Add role dependency graph to data collection, visualisation, web API, and datastore specifications
- [x] Add data export capability to visualisation, web API, datastore, and configuration specifications
- [x] Add notification system (webhook, email) to visualisation, web API, datastore, logging, and configuration specifications
- [x] Add confidence indicators to visualisation and web API specifications
- [x] Add CookStyle version profiles to analysis specification
- [x] Add cookbook upload date / first-seen tracking to data collection and datastore specifications
- [x] Update Chef API specification with policy_name, policy_group, and ohai_time partial search attributes
- [x] Add embedded Ruby environment (CookStyle, Test Kitchen, kitchen-dokken) to packaging, analysis, and configuration specifications
- [x] Add `analysis_tools` configuration section (embedded_bin_dir, timeouts) to configuration specification
- [x] Add Elasticsearch export specification (document types, NDJSON format, Logstash pipeline, ELK stack)
- [x] Update analysis specification for embedded tool resolution and startup validation
- [x] Remove Dockerfile.analysis from packaging specification — single image now includes embedded tools
- [x] Write component specification: Secrets Storage (credential encryption, storage methods, resolution, rotation, Kubernetes integration)

---

## Project Setup

- [ ] Initialise Go repository structure (`go mod init`)
- [x] Add LICENSE file (Apache 2.0)
- [x] Add README.md with project overview and getting started guide
- [x] Document technology stack (Go backend, React frontend)
- [x] Create `.gitignore` with patterns for build output, Go, Node.js, IDE, OS, secrets, runtime data, and test artifacts
- [x] Create `.dockerignore` to keep Docker build context small and exclude secrets from the daemon
- [x] Create `.helmignore` at `deploy/helm/chef-migration-metrics/.helmignore` for Helm chart packaging
- [x] Add ignore file maintenance rule to `Claude.md`
- [ ] Set up Go dependency management (`go.mod`, `go.sum`)
- [x] Set up CI pipeline
- [ ] Set up database migration tooling (`golang-migrate/migrate` or equivalent)
- [ ] Create `migrations/` directory and establish migration file naming convention
- [ ] Implement automatic migration execution on application startup
- [ ] Verify pending migrations cause startup failure with a descriptive error
- [x] Create `Makefile` with build, test, lint, package, version bump, and functional test targets

---

## Data Collection

### Node Collection
- [ ] Implement Chef Infra Server API client in Go with native RSA signed request authentication (no external signing libraries)
- [ ] Implement partial search against node index (`POST /organizations/NAME/search/node`)
- [ ] Collect required node attributes:
  - [ ] `name`
  - [ ] `chef_environment`
  - [ ] `automatic.chef_packages.chef.version`
  - [ ] `automatic.platform` and `automatic.platform_version`
  - [ ] `automatic.platform_family`
  - [ ] `automatic.filesystem` (disk space)
  - [ ] `automatic.cookbooks` (resolved cookbook list)
  - [ ] `run_list`
  - [ ] `automatic.roles` (expanded)
  - [ ] `policy_name` (Policyfile policy name, top-level attribute)
  - [ ] `policy_group` (Policyfile policy group, top-level attribute)
  - [ ] `automatic.ohai_time` (Unix timestamp of last Chef client run)
- [ ] Support multiple Chef server organisations
- [ ] Collect from multiple organisations in parallel using goroutines (one per organisation)
- [ ] Bound organisation collection concurrency with the `concurrency.organisation_collection` worker pool setting
- [ ] Use `errgroup` or equivalent to coordinate goroutines and aggregate errors without cancelling successful collections
- [ ] Implement concurrent pagination within a single organisation — fetch pages in parallel once total node count is known, bounded by the `concurrency.node_page_fetching` worker pool setting
- [ ] Implement periodic background collection job
- [ ] Implement fault tolerance — failure in one organisation must not block others
- [ ] Implement checkpoint/resume so failed jobs can continue without starting over
- [ ] Persist collected node data to datastore with timestamps

### Policyfile Support
- [ ] Classify nodes as Policyfile nodes (both `policy_name` and `policy_group` non-null) or classic nodes
- [ ] Persist `policy_name` and `policy_group` in node snapshots
- [ ] Ensure cookbook usage analysis works identically for Policyfile and classic nodes (both use `automatic.cookbooks`)

### Stale Node Detection
- [ ] After collection, compare each node's `ohai_time` against the current time
- [ ] Flag nodes whose `ohai_time` is older than `collection.stale_node_threshold_days` (default: 7) as stale in the datastore
- [ ] Persist `is_stale` flag on `node_snapshots` rows
- [ ] Include stale flagging in the collection run sequence (step 5, after cookbook fetching)

### Cookbook Fetching
- [ ] Implement cookbook fetch from Chef server (`GET /organizations/NAME/cookbooks/NAME/VERSION`)
- [ ] Skip download of cookbook versions already present in the datastore (immutability optimisation)
- [ ] Key all Chef server cookbook data in the datastore by organisation + cookbook name + version
- [ ] Implement manual rescan option to force re-download and re-analysis of a specific cookbook version
- [ ] Implement cookbook clone from git repository
- [ ] Support multiple configured base git URLs
- [ ] Pull latest changes from git repositories on every collection run
- [ ] Run git pull operations across multiple repositories in parallel using goroutines, bounded by the `concurrency.git_pull` worker pool setting
- [ ] Detect default branch automatically (`main` or `master`)
- [ ] Record HEAD commit SHA for the default branch after each pull
- [ ] Detect whether a fetched cookbook includes a test suite
- [ ] Record `first_seen_at` timestamp for each cookbook version (proxy for upload date if Chef server does not expose one)
- [ ] Flag cookbooks as stale when most recent version's `first_seen_at` is older than `collection.stale_cookbook_threshold_days` (default: 365)

### Role Dependency Graph Collection
- [ ] Fetch full list of roles per organisation using `GET /organizations/ORG/roles`
- [ ] Fetch role detail per role using `GET /organizations/ORG/roles/ROLE_NAME`
- [ ] Parse each role's `run_list` to extract cookbook references (`recipe[cookbook::recipe]`) and nested role references (`role[other_role]`)
- [ ] Build directed graph of role → role and role → cookbook dependencies
- [ ] Persist dependency graph to the `role_dependencies` table in the datastore
- [ ] Refresh dependency graph on every collection run

---

## Analysis

### Cookbook Usage Analysis
- [ ] Determine which cookbooks are in active use from collected `automatic.cookbooks` attribute
- [ ] Determine which versions of each cookbook are in use
- [ ] Determine which roles reference each cookbook
- [ ] Determine which Policyfile policy names and policy groups reference each cookbook
- [ ] Determine which nodes are running each cookbook and version
- [ ] Count nodes running each cookbook and version
- [ ] Count platform versions and platform families running each cookbook and version
- [ ] Persist usage analysis results to datastore (including policy name and policy group references)

### Cookbook Compatibility Testing
- [ ] Implement embedded tool resolution — look for `cookstyle` and `kitchen` in `analysis_tools.embedded_bin_dir` first, fall back to `PATH`
- [ ] Implement startup validation for `kitchen` (check embedded dir, then PATH; disable Test Kitchen if not found)
- [ ] Implement startup validation for `cookstyle` (check embedded dir, then PATH; disable CookStyle if not found)
- [ ] Implement startup validation for `docker` (`docker info`; warn and disable Test Kitchen if not found)
- [ ] Implement Test Kitchen integration for cookbooks sourced from git
- [ ] Support testing against multiple configured target Chef Client versions
- [ ] Test only the HEAD commit of the default branch (`main` or `master`)
- [ ] Skip test run if HEAD commit SHA is unchanged since last test run for a given cookbook + target Chef Client version
- [ ] Record HEAD commit SHA alongside each test result
- [ ] Record convergence pass/fail per cookbook + target Chef Client version + HEAD commit SHA
- [ ] Record test suite pass/fail per cookbook + target Chef Client version + HEAD commit SHA
- [ ] Dispatch Test Kitchen runs in parallel using goroutines (one per cookbook + target Chef Client version), bounded by the `concurrency.test_kitchen_run` worker pool setting
- [ ] Capture stdout/stderr from each Test Kitchen process and return alongside pass/fail result
- [ ] Honour `analysis_tools.test_kitchen_timeout_minutes` for Test Kitchen process timeout
- [ ] Implement CookStyle linting for cookbooks sourced from Chef server (no test suite)
- [ ] Implement CookStyle version profiles — enable only cops relevant to the specific target Chef Client version being tested
- [ ] Maintain CookStyle cop-to-version mapping as embedded application data
- [ ] Fall back to full `ChefDeprecations` and `ChefCorrectness` namespaces when target version cannot be mapped
- [ ] Run CookStyle scans in parallel using goroutines, bounded by the `concurrency.cookstyle_scan` worker pool setting
- [ ] Capture stdout/stderr from each CookStyle process and return alongside results
- [ ] Honour `analysis_tools.cookstyle_timeout_minutes` for CookStyle process timeout
- [ ] Skip CookStyle scan for cookbook versions already scanned in the datastore (immutability optimisation)
- [ ] Record CookStyle results and deprecation warnings keyed by organisation + cookbook name + version + target Chef version
- [ ] Implement manual rescan option for CookStyle consistent with cookbook download rescan
- [ ] Persist all test results to datastore

### Remediation Guidance
- [ ] Implement auto-correct preview generation:
  - [ ] Create temporary copy of cookbook directory
  - [ ] Run `cookstyle --auto-correct --format json` on the copy
  - [ ] Generate unified diff between original and auto-corrected files
  - [ ] Compute statistics (total offenses, correctable, remaining, files modified)
  - [ ] Persist auto-correct preview to `autocorrect_previews` table
  - [ ] Clean up temporary copy
  - [ ] Only generate for cookbooks with CookStyle offenses
  - [ ] Cache results for immutable Chef server cookbook versions
- [ ] Implement migration documentation link enrichment:
  - [ ] Build and maintain cop-to-documentation mapping table (`cop_name → { description, migration_url, introduced_in, removed_in, replacement_pattern }`)
  - [ ] Ship mapping as embedded data in the application binary
  - [ ] Enrich every `ChefDeprecations/*` and `ChefCorrectness/*` offense with its mapping entry
  - [ ] Persist enriched offenses in CookStyle results
- [ ] Implement cookbook complexity scoring:
  - [ ] Compute weighted score per cookbook per target Chef Client version (error: 5, deprecation: 3, correctness: 3, non-auto-correctable: 4, modernize: 1, TK converge fail: 20, TK test fail: 10)
  - [ ] Classify score into labels: `none` (0), `low` (1-10), `medium` (11-30), `high` (31-60), `critical` (61+)
  - [ ] Compute blast radius: affected node count, role count (using dependency graph), policy count
  - [ ] Persist complexity records to `cookbook_complexity` table
  - [ ] Recompute after every CookStyle scan and Test Kitchen run cycle

### Node Upgrade Readiness
- [ ] Implement readiness calculation per node per target Chef Client version
- [ ] Evaluate nodes in parallel using goroutines, bounded by the `concurrency.readiness_evaluation` worker pool setting
- [ ] Check all cookbooks in expanded run-list are compatible with target version
- [ ] Check available disk space meets threshold for Habitat bundle installation
- [ ] Record blocking reasons per node (incompatible cookbooks with complexity scores, insufficient disk space)
- [ ] Handle stale nodes: treat disk space data as unknown, set `stale_data` flag on readiness result
- [ ] Include complexity score and label in each blocking cookbook entry
- [ ] Persist readiness results to datastore

---

## Data Visualisation

### Dashboard
- [ ] Choose and set up web framework
- [ ] Implement Chef Client version distribution view with trend over time
- [ ] Implement cookbook compatibility status view (per cookbook, version, target Chef Client version)
- [ ] Implement confidence indicators — green for Test Kitchen pass (high), amber for CookStyle-only pass (medium), red for incompatible, grey for untested
- [ ] Implement cookbook complexity score display alongside compatibility status
- [ ] Implement stale cookbook indicator (badge/icon for cookbooks not updated in configured threshold)
- [ ] Implement node upgrade readiness summary (ready vs. blocked vs. stale counts)
- [ ] Implement stale node indicators with last check-in age display
- [ ] Implement per-node blocking reason detail view with complexity scores per blocking cookbook
- [ ] Implement interactive filters:
  - [ ] Filter by Chef server organisation
  - [ ] Filter by environment
  - [ ] Filter by role
  - [ ] Filter by Policyfile policy name
  - [ ] Filter by Policyfile policy group
  - [ ] Filter by platform / platform version
  - [ ] Filter by target Chef Client version
  - [ ] Filter by active/unused cookbook status
  - [ ] Filter by stale node status (all, stale, fresh)
  - [ ] Filter by complexity label (low, medium, high, critical)
- [ ] Implement drill-down from summary to node detail
- [ ] Implement drill-down from summary to cookbook detail
- [ ] Implement drill-down from blocking cookbook to remediation guidance
- [ ] Implement drill-down from dependency graph nodes to cookbook/role detail
- [ ] Ensure dashboard performs acceptably with many thousands of nodes

### Dependency Graph View
- [ ] Implement interactive directed graph rendering (roles and cookbooks as nodes, includes as edges)
- [ ] Colour-code cookbook nodes by compatibility status (green=compatible, red=incompatible, grey=untested, amber=CookStyle-only)
- [ ] Highlight incompatible cookbooks and the roles that depend on them
- [ ] Support filtering by specific cookbook (show subgraph involving that cookbook)
- [ ] Support filtering by specific role (show subgraph reachable from that role)
- [ ] Support filtering by compatibility status (show only paths involving incompatible/untested cookbooks)
- [ ] Implement search/filter for large graphs to focus on a subset
- [ ] Implement lazy loading or level-of-detail rendering for large graphs
- [ ] Implement alternative table view showing roles with direct and transitive cookbook dependencies
- [ ] Link cookbook nodes to cookbook detail view
- [ ] Link role nodes to node list filtered by that role

### Remediation Guidance View
- [ ] Implement remediation priority list — incompatible cookbooks sorted by priority score (complexity × blast radius)
- [ ] Display per-cookbook: complexity score/label, blast radius, auto-correctable vs. manual-fix count, top deprecations
- [ ] Implement auto-correct preview display with unified diff viewer
- [ ] Display auto-correct statistics (total offenses, correctable, remaining, files modified)
- [ ] Include prominent notice that auto-correct is preview only — tool does not modify cookbook source
- [ ] Implement migration documentation display per deprecation offense:
  - [ ] Human-readable description
  - [ ] Link to Chef migration docs
  - [ ] Chef version where deprecation was introduced/removed
  - [ ] Before/after replacement pattern code example
- [ ] Group deprecation offenses by cop name for consolidated view
- [ ] Implement effort estimation summary at top of remediation view:
  - [ ] Total cookbooks needing remediation
  - [ ] Estimated quick wins (auto-correct only)
  - [ ] Estimated manual fixes needed
  - [ ] Total blocked nodes and projected unblocked count

### Data Exports
- [ ] Implement ready node export (CSV, JSON, Chef search query string)
- [ ] Implement blocked node export (CSV, JSON) with blocking reasons and complexity scores
- [ ] Implement cookbook remediation report export (CSV, JSON)
- [ ] Ensure all exports respect currently active filters
- [ ] Implement synchronous export for small result sets
- [ ] Implement asynchronous export for large result sets (return job ID, poll for completion, download link)
- [ ] Implement export job status tracking in `export_jobs` table
- [ ] Implement export file retention and cleanup based on `exports.retention_hours`

### Notifications
- [ ] Implement webhook notification channel (HTTP POST with JSON payload)
- [ ] Implement email notification channel (SMTP)
- [ ] Implement notification trigger: cookbook status change
- [ ] Implement notification trigger: readiness milestone (configurable percentage thresholds)
- [ ] Implement notification trigger: new incompatible cookbook detected
- [ ] Implement notification trigger: collection failure
- [ ] Implement notification trigger: stale node threshold exceeded
- [ ] Implement notification filtering by organisation and cookbook
- [ ] Implement notification history display in the dashboard
- [ ] Implement notification delivery retry with configurable backoff
- [ ] Persist notification history to `notification_history` table

### Historical Trending
- [ ] Store timestamped metric snapshots during each collection run
- [ ] Implement trend charts for Chef Client version adoption over time
- [ ] Implement trend charts for node readiness counts over time
- [ ] Implement trend charts for aggregate complexity score over time
- [ ] Implement trend charts for stale node count over time

### Log Viewer
- [ ] Implement log viewer in the web UI
- [ ] Scope and display logs per collection job run (per organisation)
- [ ] Scope and display logs per cookbook git operation (clone/pull)
- [ ] Scope and display logs per Test Kitchen run (per cookbook + target Chef Client version)
- [ ] Scope and display logs per CookStyle scan (per cookbook version)
- [ ] Scope and display logs per notification dispatch (per channel)
- [ ] Scope and display logs per export job
- [ ] Implement log filtering by job type, organisation, cookbook name, and date/time
- [ ] Capture and store stdout/stderr from external processes (Test Kitchen, CookStyle, git)
- [ ] Implement log retention purge based on configured retention period

---

## Logging Infrastructure

- [ ] Implement structured logging with consistent severity levels (`DEBUG`, `INFO`, `WARN`, `ERROR`)
- [ ] Include contextual metadata in each log entry (timestamp, severity, organisation, cookbook name, commit SHA as applicable)
- [ ] Persist log entries to the datastore
- [ ] Capture stdout/stderr from external processes and associate with the relevant log scope
- [ ] Implement log retention period configuration and automated purge of expired logs
- [ ] Implement log level configuration to control minimum persisted severity
- [ ] Implement `notification_dispatch` log scope for notification delivery logging
- [ ] Implement `export_job` log scope for export operation logging

---

## Authentication and Authorisation

- [ ] Implement local user account authentication
- [ ] Implement LDAP authentication
- [ ] Implement SAML authentication
- [ ] Implement role-based authorisation
- [ ] Ensure credentials and secrets are never stored in source control

---

## Secrets Storage

- [x] Write secrets storage specification (`secrets-storage/Specification.md`)
- [x] Update `Specification.md` (top-level) specifications index with secrets-storage entry
- [x] Update `Structure.md` project layout with `internal/secrets/` package and `secrets-storage/` spec directory
- [x] Update `Structure.md` specification relationships table with secrets-storage cross-references
- [x] Update `Claude.md` task-to-spec lookup table with secrets-storage entries

### Core Encryption

- [ ] Implement `internal/secrets/encryption.go` — AES-256-GCM encrypt/decrypt with HKDF-SHA256 key derivation
- [ ] Implement 12-byte random nonce generation per encryption operation
- [ ] Implement AAD construction (`<credential_type>:<name>`) for ciphertext binding
- [ ] Implement at-rest format serialisation/deserialisation (`<nonce_hex>:<ciphertext_hex>`)
- [ ] Validate master key length ≥ 32 bytes after Base64 decode

### Memory Zeroing

- [ ] Implement `internal/secrets/zeroing.go` — `zeroBytes` and `zeroString` helpers
- [ ] Ensure all credential decrypt paths zero plaintext after use

### Credential Store

- [ ] Define `CredentialStore` interface in `internal/secrets/store.go`
- [ ] Implement database-backed `CredentialStore` (Create, Get, GetMetadata, Update, Delete, List, ListByType, Test, ReferencedBy)
- [ ] Implement reference check preventing deletion of in-use credentials

### Credential Resolution

- [ ] Implement `internal/secrets/resolver.go` — precedence logic (database → env var → file path)
- [ ] Return descriptive error when no credential source is configured

### Credential Validation

- [ ] Implement `internal/secrets/validation.go` — per-type validation (RSA PEM, URL, non-empty string)

### Credential Testing

- [ ] Implement per-type credential test functions (Chef API key, LDAP bind, SMTP AUTH, webhook HEAD, generic decrypt)

### Master Key Rotation

- [ ] Implement `internal/secrets/rotation.go` — detect dual keys, re-encrypt all rows atomically per row
- [ ] Log `INFO` with re-encryption count, `ERROR` for undecryptable rows
- [ ] Handle crash-recovery (partially rotated state)

### Startup Validation

- [ ] Validate master key presence and length when DB credentials exist
- [ ] Validate each DB credential can be decrypted (log `ERROR` per failure, continue)
- [ ] Warn on overly permissive file permissions for key files, key directories, and env files

### Web API Integration

- [ ] Wire `CredentialStore` into admin credential handlers (GET, POST, PUT, DELETE, test)
- [ ] Return `503` when encryption key is not configured
- [ ] Require `admin` role on all credential endpoints
- [ ] Verify no endpoint returns plaintext or `encrypted_value`

### Consumer Integration

- [ ] Update `internal/chefapi/` to resolve Chef API keys via `CredentialResolver`
- [ ] Update `internal/auth/` LDAP provider to resolve bind password via `CredentialResolver`
- [ ] Update `internal/notify/` SMTP and webhook senders to resolve credentials via `CredentialResolver`
- [ ] Verify plaintext is zeroed after use in all consumer call sites

### Configuration Integration

- [ ] Add `client_key_credential` and `client_key_env` fields to organisation config
- [ ] Add `bind_password_credential` field to LDAP auth config
- [ ] Add `password_credential` field to SMTP config
- [ ] Add `url_credential` field to notification channel config
- [ ] Add `secrets.credentialEncryptionKey` and `secrets.smtpPassword` to Helm `values.yaml`
- [ ] Update Helm `secret.yaml` template with `CMM_CREDENTIAL_ENCRYPTION_KEY` and `SMTP_PASSWORD`

### System Status

- [ ] Add `credential_storage` section to `GET /api/v1/admin/status` (encryption_key_configured, counts, orphaned)

### Logging

- [ ] Add `secrets` log scope
- [ ] Log credential create/rotate/delete/test at `INFO`, failed decryption at `ERROR`
- [ ] Verify no log statement includes credential plaintext, ciphertext, or encoded values

### Packaging

- [ ] Verify all ignore files include `*.pem`, `*.key`, `.env`, `keys/` patterns
- [ ] Verify RPM/DEB `postinstall.sh` sets keys directory to `0700` and env file to `0640`
- [ ] Add `CMM_CREDENTIAL_ENCRYPTION_KEY=` placeholder to env-file and `.env.example` templates
- [ ] Document key generation in Helm chart and Docker Compose READMEs

---

## Configuration

- [ ] Define configuration file format and schema
- [ ] Support configuration of multiple Chef server organisations (URL, org name, client name, key path)
- [ ] Support configuration of target Chef Client versions
- [ ] Support configuration of git base URLs
- [ ] Support configuration of disk space threshold for upgrade readiness
- [ ] Support configuration of collection schedule
- [ ] Support configuration of stale node threshold (`collection.stale_node_threshold_days`, default: 7)
- [ ] Support configuration of stale cookbook threshold (`collection.stale_cookbook_threshold_days`, default: 365)
- [ ] Support configuration of datastore connection
- [ ] Support environment variable overrides for secrets
- [ ] Support configuration of notification channels (webhook and email)
- [ ] Support configuration of notification event triggers and filters
- [ ] Support configuration of readiness milestone thresholds
- [ ] Support configuration of stale node alert count threshold
- [ ] Support configuration of SMTP settings for email notifications
- [ ] Support configuration of export settings (max_rows, async_threshold, output_directory, retention_hours)
- [ ] Support configuration of `analysis_tools.embedded_bin_dir` (default: `/opt/chef-migration-metrics/embedded/bin`)
- [ ] Support configuration of `analysis_tools.cookstyle_timeout_minutes` (default: 10)
- [ ] Support configuration of `analysis_tools.test_kitchen_timeout_minutes` (default: 30)
- [ ] Support configuration of `elasticsearch.enabled` (default: false)
- [ ] Support configuration of `elasticsearch.output_directory` (default: `/var/lib/chef-migration-metrics/elasticsearch`)
- [ ] Support configuration of `elasticsearch.retention_hours` (default: 48)
- [ ] Validate notification channel configuration on startup (url_env set, SMTP configured for email channels)
- [ ] Validate export output directory exists and is writable on startup
- [ ] Validate `analysis_tools.embedded_bin_dir` exists if set (warn, not fatal — falls back to PATH)
- [ ] Validate `analysis_tools.cookstyle_timeout_minutes` >= 1
- [ ] Validate `analysis_tools.test_kitchen_timeout_minutes` >= 1
- [ ] Validate `elasticsearch.output_directory` exists and is writable when `elasticsearch.enabled` is true

### TLS and Certificate Management
- [x] Write TLS and certificate management specification (`tls/Specification.md`)
- [x] Update configuration specification with `server.tls.mode` (off/static/acme) replacing boolean `server.tls.enabled`
- [x] Add full ACME configuration schema to configuration specification (`server.tls.acme.*`)
- [x] Add static certificate settings to configuration specification (`cert_path`, `key_path`, `ca_path`, `min_version`)
- [x] Add HTTP-to-HTTPS redirect listener setting (`server.tls.http_redirect_port`)
- [x] Add environment variable overrides for all TLS settings to configuration specification
- [x] Add TLS validation rules to configuration specification (static mode, ACME mode, backward compatibility)
- [x] Document backward compatibility with deprecated `server.tls.enabled` boolean
- [x] Update full example configuration with TLS mode and commented-out static/ACME examples
- [x] Update Helm chart values.yaml in packaging specification with TLS mode, static cert, and ACME settings
- [x] Add `tlsSecret` Helm values for mounting static TLS certificates from Kubernetes Secrets
- [x] Add `acmeStorage` Helm values for ACME certificate storage PVC
- [x] Add TLS specification to the specifications index in the top-level specification
- [x] Add TLS specification directory to Structure.md with description
- [x] Add TLS specification relationships to the specification relationship graph in Structure.md
- [ ] Implement `server.tls.mode` configuration parsing (off, static, acme)
- [ ] Implement plain HTTP listener when `mode: off`
- [ ] Implement HTTPS listener with static certificate/key when `mode: static`
- [ ] Implement certificate + key validation on startup (file exists, readable, valid pair)
- [ ] Log `WARN` on startup if static certificate is expired
- [ ] Implement automatic certificate reload on `SIGHUP` signal
- [ ] Implement filesystem watching for certificate file changes (e.g. `fsnotify`)
- [ ] Gracefully handle certificate reload failure (continue serving with previous certificate, log `ERROR`)
- [ ] Implement `min_version` enforcement (TLS 1.2 and 1.3 only)
- [ ] Implement mutual TLS (mTLS) via `ca_path` in static mode
- [ ] Log `WARN` on startup if key file permissions are more permissive than `0600`
- [ ] Implement HTTP-to-HTTPS redirect listener on `http_redirect_port`
- [ ] Ensure redirect listener serves only redirects (no API, no static assets, no health checks)
- [ ] Add `Strict-Transport-Security` (HSTS) header on all HTTPS responses when TLS is active
- [ ] Log TLS mode selection and certificate details at `INFO` level on startup
- [ ] Implement ACME client integration (CertMagic or `autocert` — CertMagic recommended)
- [ ] Implement ACME HTTP-01 challenge handler on the redirect listener
- [ ] Implement ACME TLS-ALPN-01 challenge handler on the main HTTPS listener
- [ ] Implement ACME DNS-01 challenge support with pluggable DNS provider interface
- [ ] Implement DNS-01 provider: Amazon Route 53
- [ ] Implement DNS-01 provider: Cloudflare
- [ ] Implement DNS-01 provider: Google Cloud DNS
- [ ] Implement DNS-01 provider: Azure DNS
- [ ] Implement DNS-01 provider: RFC 2136 (Dynamic DNS / TSIG)
- [ ] Implement ACME certificate storage to `acme.storage_path` with correct permissions (0700/0600)
- [ ] Implement automatic certificate renewal before expiry (`renew_before_days`)
- [ ] Implement exponential backoff on ACME renewal failure (1h → 24h cap)
- [ ] Log ACME certificate obtained/renewed at `INFO`, renewal failure at `ERROR`
- [ ] Log `WARN` when certificate is within 7 days of expiry and renewal has not succeeded
- [ ] Send `certificate_expiry_warning` notification event when certificate is near expiry
- [ ] Implement `agree_to_tos` gate — refuse to start in ACME mode unless `true`
- [ ] Log `WARN` when ACME staging CA URL is detected
- [ ] Implement multi-replica coordination for ACME via file-based locking in `storage_path`
- [ ] Implement OCSP stapling for ACME-obtained certificates
- [ ] Implement backward compatibility: treat `tls.enabled: true` as `mode: static` with deprecation warning
- [ ] Validate all ACME settings on startup (domains, email, agree_to_tos, storage_path, challenge, dns_provider)
- [ ] Validate `http_redirect_port` is set when `challenge: http-01`
- [ ] Update `healthcheck` CLI subcommand to support HTTPS with `--insecure` flag for TLS skip-verify
- [ ] Add TLS-related entries to the logging specification (`tls` log scope)
- [ ] Add `certificate_expiry_warning` to the notification events list

---

## Packaging and Deployment

### Build Tooling
- [x] Create `Makefile` with `build`, `build-all`, `build-frontend`, `build-embedded`, `build-embedded-amd64`, `build-embedded-arm64`, `test`, `lint`, `package-rpm`, `package-deb`, `package-docker`, `package-all` targets
- [x] Implement build-time version injection via `-ldflags`
- [ ] Implement `--version` CLI flag

### Embedded Ruby Environment
- [ ] Create `make build-embedded` target that builds the self-contained Ruby environment using a Docker container (`ruby:3.2-bookworm`)
- [ ] Install `cookstyle`, `test-kitchen`, and `kitchen-dokken` gems (with `--no-document`) into isolated prefix
- [ ] Create binstubs (`cookstyle`, `kitchen`) with shebangs pointing to `/opt/chef-migration-metrics/embedded/bin/ruby`
- [ ] Copy Ruby interpreter and shared libraries into the prefix
- [ ] Export the embedded tree to `./build/embedded/` for nFPM packaging
- [ ] Create `make build-embedded-amd64` target for cross-platform build (linux/amd64)
- [ ] Create `make build-embedded-arm64` target for cross-platform build (linux/arm64)
- [ ] Verify embedded `cookstyle --version` runs successfully in isolation (no system Ruby required)
- [ ] Verify embedded `kitchen version` runs successfully in isolation (no system Ruby required)
- [ ] Verify embedded tools do not interfere with a system Ruby or Chef Workstation installation

### RPM Package
- [ ] Create `nfpm.yaml` configuration for RPM and DEB builds
- [ ] Create systemd unit file (`deploy/pkg/chef-migration-metrics.service`)
- [ ] Create default config file for packages (`deploy/pkg/config.yml`)
- [ ] Create environment file for systemd (`deploy/pkg/env-file`)
- [ ] Create preinstall script (service account creation)
- [ ] Create postinstall script (directory ownership, systemd enable)
- [ ] Create preremove script (stop and disable on removal)
- [ ] Build and test RPM package (`make package-rpm`)

### DEB Package
- [ ] Verify DEB package builds from the same `nfpm.yaml` (`make package-deb`)
- [ ] Verify Debian-convention environment file path (`/etc/default/`)
- [ ] Verify preinst uses `adduser --system` for service account creation

### Container Image
- [ ] Create multi-stage `Dockerfile` (Go build stage + Ruby build stage + runtime stage)
- [ ] Verify static binary build with `CGO_ENABLED=0`
- [ ] Implement Ruby build stage using `ruby:3.2-bookworm` — install gems into isolated prefix, create binstubs, copy interpreter and shared libraries
- [ ] Copy embedded Ruby tree from Ruby build stage into runtime image at `/opt/chef-migration-metrics/embedded/`
- [ ] Install runtime shared library dependencies in runtime stage (`libyaml-0-2`, `libffi8`, `libgmp10`, `zlib1g`)
- [ ] Include OCI-standard image labels
- [ ] Create non-root runtime user in the image
- [ ] Include `HEALTHCHECK` instruction using the `healthcheck` subcommand
- [ ] Build and test container image (`make package-docker`)
- [ ] Verify embedded `cookstyle` and `kitchen` work inside the container
- [ ] Implement container image tagging strategy (semver, major, minor, latest, commit SHA)
- [ ] Remove `Dockerfile.analysis` — single image now includes embedded analysis tools
- [ ] Create `/etc/chef-migration-metrics/tls/` directory in the container image for static TLS certificate mounts
- [ ] Create `/var/lib/chef-migration-metrics/acme/` directory in the container image for ACME certificate storage

### Docker Compose
- [ ] Create `deploy/docker-compose/docker-compose.yml` with `app` and `db` services
- [ ] Create `deploy/docker-compose/config.yml` example configuration
- [ ] Create `deploy/docker-compose/.env.example` with documented variables
- [ ] Create `deploy/docker-compose/README.md` with quick-start instructions
- [ ] Verify `docker compose up -d` brings up a working stack from scratch
- [ ] Verify application connects to the Compose-managed PostgreSQL
- [ ] Verify `docker compose down -v` cleanly removes all resources

### ELK Testing Stack
- [ ] Create `deploy/elk/docker-compose.yml` with Elasticsearch, Logstash, and Kibana services
- [ ] Create `deploy/elk/logstash/pipeline/chef-migration-metrics.conf` Logstash pipeline definition
- [ ] Create `deploy/elk/.env.example` with documented variables (ELK version, ports, volume paths)
- [ ] Create `deploy/elk/README.md` with quick-start instructions
- [ ] Configure Logstash to read `*.ndjson` files from shared volume (skip `.tmp` suffix)
- [ ] Configure Logstash to extract `doc_id` as Elasticsearch `_id` for upsert behaviour
- [ ] Configure Logstash to index all document types into single `chef-migration-metrics` index
- [ ] Configure Elasticsearch with security disabled for local testing (`xpack.security.enabled=false`)
- [ ] Configure shared volume (`es_export_data`) between application and Logstash
- [ ] Verify `docker compose up -d` in `deploy/elk/` brings up a working ELK stack
- [ ] Verify Logstash picks up NDJSON files and indexes them into Elasticsearch
- [ ] Verify Kibana can query the `chef-migration-metrics` index
- [ ] Verify `docker compose down -v` cleanly removes all ELK resources
- [ ] Keep Logstash pipeline definition up to date when document types change

### Helm Chart
- [ ] Create `deploy/helm/chef-migration-metrics/Chart.yaml`
- [ ] Create `deploy/helm/chef-migration-metrics/values.yaml` with full default values
- [ ] Create `deploy/helm/chef-migration-metrics/README.md` with usage instructions
- [ ] Implement `templates/_helpers.tpl` with standard label and name helpers
- [ ] Implement `templates/deployment.yaml` with config mount, secret env injection, PVC mount, probes
- [ ] Implement `templates/service.yaml`
- [ ] Implement `templates/ingress.yaml` (conditional on `ingress.enabled`)
- [ ] Implement `templates/configmap.yaml` to render application config from values
- [ ] Implement `templates/secret.yaml` for database URL, LDAP password, and Chef API keys
- [ ] Implement `templates/serviceaccount.yaml`
- [ ] Implement `templates/hpa.yaml` (conditional on `autoscaling.enabled`)
- [ ] Implement `templates/pvc.yaml` for persistent git working directory
- [ ] Implement `templates/NOTES.txt` with post-install usage instructions
- [ ] Implement `templates/tests/test-connection.yaml` Helm test
- [ ] Add Bitnami PostgreSQL subchart dependency (`condition: postgresql.enabled`)
- [ ] Run `helm dependency build` and verify subchart is pulled
- [ ] Run `helm lint` and fix any issues
- [ ] Run `helm template` and verify rendered manifests
- [ ] Test `helm install` against a local or test Kubernetes cluster
- [ ] Verify auto-constructed `DATABASE_URL` when using the PostgreSQL subchart
- [ ] Verify `existingSecret` and `existingConfigMap` overrides work
- [ ] Verify `chefKeys.existingSecret` mounts correctly
- [ ] Verify advisory lock prevents duplicate collection runs with `replicaCount > 1`
- [ ] Package chart with `helm package` for distribution
- [ ] Implement `tlsSecret` support in Deployment template — mount `existingSecret` or chart-managed TLS Secret to `/etc/chef-migration-metrics/tls/`
- [ ] Implement chart-managed TLS Secret from inline `tlsSecret.cert` and `tlsSecret.key` values
- [ ] Implement ACME storage PVC template (conditional on `server.tls.mode == acme`)
- [ ] Mount ACME storage PVC at `acme.storage_path` in Deployment when ACME mode is active
- [ ] Update liveness/readiness probes to use HTTPS scheme when `server.tls.mode` is `static` or `acme`
- [ ] Verify Helm chart renders correctly with `tls.mode: off` (default — no TLS resources created)
- [ ] Verify Helm chart renders correctly with `tls.mode: static` and `tlsSecret.existingSecret`
- [ ] Verify Helm chart renders correctly with `tls.mode: acme` and ACME storage PVC

### CI/CD
- [x] Set up CI pipeline stage for lint (Go, frontend, Helm)
- [x] Set up CI pipeline stage for test (Go, frontend)
- [x] Set up CI pipeline stage for build (binary, frontend, embedded Ruby environment)
- [x] Set up CI pipeline stage for package (RPM, DEB, container image — all including embedded tools)
- [x] Set up CI pipeline stage for publish (container registry, release artifacts, Helm chart)
- [x] Implement release workflow triggered by `v*` tags

---

## Testing

- [ ] Unit tests for Chef API client and authentication
- [ ] Unit tests for partial search query builder
- [ ] Unit tests for cookbook usage analysis
- [ ] Unit tests for cookbook usage analysis with Policyfile nodes
- [ ] Unit tests for node readiness calculation
- [ ] Unit tests for stale node detection logic
- [ ] Unit tests for stale cookbook detection logic
- [ ] Unit tests for auto-correct preview generation (diff computation, statistics)
- [ ] Unit tests for cop-to-documentation mapping enrichment
- [ ] Unit tests for cookbook complexity score calculation (weighted scoring, label classification)
- [ ] Unit tests for blast radius computation (node count, role count via dependency graph, policy count)
- [ ] Unit tests for CookStyle version profile selection per target Chef Client version
- [ ] Unit tests for role dependency graph building (role → role, role → cookbook parsing)
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

---

## Documentation

- [ ] Document installation and deployment
- [ ] Document installation via RPM package
- [ ] Document installation via DEB package
- [ ] Document installation via container image (Docker)
- [ ] Document local development with Docker Compose
- [ ] Document Kubernetes deployment with Helm chart
- [ ] Document configuration reference
- [ ] Document Chef server API credentials setup
- [ ] Document git repository URL configuration
- [ ] Document authentication provider setup (SAML, LDAP, local)
- [ ] Document Policyfile support (what is collected, how to filter by policy name/group)
- [ ] Document stale node and stale cookbook detection (thresholds, dashboard indicators)
- [ ] Document remediation guidance features (auto-correct preview, migration docs, complexity scoring)
- [ ] Document dependency graph view (how to read the graph, filtering, table alternative)
- [ ] Document data export functionality (formats, Chef search query usage with knife ssh)
- [ ] Document notification configuration (webhook setup for Slack/Teams/PagerDuty, email/SMTP setup, trigger configuration)
- [ ] Document confidence indicators (high vs. medium vs. untested)
- [ ] Document cookbook complexity scoring model (weights, labels, blast radius)
- [ ] Document embedded analysis tools (CookStyle, Test Kitchen embedded in all packages, no Chef Workstation required, Docker is only external dep for TK)
- [ ] Document `analysis_tools` configuration section (embedded_bin_dir, timeouts, PATH fallback for dev environments)
- [ ] Document Elasticsearch export setup (enable in config, configure output directory, start ELK stack)
- [ ] Document ELK testing stack usage (`deploy/elk/` Docker Compose, Kibana dashboard creation, suggested visualisations)
- [ ] Document Logstash pipeline configuration and how to keep it in sync with document types
- [ ] Document building the embedded Ruby environment from source (`make build-embedded`)
- [ ] Document contributing guidelines