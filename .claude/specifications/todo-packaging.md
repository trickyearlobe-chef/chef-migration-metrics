# Packaging and Deployment — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

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

- [ ] Build and test container image (`make package-docker`)
- [ ] Verify embedded `cookstyle`, `kitchen`, and `inspec` work inside the container — build-time sanity checks assert exact versions (cookstyle 7.32.8, test-kitchen 3.9.1, inspec-core 5.24.7, kitchen-inspec 3.1.0, ffi 1.16.3) and require-test all kitchen drivers
- [ ] Implement container image tagging strategy (semver, major, minor, latest, commit SHA)

## Docker Compose

- [ ] Verify `docker compose up -d` brings up a working stack from scratch
- [ ] Verify application connects to the Compose-managed PostgreSQL
- [ ] Verify `docker compose down -v` cleanly removes all resources

## ELK Testing Stack

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