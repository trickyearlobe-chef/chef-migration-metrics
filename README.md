# Chef Migration Metrics

An open source tool to help organisations plan and track Chef Client upgrade projects. It collects data from Chef Infra Servers, analyses cookbook compatibility with target Chef Client versions, and visualises progress through a web dashboard.

## Overview

Upgrading Chef Client across a large fleet is a significant project. Chef Migration Metrics provides the visibility and automation needed to plan and execute the upgrade with confidence.

### What It Does

- **Tracks Chef Client versions** in use across all nodes, with historical trending
- **Collects node data** from one or more Chef Infra Server organisations via partial search
- **Supports both classic and Policyfile nodes** — collects `policy_name` and `policy_group` alongside traditional roles and run-lists
- **Fetches cookbooks** from git repositories and/or directly from the Chef server
- **Tests cookbook compatibility** against target Chef Client versions using Test Kitchen (git-sourced) and CookStyle (server-sourced)
- **Provides remediation guidance** — auto-correct previews, migration documentation links, and before/after code examples for every deprecation
- **Scores cookbook complexity** — weighted scores and labels (`low`, `medium`, `high`, `critical`) help teams prioritise which cookbooks to fix first
- **Maps dependency graphs** — shows role-to-cookbook relationships so teams understand the blast radius of incompatible cookbooks
- **Assesses node upgrade readiness** based on cookbook compatibility, available disk space, and blocking cookbook complexity
- **Detects stale nodes and cookbooks** — flags nodes that haven't checked in recently and cookbooks that haven't been updated in a long time
- **Exports data** — ready/blocked node lists (CSV, JSON, Chef search query) and remediation reports for use in external upgrade automation workflows
- **Sends notifications** — webhook (Slack, Teams, PagerDuty) and email alerts for cookbook status changes, readiness milestones, collection failures, and more
- **Visualises metrics** in a web dashboard with interactive filters, drill-downs, confidence indicators, and trend charts
- **Captures logs** from all background jobs and external processes, viewable from the web UI

### Why Disk Space Matters

From Chef Client version 19 onwards, the packaging format changed from RPMs, DEBs, and MSIs to Habitat bundles. Habitat bundles are significantly larger than previous packaging formats, and InSpec (previously a separate package) is now bundled with Chef Client. Disk space availability on each node is therefore a key factor in determining upgrade readiness.

### Why Remediation Guidance Matters

Knowing which cookbooks are incompatible is only half the battle — practitioners also need to know **how to fix them**. Chef Migration Metrics generates auto-correct previews (showing exactly what `cookstyle --auto-correct` would change), links each deprecation to its migration documentation with before/after code examples, and assigns complexity scores so teams can identify quick wins and plan for harder remediation work. A cookbook with 2 deprecation warnings is very different from one with 47 — the complexity score makes that distinction actionable.

## Architecture

Chef Migration Metrics is a Go application with the following major components:

```
┌──────────────────────────────────────────────────────────────────┐
│                        Web Dashboard                             │
│  Version distribution · Cookbook compatibility · Node readiness   │
│  Dependency graph · Remediation guidance · Complexity scores     │
│  Confidence indicators · Exports · Notifications · Log viewer    │
│  Interactive filters (org, env, role, policy, platform, stale)   │
└──────────────────────────────┬───────────────────────────────────┘
                           │ HTTP API
┌──────────────────────────┴───────────────────────────────────────┐
│                        Go Backend                                │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐ │
│  │   Data       │  │   Analysis   │  │   Web API / Auth       │ │
│  │  Collection  │  │              │  │                        │ │
│  │              │  │  Cookbook     │  │  REST endpoints        │ │
│  │  Node data   │  │  usage       │  │  SAML / LDAP / local   │ │
│  │  Policyfiles │  │  Test Kitchen│  │  RBAC                  │ │
│  │  Cookbooks   │  │  CookStyle   │  │  Session management    │ │
│  │  Git repos   │  │  Remediation │  │  Exports               │ │
│  │  Role graph  │  │  Complexity  │  │  Notifications         │ │
│  │  Stale nodes │  │  Readiness   │  │                        │ │
│  └──────┬───────┘  └──────┬───────┘  └────────────┬───────────┘ │
│         │                 │                        │             │
│  ┌──────┴─────────────────┴────────────────────────┴───────────┐ │
│  │                    PostgreSQL Datastore                      │ │
│  │  Nodes · Cookbooks · Test results · Remediation · Complexity│ │
│  │  Dependency graph · Logs · Metrics · Notifications · Exports│ │
│  └─────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
         │                                         │
         │  Chef Infra Server API          Git (clone/pull)      Webhooks / Email
         ▼                                 ▼                     ▼
  ┌──────────────┐                  ┌──────────────┐      ┌──────────────┐
  │ Chef Infra   │                  │ Git repos    │      │ Notification │
  │ Server(s)    │                  │ (GitHub,     │      │ Channels     │
  │              │                  │  GitLab,     │      │ (Slack,      │
  │ Org 1        │                  │  etc.)       │      │  Teams,      │
  │ Org 2        │                  │              │      │  Email,      │
  │ ...          │                  │              │      │  PagerDuty)  │
  └──────────────┘                  └──────────────┘      └──────────────┘
```

### Components

| Component | Description |
|-----------|-------------|
| **Data Collection** | Periodically collects node attributes from Chef Infra Server organisations using partial search. Supports both classic and Policyfile nodes. Fetches cookbooks from git repositories and the Chef server. Collects role dependency graphs. Detects stale nodes and cookbooks. |
| **Analysis** | Computes cookbook usage statistics, runs Test Kitchen and CookStyle compatibility tests, generates remediation guidance (auto-correct previews, migration docs), computes complexity scores and blast radius, and evaluates per-node upgrade readiness. |
| **Web Dashboard** | Presents version distribution, cookbook compatibility (with confidence indicators), node readiness, dependency graph, remediation guidance, and logs through an interactive web UI. Supports data exports and notification configuration. |
| **Notifications** | Sends webhook and email notifications for cookbook status changes, readiness milestones, collection failures, and stale node threshold alerts. |
| **Exports** | Generates ready/blocked node lists and remediation reports in CSV, JSON, and Chef search query formats for use in external upgrade automation. |
| **Logging** | Structured logging subsystem that captures all job activity, notification deliveries, export operations, and external process output, persisted to the datastore and viewable in the web UI. |
| **Authentication** | Supports local accounts, LDAP, and SAML authentication with role-based access control. |

## Prerequisites

### All Deployment Methods

- **PostgreSQL** 14 or later
- **Git** (for cloning cookbook repositories)
- Network access to the Chef Infra Server(s) and git repositories
- Network access to webhook URLs and/or SMTP server (if notifications are enabled)

### Optional (for Cookbook Compatibility Testing)

- **Docker** (required by the embedded Test Kitchen `kitchen-dokken` driver for container-based cookbook testing)

CookStyle and Test Kitchen are **embedded** in all packaging formats (RPM, DEB, container image) along with a self-contained Ruby runtime under `/opt/chef-migration-metrics/embedded/`. There is no need to install Chef Workstation or manage Ruby gems separately. Docker is the only external runtime dependency for cookbook compatibility testing. If Docker is not available, Test Kitchen testing is disabled and git-sourced cookbooks are reported as `untested`; CookStyle linting of Chef server-sourced cookbooks still functions without Docker.

### Building from Source

- **Go** 1.22 or later
- **Node.js** 20 or later and **npm** (for building the React frontend)
- **nFPM** (for building RPM and DEB packages)
- **Docker** (for building container images and the embedded Ruby environment)

> **Note:** When building from source and running directly (without packaging), CookStyle and Test Kitchen are **not** embedded automatically. You can either run `make build-embedded` to build the embedded Ruby environment locally, or install `cookstyle` and `kitchen` via Chef Workstation or `gem install`. The application falls back to `PATH` lookup when the embedded directory is not present.

## Installation

Chef Migration Metrics can be installed via RPM, DEB, container image, Docker Compose, or Helm chart. Choose the method that best fits your environment.

### Option 1: RPM Package (RHEL, CentOS, Fedora, Amazon Linux)

```
sudo rpm -i chef-migration-metrics-<version>.x86_64.rpm
```

Or with `dnf`:

```
sudo dnf install chef-migration-metrics-<version>.x86_64.rpm
```

The RPM installs:

| Path | Purpose |
|------|---------|
| `/usr/bin/chef-migration-metrics` | Application binary |
| `/etc/chef-migration-metrics/config.yml` | Configuration file |
| `/etc/chef-migration-metrics/keys/` | Chef API private key directory |
| `/etc/sysconfig/chef-migration-metrics` | Environment variable overrides (secrets) |
| `/var/lib/chef-migration-metrics/` | Working directory for git clones |
| `/usr/lib/systemd/system/chef-migration-metrics.service` | systemd unit |
| `/opt/chef-migration-metrics/embedded/` | Self-contained Ruby environment with CookStyle and Test Kitchen |

After installing, edit the configuration and start the service:

```
sudo vim /etc/chef-migration-metrics/config.yml
sudo vim /etc/sysconfig/chef-migration-metrics   # set DATABASE_URL, LDAP_BIND_PASSWORD, etc.
sudo systemctl start chef-migration-metrics
sudo systemctl status chef-migration-metrics
```

### Option 2: DEB Package (Debian, Ubuntu)

```
sudo dpkg -i chef-migration-metrics_<version>_amd64.deb
```

Or with `apt`:

```
sudo apt install ./chef-migration-metrics_<version>_amd64.deb
```

The DEB installs the same filesystem layout as the RPM, with the environment file at `/etc/default/chef-migration-metrics` (Debian convention).

```
sudo vim /etc/chef-migration-metrics/config.yml
sudo vim /etc/default/chef-migration-metrics   # set DATABASE_URL, LDAP_BIND_PASSWORD, etc.
sudo systemctl start chef-migration-metrics
sudo systemctl status chef-migration-metrics
```

### Option 3: Docker Compose (Local / Evaluation)

Docker Compose provides a single-command setup including the application and PostgreSQL, ideal for evaluation and local development.

```
cd deploy/docker-compose
cp .env.example .env
```

Edit `.env` to set at minimum:

```
POSTGRES_PASSWORD=your-secure-password
```

Edit `config.yml` with your Chef server organisations, target versions, and git URLs. Place Chef API private keys in the `keys/` directory.

Start the stack:

```
docker compose up -d
```

Access the dashboard at `http://localhost:8080`.

View logs:

```
docker compose logs -f app
```

Stop and remove everything (including data):

```
docker compose down -v
```

See [`deploy/docker-compose/README.md`](deploy/docker-compose/README.md) for full details.

### Option 4: Container Image (Standalone)

Pull the image:

```
docker pull ghcr.io/trickyearlobe-chef/chef-migration-metrics:<version>
```

Run with a mounted configuration file, keys, and a connection to an external PostgreSQL instance:

```
docker run -d \
  --name chef-migration-metrics \
  -p 8080:8080 \
  -v /path/to/config.yml:/etc/chef-migration-metrics/config.yml:ro \
  -v /path/to/keys/:/etc/chef-migration-metrics/keys/:ro \
  -v chef-data:/var/lib/chef-migration-metrics \
  -e DATABASE_URL="postgres://user:pass@db-host:5432/chef_migration_metrics" \
  ghcr.io/trickyearlobe-chef/chef-migration-metrics:<version>
```

Two image variants are available:

| Image | Description |
|-------|-------------|
| `chef-migration-metrics:<version>` | Base image with Git. Suitable when Test Kitchen and CookStyle run externally. |
| `chef-migration-metrics:<version>-analysis` | Extended image including Test Kitchen, CookStyle, and Ruby. For self-contained deployments. |

### Option 5: Kubernetes with Helm

The Helm chart deploys Chef Migration Metrics with an optional bundled PostgreSQL instance, Ingress, TLS, persistent storage, and horizontal pod autoscaling.

```
# Add the Bitnami repo (for the PostgreSQL subchart dependency)
helm repo add bitnami https://charts.bitnami.com/bitnami

# Build chart dependencies
cd deploy/helm/chef-migration-metrics
helm dependency build

# Install with default values (bundled PostgreSQL, local auth)
helm install chef-migration-metrics . \
  --namespace chef-migration-metrics \
  --create-namespace \
  --set postgresql.auth.password=changeme

# Install with a custom values file
helm install chef-migration-metrics . \
  --namespace chef-migration-metrics \
  --create-namespace \
  -f my-values.yaml
```

Key Helm values:

| Value | Description |
|-------|-------------|
| `replicaCount` | Number of application pods (background jobs are serialised via a database lock) |
| `image.repository` | Container image repository |
| `image.tag` | Image tag (defaults to chart `appVersion`) |
| `config.*` | Application configuration (rendered into a ConfigMap) |
| `secrets.databaseUrl` | PostgreSQL connection string (stored in a Secret) |
| `chefKeys.keys` | Inline Chef API private keys (or use `chefKeys.existingSecret`) |
| `ingress.enabled` | Enable Kubernetes Ingress |
| `postgresql.enabled` | Deploy bundled PostgreSQL (disable to use an external database) |
| `persistence.enabled` | Enable PVC for git clone working directory |
| `autoscaling.enabled` | Enable HPA for read-path scaling |

Upgrade an existing release:

```
helm upgrade chef-migration-metrics . \
  --namespace chef-migration-metrics \
  -f my-values.yaml
```

Uninstall:

```
helm uninstall chef-migration-metrics --namespace chef-migration-metrics
```

Run Helm tests to verify connectivity:

```
helm test chef-migration-metrics --namespace chef-migration-metrics
```

See [`deploy/helm/chef-migration-metrics/README.md`](deploy/helm/chef-migration-metrics/README.md) for the full values reference.

### Option 6: Build from Source

```
git clone https://github.com/trickyearlobe-chef/chef-migration-metrics.git
cd chef-migration-metrics

# Build everything (binary with embedded frontend)
make build

# Run directly
./build/chef-migration-metrics --config config.yml

# Or build packages
make package-rpm      # produces build/chef-migration-metrics-<version>.x86_64.rpm
make package-deb      # produces build/chef-migration-metrics_<version>_amd64.deb
make package-docker   # builds the container image locally
make package-all      # all of the above
```

## Configuration

Configuration is stored in a YAML file. Sensitive values (passwords, key paths) can be overridden via environment variables.

At a minimum, configure:

- One or more Chef Infra Server organisations with API client credentials
- Target Chef Client versions to test against
- PostgreSQL datastore connection URL
- Git base URLs for cookbook repositories (if applicable)

See the [Configuration specification](.claude/specifications/configuration/Specification.md) for:

- Full YAML schema with all available settings
- Environment variable override conventions
- Notification channel configuration (webhooks, email, SMTP)
- Export settings (async thresholds, retention)
- Stale node and cookbook threshold settings
- Validation rules
- A complete annotated example

### Chef Server API Credentials

For each Chef Infra Server organisation, create a dedicated API client:

```
knife client create chef-migration-metrics --orgname myorg -f /path/to/keys/myorg.pem
```

Grant the client read access to nodes, cookbooks, roles, and environments. See the [Chef API specification](.claude/specifications/chef-api/Specification.md) for details.

### Database Setup

If not using Docker Compose or the Helm PostgreSQL subchart, create a PostgreSQL database manually:

```
createdb chef_migration_metrics
```

The application runs database migrations automatically on startup — no manual schema setup is required.

## Authentication

The web UI supports three authentication providers, which can be used simultaneously:

- **Local accounts** — built-in user management with bcrypt password hashing
- **LDAP** — authenticate against an existing LDAP/Active Directory server
- **SAML 2.0** — federated authentication with an external identity provider

Role-based access control provides **Admin** and **Viewer** roles. See the [Authentication specification](.claude/specifications/auth/Specification.md) for details.

## Security — Never Commit Secrets

This project includes multiple layers of protection to prevent credentials from being committed to version control.

### Pre-commit Hook

A git pre-commit hook scans staged files for private keys, API tokens, hardcoded passwords, and other secret patterns. Install it after cloning:

```
make install-hooks
```

The hook runs automatically on every `git commit` and blocks the commit if potential secrets are detected. To bypass it in exceptional cases (e.g. committing test fixtures with obviously fake keys), use `git commit --no-verify`.

### CI Secret Scanning

The CI pipeline runs [gitleaks](https://github.com/gitleaks/gitleaks) on every push and pull request. The configuration is in `.gitleaks.toml`. The build will fail if secrets are detected in any commit.

To run the same scan locally:

```
make scan-secrets
```

### .gitignore Protection

The `.gitignore` file excludes common secret file types (`*.pem`, `*.key`, `.env`, `keys/`, `acme/`). The `.dockerignore` and `.helmignore` files mirror these patterns to prevent secrets from leaking into container images or Helm chart archives.

### Credential Management

For details on how the application manages credentials at runtime (encrypted storage, environment variable injection, file-based keys), see the [Secrets Storage Specification](.claude/specifications/secrets-storage/Specification.md).

## Specifications

Detailed specifications for every component are maintained under `.claude/specifications/`:

| Document | Description |
|----------|-------------|
| [Project Specification](.claude/specifications/Specification.md) | Top-level overview, scope, and non-functional requirements |
| [Data Collection](.claude/specifications/data-collection/Specification.md) | Node collection, Policyfile support, cookbook fetching, stale detection, role dependency graph, fault tolerance |
| [Analysis](.claude/specifications/analysis/Specification.md) | Cookbook usage, compatibility testing, remediation guidance, complexity scoring, node readiness |
| [Visualisation](.claude/specifications/visualisation/Specification.md) | Dashboard views, dependency graph, remediation guidance, confidence indicators, exports, notifications, filters, drill-downs, log viewer |
| [Configuration](.claude/specifications/configuration/Specification.md) | Full YAML schema, environment variable overrides, notification channels, export settings, stale thresholds |
| [Authentication](.claude/specifications/auth/Specification.md) | Local, LDAP, SAML providers and RBAC |
| [Logging](.claude/specifications/logging/Specification.md) | Structured logging, scopes (including notifications and exports), retention |
| [Chef API](.claude/specifications/chef-api/Specification.md) | Chef Infra Server API endpoints and signing protocol |
| [Datastore](.claude/specifications/datastore/Specification.md) | Database schema, tables, indexes, relationships (including remediation, complexity, dependency graph, notifications, and exports) |
| [Web API](.claude/specifications/web-api/Specification.md) | HTTP API endpoints between backend and frontend (including remediation, dependency graph, exports, and notifications) |
| [Packaging](.claude/specifications/packaging/Specification.md) | RPM, DEB, container image, Docker Compose, and Helm chart |
| [Ownership](.claude/specifications/ownership/Specification.md) | Ownership tracking for nodes, roles, policyfiles, cookbooks, and git repositories — owner model, auto-derivation rules, bulk import, owner-scoped views and exports |

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).