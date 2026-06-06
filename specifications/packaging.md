# Packaging and Deployment - Component Specification

> Component specification for the packaging and deployment of Chef Migration Metrics.
> See the [top-level specification](overview.md) for project overview and scope.

---

## TL;DR

This spec covers how the application is packaged and deployed across all supported formats. Key points:

- **Packaging formats:** RPM, DEB, and distribution archives — all built from the same Go binary + embedded frontend assets.
- **Embedded Ruby:** All packages ship a self-contained Ruby runtime under `/opt/chef-migration-metrics/embedded/` with CookStyle, Test Kitchen, and kitchen-dokken pre-installed. No external Ruby or Chef Workstation required.
- **Systemd integration:** RPM/DEB packages include a systemd unit file, pre/post install scripts, and environment file.
- **Docker Compose:** Local dev stack with app + PostgreSQL (`deploy/docker-compose/`).
- **ELK testing stack:** Elasticsearch + Logstash + Kibana for testing NDJSON export (`deploy/elk/`).
- **CI/CD:** GitHub Actions workflows for CI (`ci.yml`) and release (`release.yml`) — lint, test, build, package.

---

## Overview

Chef Migration Metrics is distributed as native Linux packages (RPM and DEB) and as pre-built distribution archives. Docker Compose is used for local development (database, ELK stack) but the application itself is not published as a container image.

All packaging artifacts are built from the same Go binary and embedded frontend assets. The packaging layer adds platform-specific integration (systemd, file layout, default configuration) around the single compiled binary.

All packaging formats **embed** CookStyle, Test Kitchen, and a self-contained Ruby runtime so that cookbook compatibility testing works out of the box with no external dependencies on Chef Workstation or system Ruby. The embedded tools are installed under `/opt/chef-migration-metrics/embedded/` and are isolated from any other Ruby installation on the host.

---

## 1. Build Artifacts

Moved to [packaging-build-artifacts.md](packaging-build-artifacts.md).

---

## 2. RPM Package

Moved to [packaging-rpm-deb.md](packaging-rpm-deb.md).

---

## 3. DEB Package

Moved to [packaging-rpm-deb.md](packaging-rpm-deb.md).

---

## 4. Docker Compose

Moved to [packaging-docker-compose.md](packaging-docker-compose.md).

---

## 5. CI/CD Integration

Moved to [packaging-ci-cd-repository-layout.md](packaging-ci-cd-repository-layout.md).

---

## 6. nFPM Configuration

Moved to [packaging-nfpm.md](packaging-nfpm.md).

---

## 7. Repository Layout for Packaging Files

Moved to [packaging-ci-cd-repository-layout.md](packaging-ci-cd-repository-layout.md).

---

## Related Specifications

- [Top-level Specification](overview.md)
- [Configuration Specification](configuration.md)
- [Analysis Specification](analysis.md) — startup validation for external tools
- [Data Collection Specification](data-collection.md) — background job serialisation
- [Web API Specification](web-api.md) — health endpoint used by status checks
- [Datastore Specification](datastore.md) — advisory locks for collection serialisation
