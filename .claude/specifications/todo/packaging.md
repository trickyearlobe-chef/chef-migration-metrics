# Packaging and Deployment — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Build Tooling

- [x] Create `Makefile` with `build`, `build-all`, `build-frontend`, `build-embedded`, `build-embedded-amd64`, `build-embedded-arm64`, `test`, `lint`, `package-rpm`, `package-deb`, `package-docker`, `package-all` targets
- [x] Implement build-time version injection via `-ldflags`
- [ ] Implement `--version` CLI flag

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

## Docker Compose

- [ ] Create `deploy/docker-compose/docker-compose.yml` with `app` and `db` services
- [ ] Create `deploy/docker-compose/config.yml` example configuration
- [ ] Create `deploy/docker-compose/.env.example` with documented variables
- [ ] Create `deploy/docker-compose/README.md` with quick-start instructions
- [ ] Verify `docker compose up -d` brings up a working stack from scratch
- [ ] Verify application connects to the Compose-managed PostgreSQL
- [ ] Verify `docker compose down -v` cleanly removes all resources

## ELK Testing Stack

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