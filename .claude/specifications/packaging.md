# Packaging and Deployment - Component Specification

> Component specification for the packaging and deployment of Chef Migration Metrics.
> See the [top-level specification](../Specification.md) for project overview and scope.

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

### 1.1 Go Binary

The primary build artifact is a statically linked Go binary with the React frontend embedded using Go's `embed` package. Database migration SQL files are also embedded.

| Property | Value |
|----------|-------|
| Binary name | `chef-migration-metrics` |
| Supported `GOOS` | `linux` |
| Supported `GOARCH` | `amd64`, `arm64` |
| Static linking | Yes — `CGO_ENABLED=0` to produce a fully static binary |
| Embedded assets | React SPA build output, SQL migration files |

A `Makefile` (or equivalent task runner) must provide targets for:

| Target | Description |
|--------|-------------|
| `build` | Compile the Go binary for the host platform |
| `build-all` | Cross-compile for all supported OS/arch combinations |
| `build-frontend` | Build the React SPA and place output in the embed directory |
| `build-embedded` | Build the embedded Ruby environment (CookStyle, Test Kitchen) for the host platform |
| `build-embedded-amd64` | Build the embedded Ruby environment for `linux/amd64` |
| `build-embedded-arm64` | Build the embedded Ruby environment for `linux/arm64` |
| `test` | Run all Go unit tests |
| `lint` | Run `golangci-lint` and `cookstyle --format json` |
| `package-rpm` | Build the RPM package (includes embedded Ruby environment) |
| `package-deb` | Build the DEB package (includes embedded Ruby environment) |
| `package-all` | Build RPM and DEB packages |

### 1.2 Version Injection

The application version must be injected at build time via `-ldflags`:

```
go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)"
```

The version string is used in:

- The `User-Agent` header for Chef API requests (see [Chef API specification](../chef-api/Specification.md))
- The `/api/v1/admin/status` endpoint response
- Package metadata (RPM, DEB)
- The `--version` CLI flag

---

## 2. RPM Package

### 2.1 Tooling

RPM packages are built using [nFPM](https://nfpm.goreleaser.com/), a Go-based packager that does not require `rpmbuild` or a full RPM toolchain. The nFPM configuration is maintained in a `nfpm.yaml` file at the repository root.

### 2.2 Package Metadata

| Field | Value |
|-------|-------|
| Name | `chef-migration-metrics` |
| Version | Injected from the build version string |
| Release | `1` (incremented for packaging-only changes) |
| Architecture | `x86_64` or `aarch64` (matches `GOARCH`) |
| License | `Apache-2.0` |
| Vendor | Project maintainers |
| Description | Tool for planning and tracking Chef Client upgrade projects |
| URL | Repository URL |

### 2.3 Dependencies

| Dependency | Type | Reason |
|------------|------|--------|
| `git` | Requires | Cookbook repository clone and pull operations |
| `shadow-utils` | Requires | Provides `useradd` / `groupadd` for the service account |

Test Kitchen, CookStyle, and their Ruby runtime are **embedded** in the package under `/opt/chef-migration-metrics/embedded/`. This self-contained Ruby environment eliminates external dependencies on Chef Workstation or system Ruby. See section 2.4 for the filesystem layout.

### 2.4 Filesystem Layout

```
/usr/bin/chef-migration-metrics                          # Application binary
/etc/chef-migration-metrics/config.yml                   # Default configuration file (noreplace)
/etc/chef-migration-metrics/keys/                        # Directory for Chef API private keys (0700)
/var/lib/chef-migration-metrics/                         # Working directory for git clones and cookbook downloads
/var/log/chef-migration-metrics/                         # Optional file-based log output (stdout preferred)
/usr/lib/systemd/system/chef-migration-metrics.service   # systemd unit file
/opt/chef-migration-metrics/embedded/                    # Self-contained Ruby environment
/opt/chef-migration-metrics/embedded/bin/ruby            # Embedded Ruby interpreter
/opt/chef-migration-metrics/embedded/bin/cookstyle       # Embedded CookStyle binary
/opt/chef-migration-metrics/embedded/bin/kitchen         # Embedded Test Kitchen binary
/opt/chef-migration-metrics/embedded/lib/                # Ruby standard library and installed gems
```

Configuration files are marked `%config(noreplace)` so that upgrades do not overwrite user-customised files.

The embedded Ruby tree is fully self-contained and does not interfere with any system Ruby installation. The application resolves `cookstyle` and `kitchen` from `/opt/chef-migration-metrics/embedded/bin/` by default (see [Configuration Specification](../configuration/Specification.md) for the `embedded_bin_dir` setting), falling back to `PATH` lookup if the embedded directory does not exist.

### 2.5 systemd Unit File

```ini
[Unit]
Description=Chef Migration Metrics
Documentation=https://github.com/trickyearlobe-chef/chef-migration-metrics
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=chef-migration-metrics
Group=chef-migration-metrics
ExecStart=/usr/bin/chef-migration-metrics --config /etc/chef-migration-metrics/config.yml
Restart=on-failure
RestartSec=10
EnvironmentFile=-/etc/sysconfig/chef-migration-metrics
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/chef-migration-metrics /var/log/chef-migration-metrics
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

The `EnvironmentFile` directive points to `/etc/sysconfig/chef-migration-metrics` (RPM convention) where operators can set environment variable overrides such as `DATABASE_URL` and `LDAP_BIND_PASSWORD` without modifying the config file.

### 2.6 Pre/Post Install Scripts

**Pre-install:**

```bash
#!/bin/bash
# Create the service account if it does not exist
getent group chef-migration-metrics >/dev/null || groupadd -r chef-migration-metrics
getent passwd chef-migration-metrics >/dev/null || \
    useradd -r -g chef-migration-metrics -d /var/lib/chef-migration-metrics \
    -s /sbin/nologin -c "Chef Migration Metrics" chef-migration-metrics
```

**Post-install:**

```bash
#!/bin/bash
# Set ownership on data and log directories
chown -R chef-migration-metrics:chef-migration-metrics /var/lib/chef-migration-metrics
chown -R chef-migration-metrics:chef-migration-metrics /var/log/chef-migration-metrics
chown -R chef-migration-metrics:chef-migration-metrics /etc/chef-migration-metrics/keys

# Reload systemd and enable the service (but do not start — let the operator configure first)
systemctl daemon-reload
systemctl enable chef-migration-metrics.service

echo "Chef Migration Metrics installed. Edit /etc/chef-migration-metrics/config.yml, then run:"
echo "  systemctl start chef-migration-metrics"
```

**Pre-uninstall:**

```bash
#!/bin/bash
# Stop and disable the service on removal (not on upgrade)
if [ "$1" = "0" ]; then
    systemctl stop chef-migration-metrics.service || true
    systemctl disable chef-migration-metrics.service || true
fi
```

---

## 3. DEB Package

### 3.1 Tooling

DEB packages are also built using nFPM. The same `nfpm.yaml` file supports both RPM and DEB output formats.

### 3.2 Package Metadata

| Field | Value |
|-------|-------|
| Name | `chef-migration-metrics` |
| Version | Injected from the build version string |
| Architecture | `amd64` or `arm64` |
| Section | `admin` |
| Priority | `optional` |
| License | `Apache-2.0` |
| Maintainer | Project maintainers |
| Description | Tool for planning and tracking Chef Client upgrade projects |
| Homepage | Repository URL |

### 3.3 Dependencies

| Dependency | Type | Reason |
|------------|------|--------|
| `git` | Depends | Cookbook repository clone and pull operations |
| `adduser` | Pre-Depends | Service account creation |

Test Kitchen, CookStyle, and their Ruby runtime are **embedded** in the package under `/opt/chef-migration-metrics/embedded/`, identical to the RPM layout (section 2.4).

### 3.4 Filesystem Layout

Identical to the RPM layout (section 2.4) with one exception:

- The environment file is at `/etc/default/chef-migration-metrics` (Debian convention) instead of `/etc/sysconfig/chef-migration-metrics`.
- The systemd unit file references this path in `EnvironmentFile`.

### 3.5 systemd Unit File

Identical to the RPM unit file (section 2.5) except the `EnvironmentFile` line:

```ini
EnvironmentFile=-/etc/default/chef-migration-metrics
```

### 3.6 Maintainer Scripts

The DEB package uses `preinst`, `postinst`, and `prerm` scripts that are functionally identical to the RPM scripts in section 2.6, adapted for Debian conventions:

- `preinst` creates the service account using `adduser --system --group --no-create-home`.
- `postinst` sets ownership and enables the service.
- `prerm` stops and disables the service on purge or remove.

---

## 4. Docker Compose

### 4.1 Purpose

The Docker Compose file starts a PostgreSQL database for local development. The application runs on the host via `make run` or `make dev`.

### 4.2 File Location

```
deploy/
└── docker-compose/
    ├── docker-compose.yml          # Compose file
    ├── config.yml                  # Example application configuration for local use
    ├── .env.example                # Example environment variables
    └── README.md                   # Quick-start instructions
```

### 4.3 Services

#### `db` — PostgreSQL

| Property | Value |
|----------|-------|
| Image | `postgres:16-bookworm` |
| Ports | `5432:5432` (exposed for local debugging; not required in production) |
| Volumes | Named volume `pgdata` for data persistence across restarts |
| Environment | `POSTGRES_DB=chef_migration_metrics`, `POSTGRES_USER`, `POSTGRES_PASSWORD` from `.env` |
| Health check | `pg_isready -U $POSTGRES_USER -d $POSTGRES_DB` |

### 4.4 docker-compose.yml

```yaml
services:
  db:
    image: postgres:16-bookworm
    restart: unless-stopped
    command: ["-c", "shared_preload_libraries=pg_stat_statements"]
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-chef_migration_metrics}
      POSTGRES_USER: ${POSTGRES_USER:-chef_migration_metrics}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?Set POSTGRES_PASSWORD in .env}
    volumes:
      - pgdata:/var/lib/postgresql/data
    ports:
      - "${POSTGRES_PORT:-5432}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-chef_migration_metrics} -d ${POSTGRES_DB:-chef_migration_metrics}"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  pgdata:
    driver: local
```

### 4.5 Environment File

The `.env.example` file documents all configurable environment variables:

```
# PostgreSQL
POSTGRES_DB=chef_migration_metrics
POSTGRES_USER=chef_migration_metrics
POSTGRES_PASSWORD=changeme
POSTGRES_PORT=5432
```

### 4.6 Usage

```bash
cd deploy/docker-compose
cp .env.example .env
# Edit .env — at minimum set POSTGRES_PASSWORD

docker compose up -d    # start PostgreSQL
make run                # start the app on the host

# View DB logs
docker compose logs -f db

# Stop
docker compose down

# Stop and remove data
docker compose down -v
```

---

## 5. CI/CD Integration

### 5.1 Build Pipeline

The CI pipeline (e.g. GitHub Actions) should include the following stages:

| Stage | Steps |
|-------|-------|
| **Lint** | `golangci-lint`, `npm run lint` (frontend) |
| **Test** | Go unit tests, frontend unit tests |
| **Build** | Compile binary, build frontend, embed assets |
| **Package** | Build RPM (`make package-rpm`), DEB (`make package-deb`) |
| **Publish** | Upload RPM/DEB to release artifacts |

### 5.2 Release Workflow

- Releases are triggered by pushing a git tag matching `v*` (e.g. `v1.2.0`).
- The version is extracted from the tag and injected into the binary and package metadata.
- RPM and DEB packages are attached to the GitHub Release as assets.

---

## 6. nFPM Configuration

The `nfpm.yaml` file at the repository root configures both RPM and DEB package builds:

```yaml
name: chef-migration-metrics
arch: ${ARCH}
platform: linux
version: ${VERSION}
release: 1
section: admin
priority: optional
maintainer: Project Maintainers
description: Tool for planning and tracking Chef Client upgrade projects
vendor: Chef Migration Metrics Project
homepage: https://github.com/trickyearlobe-chef/chef-migration-metrics
license: Apache-2.0

contents:
  - src: ./build/chef-migration-metrics
    dst: /usr/bin/chef-migration-metrics
    file_info:
      mode: 0755

  - src: ./deploy/pkg/config.yml
    dst: /etc/chef-migration-metrics/config.yml
    type: config|noreplace
    file_info:
      mode: 0640

  - dst: /etc/chef-migration-metrics/keys/
    type: dir
    file_info:
      mode: 0700

  - dst: /var/lib/chef-migration-metrics/
    type: dir
    file_info:
      mode: 0750

  - dst: /var/log/chef-migration-metrics/
    type: dir
    file_info:
      mode: 0750

  # Embedded Ruby environment with CookStyle and Test Kitchen
  - src: ./build/embedded/
    dst: /opt/chef-migration-metrics/embedded/
    file_info:
      mode: 0755

  - src: ./deploy/pkg/chef-migration-metrics.service
    dst: /usr/lib/systemd/system/chef-migration-metrics.service
    file_info:
      mode: 0644

  - src: ./deploy/pkg/env-file
    dst: /etc/default/chef-migration-metrics
    type: config|noreplace
    file_info:
      mode: 0640
    packager: deb

  - src: ./deploy/pkg/env-file
    dst: /etc/sysconfig/chef-migration-metrics
    type: config|noreplace
    file_info:
      mode: 0640
    packager: rpm

scripts:
  preinstall: ./deploy/pkg/scripts/preinstall.sh
  postinstall: ./deploy/pkg/scripts/postinstall.sh
  preremove: ./deploy/pkg/scripts/preremove.sh

depends:
  - git

rpm:
  group: Applications/System

deb:
  pre_depends:
    - adduser
```

### 6.1 Building the Embedded Ruby Environment

The embedded Ruby environment is built during the `make build-embedded` step (or as part of `make package-all`) into `./build/embedded/`. The build process:

1. Uses a Docker container (`ruby:3.1-bookworm`) to install gems into an isolated prefix, ensuring a consistent build regardless of the host system. Ruby 3.1 is used to match Chef Workstation 25.13.7.
2. Pins `ffi:1.16.3` first, then installs `cookstyle:7.32.8`, `test-kitchen:3.9.1`, `inspec-bin:5.24.7`, `kitchen-inspec:3.1.0`, all kitchen drivers (vagrant, ec2, azurerm, google, hyperv, vcenter, vra, openstack, digitalocean), and `kitchen-dokken` from the Stromweld fork — all version-pinned to match Chef Workstation 25.13.7.
3. Creates binstubs (`cookstyle`, `kitchen`, `inspec`) with shebangs pointing to `/opt/chef-migration-metrics/embedded/bin/ruby`.
4. Copies the Ruby interpreter and shared libraries into the prefix.
5. Exports the entire tree to `./build/embedded/` on the host for nFPM to package.

This produces a platform-specific artifact — the `ARCH` and `GOOS` of the Ruby build must match the target package architecture.

**Makefile targets:**

| Target | Description |
|--------|-------------|
| `build-embedded` | Build the embedded Ruby environment for the host platform |
| `build-embedded-amd64` | Build for `linux/amd64` |
| `build-embedded-arm64` | Build for `linux/arm64` |

---

## 7. Repository Layout for Packaging Files

```
deploy/
├── docker-compose/
│   ├── docker-compose.yml
│   ├── config.yml
│   ├── .env.example
│   └── README.md
└── pkg/
    ├── config.yml                          # Default config file shipped in RPM/DEB
    ├── env-file                            # Default environment file for systemd
    ├── chef-migration-metrics.service      # systemd unit file
    └── scripts/
        ├── preinstall.sh
        ├── postinstall.sh
        └── preremove.sh

build/
├── chef-migration-metrics                  # Compiled Go binary (build output)
└── embedded/                               # Embedded Ruby environment (build output)
    ├── bin/
    │   ├── ruby                            # Ruby interpreter
    │   ├── cookstyle                       # CookStyle binstub
    │   └── kitchen                         # Test Kitchen binstub
    ├── lib/
    │   ├── libruby*                        # Ruby shared libraries
    │   └── ruby/                           # Ruby stdlib and installed gems
    └── ...

Makefile                                    # Build, test, lint, and package targets
nfpm.yaml                                   # nFPM configuration for RPM and DEB builds
```

> **Note:** The application is not containerised. Docker Compose is used only for local development services (PostgreSQL, ELK stack). The `deploy/docker-compose/` directory contains Compose files for these supporting services.

---

## Related Specifications

- [Top-level Specification](../Specification.md)
- [Configuration Specification](../configuration/Specification.md)
- [Analysis Specification](../analysis/Specification.md) — startup validation for external tools
- [Data Collection Specification](../data-collection/Specification.md) — background job serialisation
- [Web API Specification](../web-api/Specification.md) — health endpoint used by status checks
- [Datastore Specification](../datastore/Specification.md) — advisory locks for collection serialisation