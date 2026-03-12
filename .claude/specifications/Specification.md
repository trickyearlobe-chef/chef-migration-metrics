# Chef Migration Metrics - Project Specification

> **TL;DR:** Open source (Apache 2.0) Go + React tool for planning Chef Client upgrade projects. Collects node data from Chef Infra Server orgs via partial search, fetches cookbooks from git and Chef server, tests compatibility with target Chef Client versions using embedded CookStyle and Test Kitchen, computes per-node upgrade readiness (cookbook compatibility + disk space), and generates remediation guidance (auto-correct previews, migration docs, complexity scores). Supports Policyfile and classic nodes, stale detection, role dependency graphs, data exports, webhook/email notifications, and historical trending. PostgreSQL backend. Packaged as RPM, DEB, container image, and Helm chart. See individual component specs for detail — each has a TL;DR at the top.

## Overview

Chef Migration Metrics is an open source (Apache 2.0) tool to help organisations plan and track Chef Client upgrade projects. It collects data from Chef Infra Servers, analyses cookbook compatibility with target Chef Client versions, and visualises progress through a web dashboard.

The primary upgrade use case is the periodic upgrade of Chef Client versions across a node fleet. From Chef Client version 19 onwards, the packaging format changed from RPMs, DEBs and MSIs to Habitat bundles. Habitat bundles are significantly larger than the previous packaging formats, so disk space availability is a key factor in determining upgrade readiness. InSpec, previously a separate package, is now bundled with Chef Client and must also be accounted for.

## Scope

### In Scope

- Tracking Chef Client versions in use across nodes over time
- Assessing cookbook compatibility with declared Chef Client versions
- Determining node upgrade readiness based on:
  - Cookbook compatibility with the target Chef Client version
  - Available disk space on the node
- Collecting node data from one or more Chef Infra Server organisations
- Supporting both classic (role/run-list) and Policyfile-based node management
- Fetching cookbooks from git repositories and/or directly from the Chef server
- Testing cookbooks against multiple Chef Client versions using Test Kitchen and CookStyle, both embedded in all packaging formats along with a self-contained Ruby runtime
- Generating remediation guidance for incompatible cookbooks (auto-correct previews, migration documentation links, complexity scores)
- Mapping role-to-cookbook dependency graphs to understand blast radius of incompatible cookbooks
- Detecting stale nodes (nodes that have not checked in recently) and stale cookbooks (cookbooks not updated in a long time)
- Exporting ready/blocked node lists and remediation reports for use in external upgrade automation workflows
- Sending notifications (webhook, email) when significant events occur (cookbook status changes, readiness milestones, collection failures)
- Visualising metrics in a web dashboard with interactive filters, drill-downs, and remediation guidance

### Out of Scope

- Migration of organisations between Chef servers
- Managing external Ruby, Chef Workstation, or gem installations — CookStyle, Test Kitchen, and their Ruby dependencies are embedded in all packaging formats and require no external toolchain

---

## Components

The system has three logical layers. Each has its own detailed specification with a TL;DR at the top — read the TL;DR first, then load the full spec only if needed.

### 1. Data Collection

Periodic collection of node data (via Chef partial search), cookbooks (from git and Chef server), and role dependency graphs from one or more Chef organisations. Supports Policyfile nodes, stale node/cookbook detection, and fault-tolerant parallel collection.

→ Full spec: [`data-collection/Specification.md`](data-collection/Specification.md), [`chef-api/Specification.md`](chef-api/Specification.md)

### 2. Analysis

Cookbook usage analysis, compatibility testing (Test Kitchen for git-sourced, CookStyle for server-sourced), remediation guidance (auto-correct previews, migration doc links, complexity scoring with blast radius), and per-node upgrade readiness evaluation (cookbook compatibility + disk space). All work parallelised via bounded worker pools.

→ Full spec: [`analysis/Specification.md`](analysis/Specification.md)

### 3. Data Visualisation

React web dashboard with: version distribution, cookbook compatibility matrix, readiness summary, dependency graph view, remediation priority list. Interactive filters (org, environment, role, policy, platform, target version, stale status, complexity). Data exports (CSV, JSON, Chef search query). Notifications (webhook, email). Historical trending. Integrated log viewer.

→ Full spec: [`visualisation/Specification.md`](visualisation/Specification.md), [`web-api/Specification.md`](web-api/Specification.md)

### Supporting Components

| Component | Spec |
|-----------|------|
| PostgreSQL schema & migrations | [`datastore/Specification.md`](datastore/Specification.md) |
| Configuration (YAML + env vars) | [`configuration/Specification.md`](configuration/Specification.md) |
| Auth (local, LDAP, SAML, RBAC) | [`auth/Specification.md`](auth/Specification.md) |
| TLS (off / static / ACME) | [`tls/Specification.md`](tls/Specification.md) |
| Structured logging | [`logging/Specification.md`](logging/Specification.md) |
| Elasticsearch NDJSON export | [`elasticsearch/Specification.md`](elasticsearch/Specification.md) |
| Packaging (RPM, DEB, container, Helm) | [`packaging/Specification.md`](packaging/Specification.md) |

---

## Configuration

The tool must be configurable to support different deployment environments. Configuration covers:

- **Chef server organisations**: list of organisations, each with:
  - Chef server URL
  - Organisation name
  - Client name and key path for API authentication
- **Target Chef Client versions**: list of versions to test compatibility against.
- **Git base URLs**: list of base URLs for resolving cookbook git repositories.
- **Disk space threshold**: minimum free disk space required for upgrade (default should reflect the Habitat bundle size).
- **Collection schedule**: how frequently the background collection job runs.
- **Stale node threshold**: how many days since last check-in before a node is flagged as stale (default: 7 days).
- **Stale cookbook threshold**: how many days since last version update before a cookbook is flagged as stale (default: 365 days).
- **Datastore connection**: location/credentials for the persistence backend.
- **Log retention period**: how long logs are retained in the datastore before being purged.
- **Log level**: minimum severity level to persist (`DEBUG`, `INFO`, `WARN`, `ERROR`).
- **Notifications**: webhook and email channel configuration, event triggers, readiness milestone thresholds, and SMTP settings.
- **Exports**: maximum export size, async threshold, output directory, and retention period.

---

## Non-Functional Requirements

- **Language**: All backend components must be implemented in **Go**.
- **Database migrations**: All database schema changes must be managed through versioned migration files. Migrations must run automatically on application startup. The migration tool and file conventions are defined in the [Configuration specification](configuration/Specification.md).
- **Concurrency**: Go goroutines must be used to parallelise independent units of work. Key areas include collecting from multiple Chef server organisations, pulling multiple git repositories, running CookStyle scans, and running Test Kitchen tests. Goroutine concurrency must be bounded using worker pools to avoid overwhelming external services or local resources. See `../../Claude.md` for concurrency rules.
- **License**: Apache 2.0. All components must be licensed accordingly.
- **Chef API compliance**: All Chef Infra Server API calls must conform to the specification in [`chef-api/Specification.md`](chef-api/Specification.md) and the upstream reference at https://docs.chef.io/server/api_chef_server.
- **Scalability**: The collection process must remain efficient as the number of nodes and organisations grows. Partial search is mandatory to limit payload size and API load.
- **Reliability**: The background collection job must be fault-tolerant — failures collecting from one organisation must not prevent collection from others.
- **Reliability**: The background collection job must be able to recover from failures and continue collecting data without starting over.
- **Logging**: All components must emit structured logs with consistent severity levels (`DEBUG`, `INFO`, `WARN`, `ERROR`). Logs must be persisted to the datastore and viewable from the web UI log viewer (see section 3.3). Stdout/stderr output from external processes (Test Kitchen, CookStyle, git) must be captured and associated with the relevant log scope.
- **Security**: Chef server credentials (private keys) must never be stored in source control. Configuration must support referencing key files by path or via environment variables.
- **Security**: The Web UI must support authentication and authorization via SAML, LDAP, and local user accounts.
- **Embedded analysis tools**: All packaging formats (RPM, DEB, container image) must ship with a self-contained Ruby environment under `/opt/chef-migration-metrics/embedded/` that includes CookStyle, Test Kitchen, the `kitchen-dokken` driver, and their gem dependencies. This eliminates external dependencies on Chef Workstation or system Ruby. Docker is the only external runtime dependency for Test Kitchen testing. The application falls back to `PATH` lookup when the embedded directory is not present (e.g. development environments and source builds). See [`packaging/Specification.md`](packaging/Specification.md) for the embedded environment build and layout, and [`analysis/Specification.md`](analysis/Specification.md) for tool resolution and startup validation.
- **Packaging**: The application must be distributable as RPM and DEB packages for native Linux installation, and as a container image for containerised deployments. A Docker Compose file must be provided for local development and evaluation. A Helm chart must be provided for Kubernetes deployments. All packaging formats must be built from the same Go binary, embedded frontend assets, and embedded Ruby environment. See [`packaging/Specification.md`](packaging/Specification.md) for the full packaging and deployment specification.

---

## Specifications Index

All component-level specifications are documented as separate files under `.claude/specifications/`:

| File | Description |
|------|-------------|
| `Specification.md` | This file — top-level project specification |
| `data-collection/Specification.md` | Node collection, Policyfile support, cookbook fetching, stale detection, role dependency graph collection |
| `analysis/Specification.md` | Cookbook usage analysis, compatibility testing, remediation guidance, complexity scoring, node readiness |
| `visualisation/Specification.md` | Web dashboard, filters, drill-downs, dependency graph, remediation guidance view, exports, notifications, log viewer, historical trending |
| `logging/Specification.md` | Structured logging, log scopes, retention, external process capture |
| `auth/Specification.md` | Authentication (local, LDAP, SAML) and authorisation (RBAC) |
| `configuration/Specification.md` | Full configuration schema and environment variable overrides |
| `chef-api/Specification.md` | Chef Infra Server API usage, authentication, endpoints, known bugs/quirks |
| `datastore/Specification.md` | Database schema, tables, indexes, relationships, retention, and performance |
| `web-api/Specification.md` | HTTP API endpoints between Go backend and web frontend, auth middleware, pagination |
| `packaging/Specification.md` | RPM, DEB, container image, Docker Compose, Helm chart, CI/CD integration, embedded Ruby environment |
| `tls/Specification.md` | TLS and certificate management — plain HTTP, static certificates, ACME automatic management (Let's Encrypt), challenge types, DNS providers, certificate reload, mTLS |
| `secrets-storage/Specification.md` | Secrets and credential management — three storage methods (database AES-256-GCM encrypted, environment variable, file path), credential resolution precedence, master encryption key management, key rotation, plaintext handling rules, Kubernetes/Helm/RPM/DEB secrets integration |
| `elasticsearch/Specification.md` | Elasticsearch export, Logstash pipeline, ELK testing stack |
| `ownership/Specification.md` | Ownership tracking for nodes, roles, policyfiles, cookbooks, and git repositories — owner model, auto-derivation rules, bulk import, owner-scoped dashboard views and exports |

See also [`chef-api/Specification.md`](chef-api/Specification.md) for the Chef Infra Server API technical specification.