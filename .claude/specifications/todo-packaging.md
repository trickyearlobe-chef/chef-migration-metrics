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

