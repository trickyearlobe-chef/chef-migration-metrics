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
- **Visualises metrics** in a web dashboard with interactive filters, drill-downs, confidence indicators, and trend charts
- **Captures logs** from all background jobs and external processes, viewable from the web UI

### Why Disk Space Matters

From Chef Client version 19 onwards, the packaging format changed from RPMs, DEBs, and MSIs to Habitat bundles. Habitat bundles are significantly larger than previous packaging formats, and InSpec (previously a separate package) is now bundled with Chef Client. Disk space availability on each node is therefore a key factor in determining upgrade readiness.

### Why Remediation Guidance Matters

Knowing which cookbooks are incompatible is only half the battle — practitioners also need to know **how to fix them**. Chef Migration Metrics generates auto-correct previews (showing exactly what `cookstyle --auto-correct` would change), links each deprecation to its migration documentation with before/after code examples, and assigns complexity scores so teams can identify quick wins and plan for harder remediation work. A cookbook with 2 deprecation warnings is very different from one with 47 — the complexity score makes that distinction actionable.

## Architecture

Chef Migration Metrics is a Go application with an embedded React frontend. CookStyle and Test Kitchen are **not** bundled in the application — they are external tools provided by [Chef Workstation](https://docs.chef.io/workstation/) and must be available at runtime for cookbook compatibility analysis. If they are not available, data collection and the dashboard still work but cookbook analysis is skipped.

```
┌───────────────────────────────────────────────────────────────────┐
│                         Web Dashboard                             │
│  Version distribution · Cookbook compatibility · Node readiness   │
│  Dependency graph · Remediation guidance · Complexity scores      │
│  Confidence indicators · Exports · Log viewer                     │
│  Interactive filters (org, env, role, policy, platform, stale)    │
└───────────────────────────────┬───────────────────────────────────┘
                                │
                                │ HTTP API
                                │
┌───────────────────────────────┴───────────────────────────────────┐
│                         Go Backend                                │
│                                                                   │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────────┐  │
│  │ Data            │ │ Analysis        │ │ Web API / Auth      │  │
│  │ Collection      │ │                 │ │                     │  │
│  │                 │ │ Cookbook usage  │ │ REST endpoints      │  │
│  │ Node data       │ │ CookStyle *     │ │ Local accounts      │  │
│  │ Policyfiles     │ │ Kitchen *       │ │ RBAC                │  │
│  │ Cookbooks       │ │ Remediation     │ │ Session management  │  │
│  │ Git repos       │ │ Complexity      │ │ Exports             │  │
│  │ Role graph      │ │ Readiness       │ │                     │  │
│  │ Stale nodes     │ │                 │ │                     │  │
│  └────────┬────────┘ └────────┬────────┘ └──────────┬──────────┘  │
│           │                   │                     │             │
│  ┌────────┴───────────────────┴─────────────────────┴──────────┐  │
│  │                   PostgreSQL Datastore                      │  │
│  │  Nodes · Server Cookbooks · Git Repos · Test results        │  │
│  │  Remediation · Complexity · Dependency graph · Logs         │  │
│  │  Metrics · Exports                                          │  │
│  └──────┬─────────────────────────────────────┬────────────────┘  │
│         │                                     │                   │
└─────────┼─────────────────────────────────────┼───────────────────┘
          │                                     │
          │ Chef Infra Server API               │ Git (clone/pull)
          ▼                                     ▼
  ┌──────────────────┐              ┌───────────────────┐
  │ Chef Infra       │              │ Git repos         │
  │ Server(s)        │              │ (GitHub, GitLab,  │
  │                  │              │  Bitbucket, etc.) │
  │ Org 1, Org 2 …   │              │                   │
  └──────────────────┘              └───────────────────┘

  * CookStyle and Test Kitchen require Chef Workstation
    to be installed separately (see Prerequisites).
```

### Components

| Component | Description |
|-----------|-------------|
| **Data Collection** | Periodically collects node attributes from Chef Infra Server organisations using partial search. Supports both classic and Policyfile nodes. Fetches server cookbooks from the Chef server and manages git repository clones for cookbooks with known git repos. Collects role dependency graphs. Detects stale nodes and cookbooks. |
| **Analysis** | Computes cookbook usage statistics, runs Test Kitchen and CookStyle compatibility tests (when Chef Workstation is available), generates remediation guidance (auto-correct previews, migration docs), computes complexity scores and blast radius, and evaluates per-node upgrade readiness. |
| **Web Dashboard** | Presents version distribution, cookbook compatibility (with confidence indicators), node readiness, dependency graph, remediation guidance, and logs through an interactive web UI. Supports data exports. |
| **Exports** | Generates ready/blocked node lists and remediation reports in CSV, JSON, and Chef search query formats for use in external upgrade automation. |
| **Logging** | Structured logging subsystem that captures all job activity, export operations, and external process output, persisted to the datastore and viewable in the web UI. |
| **Authentication** | Local accounts with bcrypt password hashing and role-based access control (Admin / Viewer). |

## Prerequisites

### All Deployment Methods

- **PostgreSQL** 14 or later
- **Git** (for cloning cookbook repositories)
- Network access to the Chef Infra Server(s) and git repositories

### Optional (for Cookbook Compatibility Testing)

- **Chef Workstation** — provides CookStyle, Test Kitchen, and InSpec
- **Docker** (required by the Test Kitchen `kitchen-dokken` driver for container-based cookbook testing)

The application looks for `cookstyle` and `kitchen` binaries in a configurable directory first (see `analysis_tools.embedded_bin_dir` in the configuration), then falls back to searching `PATH`. If neither is found, cookbook analysis is skipped gracefully — data collection and the dashboard still work.

### Building from Source

- **Go** 1.25 or later
- **Node.js** 20 or later and **npm** (for building the React frontend)
- **nFPM** (for building RPM and DEB packages)
- **Docker** (for building the embedded Ruby environment)

## Installation

Chef Migration Metrics can be installed via RPM, DEB, distribution archive, or Docker Compose. Choose the method that best fits your environment.

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

The package lists `chef-workstation` as a soft dependency (`Recommends`). Install Chef Workstation separately to enable cookbook compatibility testing.

After installing, edit the configuration and start the service:

```
sudo vim /etc/chef-migration-metrics/config.yml
sudo vim /etc/sysconfig/chef-migration-metrics   # set DATABASE_URL, etc.
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
sudo vim /etc/default/chef-migration-metrics   # set DATABASE_URL, etc.
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

### Option 4: Build from Source

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
make package-all      # RPM + DEB + distribution archives
```

When running from source, CookStyle and Test Kitchen are resolved from `PATH`. Install [Chef Workstation](https://docs.chef.io/workstation/install/) or `gem install cookstyle test-kitchen` to make them available.

## Configuration

Configuration is stored in a YAML file. Sensitive values (passwords, key paths) can be overridden via environment variables.

At a minimum, configure:

- One or more Chef Infra Server organisations with API client credentials
- Target Chef Client versions to test against
- PostgreSQL datastore connection URL
- Git base URLs for cookbook repositories (if applicable)

See the [Configuration specification](specifications/configuration.md) for:

- Full YAML schema with all available settings
- Environment variable override conventions
- Export settings (async thresholds, retention)
- Stale node and cookbook threshold settings
- Validation rules
- A complete annotated example

### Chef Server API Credentials

For each Chef Infra Server organisation, create a dedicated API client:

```
knife client create chef-migration-metrics --orgname myorg -f /path/to/keys/myorg.pem
```

Grant the client read access to nodes, cookbooks, roles, and environments. See the [Chef API specification](specifications/chef-api.md) for details.

### Database Setup

If not using Docker Compose, create a PostgreSQL database manually:

```
createdb chef_migration_metrics
```

The application runs database migrations automatically on startup — no manual schema setup is required.

### Test Kitchen Driver Configuration

The Test Kitchen driver is configured under `analysis_tools.test_kitchen` in the YAML config file. The default driver is `dokken` (Docker-based, zero-config). Non-dokken drivers require additional settings.

**Minimal config (dokken):**

```yaml
analysis_tools:
  test_kitchen:
    enabled: true
    driver: dokken
    timeout_minutes: 30
```

**Non-dokken config (e.g. vCenter, EC2, vRA):**

```yaml
analysis_tools:
  test_kitchen:
    enabled: true
    driver: vcenter
    timeout_minutes: 60
    driver_settings:
      vcenter_host: vcenter.example.com
      vcenter_username: user@vsphere.local
    driver_secrets:
      vcenter_password: vcenter-password
    platform_map:
      - kitchen_name: ubuntu-22.04
        image: tmpl-ubuntu-2204-base
        transport:
          username: kitchen
          password_credential: kitchen-vm-password
      - kitchen_name: centos-7
        image: tmpl-centos-7-base
```

| Setting | Default | Description |
|---------|---------|-------------|
| `driver` | `dokken` | Built-in profiles: `dokken`, `vcenter`, `vra`, `ec2`, `azurerm`, `google`, `vagrant`, `openstack`, `proxmox`, or `custom` |
| `timeout_minutes` | `30` | Maximum time per Test Kitchen run |
| `driver_settings` | empty | Plaintext driver connection settings |
| `driver_secrets` | empty | Credential names resolved at runtime from the credential store |
| `image_field_name` | auto | Set automatically by built-in profiles; required for `custom` |
| `platform_map` | empty | Maps kitchen platform names to VM/AMI images |

Credentials referenced in `driver_secrets` and `transport.password_credential` / `transport.ssh_key_credential` are managed via the **Admin → Credentials** page in the web UI and resolved at test runtime. Plaintext is zeroed from memory after use.

See the [Test Kitchen Driver specification](specifications/test-kitchen-drivers.md) for full details.

### vCenter Platform Map Setup

When using the `vcenter` driver, each platform in the `platform_map` maps a Test Kitchen platform name to a vSphere VM template. The application generates a `.kitchen.local.yml` overlay that references these templates with ERB credential injection.

**Step 1:** Store credentials via **Admin → Credentials** in the web UI:

- Create a credential named `vcenter-password` (type: `generic`) with the vCenter connection password.
- Create a credential named `kitchen-vm-password` (type: `generic`) with the VM transport password (for SSH/WinRM into test VMs).

**Step 2:** Configure the driver in the YAML config:

```yaml
analysis_tools:
  test_kitchen:
    driver: vcenter
    driver_settings:
      vcenter_host: vcenter.example.com
      vcenter_username: user@vsphere.local
      vcenter_disable_ssl_verify: false
      clone_type: full
      datacenter: "Datacenter"
    driver_secrets:
      vcenter_password: vcenter-password
    platform_map:
      - kitchen_name: ubuntu-22.04
        image: tmpl-ubuntu-2204-base
        driver_settings:
          cluster: "Cluster-01"
          resource_pool: "Kitchen"
          folder: "kitchen-vms"
        transport:
          username: kitchen
          password_credential: kitchen-vm-password
      - kitchen_name: windows-2022
        image: tmpl-win2022-base
        driver_settings:
          vm_customization:
            numCPUs: 4
            memoryMB: 4096
        transport:
          username: Administrator
          password_credential: kitchen-win-password
```

**Step 3:** Restart the application. At startup, the driver validates that all referenced credentials exist and decrypt successfully.

Per-platform `driver_settings` are merged with the top-level defaults. Platform entries not found in the map are skipped with a warning.

### Driver Migration Procedure

Switching drivers is a config-only operation — no code changes required. The platform map structure and transport credentials are driver-independent.

**Example: vCenter → vRA**

1. **Store new credentials** via **Admin → Credentials** in the web UI (e.g. create `vra-password` of type `generic`).

2. **Update the config:**
   ```yaml
   analysis_tools:
     test_kitchen:
       driver: vra                        # changed
       driver_settings:                    # changed
         base_url: https://vra.example.com
         username: user@example.com
         tenant: "my-tenant"
       driver_secrets:
         password: vra-password            # changed
       platform_map:
         - kitchen_name: ubuntu-22.04
           image: ubuntu-22.04-catalog     # changed (vRA catalog item)
           transport:                       # unchanged
             username: kitchen
             password_credential: kitchen-vm-password
         - kitchen_name: centos-7
           image: centos-7-catalog         # changed
   ```

3. **Restart the application.** The startup validator checks the new credentials and platform map.

**What changes:** `driver`, `driver_settings`, `driver_secrets`, and `image` values in the platform map.

**What stays the same:** Platform map structure, `transport` blocks, application code, and test results schema.

### Proxmox Platform Map Setup

When using the `proxmox` driver, each platform in the `platform_map` maps a Test Kitchen platform name to a Proxmox VM template. The application generates a `.kitchen.local.yml` overlay that references these templates with ERB credential injection.

**Step 1:** Store credentials via **Admin → Credentials** in the web UI:

- Create a credential named `proxmox-password` (type: `generic`) with the Proxmox connection password.
- Create a credential named `kitchen-vm-password` (type: `generic`) with the VM transport password (for SSH into test VMs).

**Step 2:** Configure the driver in the YAML config:

```yaml
analysis_tools:
  test_kitchen:
    driver: proxmox
    driver_settings:
      proxmox_url: "https://pve.example.com:8006"
      proxmox_username: "user@pam"
      proxmox_node: "pve1"
    driver_secrets:
      proxmox_password: proxmox-password
    platform_map:
      - kitchen_name: ubuntu-22.04
        image: tmpl-ubuntu-2204
        transport:
          username: kitchen
          password_credential: kitchen-vm-password
      - kitchen_name: centos-7
        image: tmpl-centos-7
```

**Step 3:** Restart the application. At startup, the driver validates that all referenced credentials exist and decrypt successfully.

Per-platform `driver_settings` are merged with the top-level defaults. Platform entries not found in the map are skipped with a warning.

## Authentication

The web UI currently supports **local accounts** with bcrypt password hashing, session-based authentication, and role-based access control with **Admin** and **Viewer** roles.

See the [Authentication specification](specifications/auth.md) for details.

> **Planned:** SAML 2.0 authentication is defined in the configuration schema but not yet implemented.

## Security — Never Commit Secrets

This project includes multiple layers of protection to prevent credentials from being committed to version control.

### Pre-commit Hook

A git pre-commit hook scans staged files for private keys, API tokens, hardcoded passwords, and other secret patterns. Install it after cloning:

```
make install-hooks
```

The hook runs automatically on every `git commit` and blocks the commit if potential secrets are detected. To bypass it in exceptional cases (e.g. committing test fixtures with obviously fake keys), use `git commit --no-verify`.

### Secret Scanning

The pre-commit hook (installed via `make install-hooks`) scans staged files for secret patterns including private keys, AWS credentials, GitHub tokens, database connection strings, and more. This catches secrets **before** they enter the repository.

GitHub's built-in secret scanning provides an additional layer of protection at the repository level.

### .gitignore Protection

The `.gitignore` file excludes common secret file types (`*.pem`, `*.key`, `.env`, `keys/`, `acme/`). The `.dockerignore` mirrors these patterns to prevent secrets from leaking into Docker build contexts.

### Credential Management

For details on how the application manages credentials at runtime (encrypted storage, environment variable injection, file-based keys), see the [Secrets Storage Specification](specifications/secrets-storage.md).

## Roadmap

The following features are defined in the specifications but not yet implemented:

| Feature | Status |
|---------|--------|
| Webhook notifications (Slack, Teams, PagerDuty) | Configuration and validation in place; runtime dispatcher not yet built |
| Email notifications (SMTP) | Configuration and validation in place; SMTP sender not yet built |
| SAML 2.0 authentication | Configuration and validation in place; SP logic not yet built (endpoints return 501) |

## Specifications

Detailed specifications for every component are maintained under `specifications/`:

| Document | Description |
|----------|-------------|
| [Data Collection](specifications/data-collection.md) | Node collection, Policyfile support, cookbook fetching, stale detection, role dependency graph, fault tolerance |
| [Analysis](specifications/analysis.md) | Cookbook usage, compatibility testing, remediation guidance, complexity scoring, node readiness |
| [Visualisation](specifications/visualisation.md) | Dashboard views, dependency graph, remediation guidance, confidence indicators, exports, notifications, filters, drill-downs, log viewer |
| [Configuration](specifications/configuration.md) | Full YAML schema, environment variable overrides, notification channels, export settings, stale thresholds |
| [Authentication](specifications/auth.md) | Local and SAML providers and RBAC |
| [Logging](specifications/logging.md) | Structured logging, scopes (including notifications and exports), retention |
| [Chef API](specifications/chef-api.md) | Chef Infra Server API endpoints and signing protocol |
| [Web API](specifications/web-api.md) | HTTP API endpoints between backend and frontend (including remediation, dependency graph, exports, and notifications) |
| [Packaging](specifications/packaging.md) | RPM, DEB, distribution archives, and Docker Compose |
| [Ownership](specifications/ownership.md) | Ownership tracking for nodes, roles, policyfiles, cookbooks, and git repositories — owner model, auto-derivation rules, bulk import, owner-scoped views and exports |

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).
