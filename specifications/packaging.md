# Packaging and Deployment - Component Specification

> Component specification for the packaging and deployment of Chef Migration Metrics.
> See the [top-level specification](overview.md) for project overview and scope.

---

## TL;DR

This spec covers how the application is packaged and deployed across all supported formats. Key points:

- **Packaging formats:** RPM, DEB, and distribution archives — all built from the same Go binary + embedded frontend assets.
- **External tools:** CookStyle and Test Kitchen are **not** bundled — they are provided by [Chef Workstation](https://docs.chef.io/workstation/) on the host and resolved from `PATH`. Packages ship only the Go binary, config, systemd unit, and env-file.
- **Systemd integration:** RPM/DEB packages include a systemd unit file, pre/post install scripts, and environment file.
- **Docker Compose:** Local dev stack with app + PostgreSQL (`deploy/docker-compose/`).
- **ELK testing stack:** Elasticsearch + Logstash + Kibana for testing NDJSON export (`deploy/elk/`).
- **CI/CD:** GitHub Actions workflows for CI (`ci.yml`) and release (`release.yml`) — lint, test, build, package.

---

## Overview

Chef Migration Metrics is distributed as native Linux packages (RPM and DEB) and as pre-built distribution archives. Docker Compose is used for local development (database, ELK stack) but the application itself is not published as a container image.

All packaging artifacts are built from the same Go binary and embedded frontend assets. The packaging layer adds platform-specific integration (systemd, file layout, default configuration) around the single compiled binary.

Cookbook compatibility testing (CookStyle, Test Kitchen) requires **Chef Workstation** on the host — these tools are **not** bundled (an embedded Ruby runtime was tried but dropped as too unreliable to build). The application resolves `cookstyle` and `kitchen` from `PATH`; when they are absent, compatibility analysis is skipped and the rest of the product still works.

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
