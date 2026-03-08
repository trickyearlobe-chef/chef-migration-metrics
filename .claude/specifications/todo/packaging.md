# Packaging and Deployment — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Build Tooling

- [x] Create `Makefile` with `build`, `build-all`, `build-frontend`, `build-embedded`, `build-embedded-amd64`, `build-embedded-arm64`, `test`, `lint`, `package-rpm`, `package-deb`, `package-docker`, `package-all` targets
- [x] Implement build-time version injection via `-ldflags`
- [x] Implement `--version` CLI flag — `main.go` supports `-version` flag with build-time version injection via `-ldflags`

## Embedded Ruby Environment

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

## RPM Package

- [ ] Create `nfpm.yaml` configuration for RPM and DEB builds
- [ ] Create systemd unit file (`deploy/pkg/chef-migration-metrics.service`)
- [ ] Create default config file for packages (`deploy/pkg/config.yml`)
- [ ] Create environment file for systemd (`deploy/pkg/env-file`)
- [ ] Create preinstall script (service account creation)
- [ ] Create postinstall script (directory ownership, systemd enable)
- [ ] Create preremove script (stop and disable on removal)
- [ ] Build and test RPM package (`make package-rpm`)

## DEB Package

- [ ] Verify DEB package builds from the same `nfpm.yaml` (`make package-deb`)
- [ ] Verify Debian-convention environment file path (`/etc/default/`)
- [ ] Verify preinst uses `adduser --system` for service account creation

## Container Image

- [x] Create multi-stage `Dockerfile` (unified build stage + runtime stage) — two-stage build: `ruby:3.1-bookworm` (with Go toolchain added) → `debian:bookworm-slim`; conditional frontend build; version injection via `ARG`/`-ldflags`. Ruby 3.1 matches Chef Workstation 25.13.7 (ships Ruby 3.1.7).
- [x] Verify static binary build with `CGO_ENABLED=0` — `CGO_ENABLED=0 GOOS=linux go build` with `-s -w` strip flags in the Go build stage
- [x] Implement Ruby build stage using `ruby:3.1-bookworm` — gem versions pinned to Chef Workstation 25.13.7 (`components/gems/Gemfile.lock`); `ffi:1.16.3` installed first as ecosystem-wide constraint; `GEM_HOME` set to `/opt/chef-migration-metrics/embedded/lib/ruby/gems/3.1.0`; shell-wrapper binstubs source `env.sh` for `RUBYLIB`/`GEM_HOME`/`GEM_PATH`
- [x] Install all kitchen drivers: `kitchen-dokken:2.22.2` (Stromweld fork), `kitchen-vagrant:2.2.0`, `kitchen-ec2:3.22.1`, `kitchen-azurerm:1.13.6`, `kitchen-google:2.6.1`, `kitchen-hyperv:0.10.3`, `kitchen-vcenter:2.12.2`, `kitchen-vra:3.3.3`, `kitchen-openstack:6.2.1`, `kitchen-digitalocean:0.16.1`
- [x] Install kitchen-inspec verifier (`kitchen-inspec:3.1.0`) and InSpec (`inspec-bin:5.24.7`)
- [x] Install legacy busser verifier for older cookbook repos: `busser:0.8.0`, `busser-serverspec:0.6.3`, `busser-bats:0.5.0` — installed with `--force` because busser 0.8.0 requires `thor <= 0.19.0` which conflicts with thor 1.4.0 needed by test-kitchen/inspec/cookstyle; safe because busser uses thor only for its own CLI and TK manages busser internally without invoking the CLI directly
- [x] Replace `kitchen-vsphere` with `kitchen-vcenter:2.12.2` — `kitchen-vsphere` 0.2.0 (2015) requires `test-kitchen ~> 1.0` and is incompatible with TK 3.x at both dependency and API level; `kitchen-vcenter` is the maintained vSphere driver using `rbvmomi2` and the REST API
- [x] Copy embedded Ruby tree from build stage into runtime image at `/opt/chef-migration-metrics/embedded/` — `COPY --from=builder`; `ldconfig` registers embedded `libruby.so`
- [x] Install runtime shared library dependencies in runtime stage (`libyaml-0-2`, `libffi8`, `libgmp10`, `zlib1g`, `libxml2`, `libxslt1.1`, `libssl3`, `libgcc-s1`, `libreadline8`) — plus `ca-certificates`, `git`, `openssh-client`
- [x] Include OCI-standard image labels — `org.opencontainers.image.{title,description,version,revision,created,source,licenses,vendor}` using re-declared `ARG`s
- [x] Create non-root runtime user in the image — `chef-migration-metrics` system user/group via `groupadd -r` / `useradd -r`
- [x] Include `HEALTHCHECK` instruction using the `healthcheck` subcommand — `--interval=30s --timeout=5s --start-period=60s --retries=3`
- [ ] Build and test container image (`make package-docker`)
- [ ] Verify embedded `cookstyle`, `kitchen`, and `inspec` work inside the container — build-time sanity checks assert exact versions (cookstyle 7.32.8, test-kitchen 3.9.1, inspec-core 5.24.7, kitchen-inspec 3.1.0, ffi 1.16.3) and require-test all kitchen drivers
- [ ] Implement container image tagging strategy (semver, major, minor, latest, commit SHA)
- [x] Remove `Dockerfile.analysis` — single image now includes embedded analysis tools — no `Dockerfile.analysis` exists; the single `Dockerfile` embeds all analysis tools via the Ruby build stage
- [x] Create `/etc/chef-migration-metrics/tls/` directory in the container image for static TLS certificate mounts — created in the filesystem layout `RUN mkdir -p` step
- [x] Create `/var/lib/chef-migration-metrics/acme/` directory in the container image for ACME certificate storage — created in the filesystem layout `RUN mkdir -p` step

## Docker Compose

- [x] Create `deploy/docker-compose/docker-compose.yml` with `app` and `db` services — `app` (chef-migration-metrics:latest, build from root Dockerfile, `DATABASE_URL` injected, config + keys mounted read-only, `cookbook_data` volume, depends on `db` health check) and `db` (postgres:16-bookworm, `pgdata` volume, `pg_isready` health check); all ports and credentials configurable via `.env`
- [x] Create `deploy/docker-compose/config.yml` example configuration — pre-configured for Docker Compose with `DATABASE_URL` env override, embedded bin dir at `/opt/chef-migration-metrics/embedded/bin`, example organisation with key path, target chef versions, collection schedule, concurrency, readiness, server, and commented-out optional sections (elasticsearch, notifications, SMTP, exports, auth)
- [x] Create `deploy/docker-compose/.env.example` with documented variables — `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` (required), `POSTGRES_PORT`, `APP_IMAGE`, `APP_PORT`, build args (`VERSION`, `GIT_COMMIT`, `BUILD_DATE`), `LDAP_BIND_PASSWORD`, `CMM_CREDENTIAL_ENCRYPTION_KEY`
- [x] Create `deploy/docker-compose/README.md` with quick-start instructions — prerequisites, 4-step quick start, services table, files table, environment variable reference, config guidance, Chef API key mounting, common operations (logs, health, rebuild, psql, stop, reset), volumes, port conflicts, ELK integration, and troubleshooting section
- [ ] Verify `docker compose up -d` brings up a working stack from scratch
- [ ] Verify application connects to the Compose-managed PostgreSQL
- [ ] Verify `docker compose down -v` cleanly removes all resources

## ELK Testing Stack

- [x] Create `deploy/elk/docker-compose.yml` with Elasticsearch, Logstash, and Kibana services — fully implemented with health checks, configurable ports/versions via env vars, shared `es_export_data` volume, sincedb persistence, and `unless-stopped` restart policy
- [x] Create `deploy/elk/logstash/pipeline/chef-migration-metrics.conf` Logstash pipeline definition — reads `*.ndjson` (excludes `.tmp`), parses JSON, validates envelope fields (`doc_type`, `doc_id`), preserves app-set `@timestamp`, parses all ISO 8601 date fields, uses `doc_id` as Elasticsearch `_id` for upsert, applies index template
- [x] Create `deploy/elk/.env.example` with documented variables (ELK version, ports, volume paths)
- [x] Create `deploy/elk/README.md` with quick-start instructions — 621 lines covering architecture, prerequisites, quick start, connecting to the app, Kibana setup, 19 dashboard visualisations, document types, Logstash pipeline details, operations, and troubleshooting
- [x] Configure Logstash to read `*.ndjson` files from shared volume (skip `.tmp` suffix) — `file` input with `exclude => "*.tmp"`, `mode => "tail"`, `start_position => "beginning"`, `sincedb_write_interval => 5`
- [x] Configure Logstash to extract `doc_id` as Elasticsearch `_id` for upsert behaviour — `mutate { copy => { "doc_id" => "[@metadata][_id]" } }` and `document_id => "%{[@metadata][_id]}"` in output
- [x] Configure Logstash to index all document types into single `chef-migration-metrics` index — `index => "${ELASTICSEARCH_INDEX:chef-migration-metrics}"` with `manage_template => true` and custom index template (`chef-migration-metrics-template.json`)
- [x] Configure Elasticsearch with security disabled for local testing (`xpack.security.enabled=false`) — plus `xpack.security.enrollment.enabled=false`, `xpack.security.http.ssl.enabled=false`, `xpack.security.transport.ssl.enabled=false`
- [x] Configure shared volume (`es_export_data`) between application and Logstash — named volume with documentation for bind-mount override and host directory options
- [ ] Verify `docker compose up -d` in `deploy/elk/` brings up a working ELK stack
- [ ] Verify Logstash picks up NDJSON files and indexes them into Elasticsearch
- [ ] Verify Kibana can query the `chef-migration-metrics` index
- [ ] Verify `docker compose down -v` cleanly removes all ELK resources
- [ ] Keep Logstash pipeline definition up to date when document types change

## Helm Chart

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

## CI/CD

- [x] Set up CI pipeline stage for lint (Go, frontend, Helm)
- [x] Set up CI pipeline stage for test (Go, frontend)
- [x] Set up CI pipeline stage for build (binary, frontend, embedded Ruby environment)
- [x] Set up CI pipeline stage for package (RPM, DEB, container image — all including embedded tools)
- [x] Set up CI pipeline stage for publish (container registry, release artifacts, Helm chart)
- [x] Implement release workflow triggered by `v*` tags