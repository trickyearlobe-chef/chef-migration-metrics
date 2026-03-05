# Packaging and Deployment - Component Specification

> Component specification for the packaging and deployment of Chef Migration Metrics.
> See the [top-level specification](../Specification.md) for project overview and scope.

---

## TL;DR

This spec covers how the application is packaged and deployed across all supported formats. Key points:

- **Packaging formats:** RPM, DEB, and container image — all built from the same Go binary + embedded frontend assets.
- **Embedded Ruby:** All packages ship a self-contained Ruby runtime under `/opt/chef-migration-metrics/embedded/` with CookStyle, Test Kitchen, and kitchen-dokken pre-installed. No external Ruby or Chef Workstation required.
- **Container image:** Multi-stage Dockerfile — Go build stage, Ruby build stage, runtime stage (Debian slim). Multi-arch (amd64/arm64).
- **Systemd integration:** RPM/DEB packages include a systemd unit file, pre/post install scripts, and environment file.
- **Docker Compose:** Local dev stack with app + PostgreSQL (`deploy/docker-compose/`).
- **ELK testing stack:** Elasticsearch + Logstash + Kibana for testing NDJSON export (`deploy/elk/`).
- **Helm chart:** Full Kubernetes deployment at `deploy/helm/chef-migration-metrics/` with PostgreSQL subchart, TLS Secret support, ACME storage PVC, ingress, HPA, and PVC for git working directory.
- **CI/CD:** GitHub Actions workflows for CI (`ci.yml`) and release (`release.yml`) — lint, test, build, package, publish to GHCR.

---

## Overview

Chef Migration Metrics must be distributable as native Linux packages (RPM and DEB) and as a container image. Containerised deployments must be supported both locally via Docker Compose and in Kubernetes via a Helm chart.

All packaging artifacts are built from the same Go binary and embedded frontend assets. The packaging layer adds platform-specific integration (systemd, file layout, default configuration) or container runtime scaffolding (image, orchestration) around the single compiled binary.

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
| `package-docker` | Build the container image (Ruby build stage is part of the multi-stage Dockerfile) |
| `package-all` | Build RPM, DEB, and container image |

### 1.2 Version Injection

The application version must be injected at build time via `-ldflags`:

```
go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)"
```

The version string is used in:

- The `User-Agent` header for Chef API requests (see [Chef API specification](../chef-api/Specification.md))
- The `/api/v1/admin/status` endpoint response
- Package metadata (RPM, DEB, container image labels)
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

## 4. Container Image

### 4.1 Base Image

The container image uses a multi-stage build:

1. **Build stage** — `golang:1.22-bookworm` (or later) to compile the binary and build the frontend.
2. **Ruby build stage** — `ruby:3.2-bookworm` to install CookStyle, Test Kitchen, and their gem dependencies into a self-contained directory.
3. **Runtime stage** — `debian:bookworm-slim` as the minimal runtime base.

`debian:bookworm-slim` is chosen over `scratch` or `alpine` because the application shells out to external tools (`git`, `kitchen`, `cookstyle`) that require a C library and a shell. The image includes `git` and the embedded Ruby environment with CookStyle and Test Kitchen pre-installed.

### 4.2 Dockerfile

```dockerfile
# --- Go build stage ---
FROM golang:1.22-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the React frontend
RUN cd frontend && npm ci && npm run build

# Build the Go binary with embedded assets
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${GIT_COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /chef-migration-metrics .

# --- Ruby build stage ---
FROM ruby:3.2-bookworm AS ruby-builder

# Install gems into an isolated prefix that can be copied wholesale
ENV GEM_HOME=/opt/chef-migration-metrics/embedded/lib/ruby/gems/3.2.0
ENV GEM_PATH=$GEM_HOME
RUN mkdir -p $GEM_HOME && \
    gem install --no-document \
        cookstyle \
        test-kitchen \
        kitchen-dokken

# Create wrapper binstubs that use the embedded Ruby
RUN mkdir -p /opt/chef-migration-metrics/embedded/bin && \
    cp $(which ruby) /opt/chef-migration-metrics/embedded/bin/ruby && \
    for cmd in cookstyle kitchen; do \
        printf '#!/opt/chef-migration-metrics/embedded/bin/ruby\n' > /opt/chef-migration-metrics/embedded/bin/$cmd && \
        cat $(gem environment gemdir)/bin/$cmd >> /opt/chef-migration-metrics/embedded/bin/$cmd && \
        chmod 0755 /opt/chef-migration-metrics/embedded/bin/$cmd; \
    done

# Copy the Ruby shared libraries needed at runtime
RUN mkdir -p /opt/chef-migration-metrics/embedded/lib && \
    cp -a /usr/local/lib/libruby* /opt/chef-migration-metrics/embedded/lib/ 2>/dev/null || true && \
    cp -a /usr/local/lib/ruby /opt/chef-migration-metrics/embedded/lib/ruby/ 2>/dev/null || true

# --- Runtime stage ---
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        openssh-client \
        libyaml-0-2 \
        libffi8 \
        libgmp10 \
        zlib1g \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r chef-migration-metrics && \
    useradd -r -g chef-migration-metrics -d /var/lib/chef-migration-metrics \
    -s /usr/sbin/nologin chef-migration-metrics

# Filesystem layout matching native packages
RUN mkdir -p /etc/chef-migration-metrics/keys \
             /var/lib/chef-migration-metrics \
             /var/log/chef-migration-metrics && \
    chown -R chef-migration-metrics:chef-migration-metrics \
             /etc/chef-migration-metrics \
             /var/lib/chef-migration-metrics \
             /var/log/chef-migration-metrics

COPY --from=builder /chef-migration-metrics /usr/bin/chef-migration-metrics
COPY --from=ruby-builder /opt/chef-migration-metrics/embedded /opt/chef-migration-metrics/embedded

USER chef-migration-metrics
WORKDIR /var/lib/chef-migration-metrics

EXPOSE 8080

ENTRYPOINT ["/usr/bin/chef-migration-metrics"]
CMD ["--config", "/etc/chef-migration-metrics/config.yml"]
```

### 4.3 Image Labels

The image must include OCI-standard labels for traceability:

```dockerfile
LABEL org.opencontainers.image.title="chef-migration-metrics"
LABEL org.opencontainers.image.description="Tool for planning and tracking Chef Client upgrade projects"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${GIT_COMMIT}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"
LABEL org.opencontainers.image.source="https://github.com/trickyearlobe-chef/chef-migration-metrics"
LABEL org.opencontainers.image.licenses="Apache-2.0"
```

### 4.4 Image Tags

| Tag | Purpose |
|-----|---------|
| `<version>` (e.g. `1.2.0`) | Immutable release tag |
| `<major>.<minor>` (e.g. `1.2`) | Floating minor tag, updated on each patch release |
| `<major>` (e.g. `1`) | Floating major tag |
| `latest` | Points to the most recent stable release |
| `<commit-sha>` | Every CI build, for traceability |

> **Note:** There is only one image — all tags refer to the same image that includes the embedded Ruby environment with CookStyle and Test Kitchen. The previous `<tag>-analysis` variant has been removed.

### 4.5 Analysis Tools (Embedded)

CookStyle, Test Kitchen, and their Ruby runtime are embedded directly in the base container image via the Ruby build stage (see section 4.2). There is no separate extension image — all containers ship with full analysis capability.

The `Dockerfile.analysis` file is **removed**. The previous two-image approach (base image + analysis extension) is replaced by a single image that always includes the embedded tools.

> **Rationale:** Embedding the analysis tools eliminates a common deployment pitfall where the base image was used accidentally, resulting in all cookbooks being reported as `untested`. A single image with everything included reduces support burden and simplifies documentation.

If a deployment does not need cookbook compatibility testing (e.g. a read-only dashboard replica), the tools are simply unused — they add approximately 80–120 MB to the image size but have no runtime overhead when not invoked.

### 4.6 Container Configuration

Inside a container, configuration is supplied via:

1. **Mounted config file** — Mount a `config.yml` to `/etc/chef-migration-metrics/config.yml`.
2. **Environment variables** — All configuration values support environment variable overrides (see [Configuration specification](../configuration/Specification.md)).
3. **Mounted secrets** — Chef API private keys are mounted into `/etc/chef-migration-metrics/keys/`. TLS certificate and key files (for `static` mode) are mounted into `/etc/chef-migration-metrics/tls/`.

The container must not require any writable volumes to start for basic operation. Git clone working directories (`/var/lib/chef-migration-metrics`) should be backed by a persistent volume if cookbook repositories are large or if the container is ephemeral. When using ACME mode for TLS, the `acme.storage_path` (`/var/lib/chef-migration-metrics/acme`) **must** be backed by a persistent volume to preserve ACME account keys and certificates across restarts and avoid hitting CA rate limits.

### 4.6.1 Embedded Ruby Environment

The Ruby build stage in the Dockerfile produces a self-contained tree under `/opt/chef-migration-metrics/embedded/` that includes:

| Path | Contents |
|------|----------|
| `bin/ruby` | Ruby interpreter (copied from the build stage) |
| `bin/cookstyle` | CookStyle binstub using the embedded Ruby |
| `bin/kitchen` | Test Kitchen binstub using the embedded Ruby |
| `lib/libruby*` | Ruby shared libraries |
| `lib/ruby/` | Ruby standard library |
| `lib/ruby/gems/3.2.0/` | Installed gems (cookstyle, test-kitchen, kitchen-dokken, and dependencies) |

The binstubs use a shebang of `#!/opt/chef-migration-metrics/embedded/bin/ruby` so they are fully independent of any system Ruby. The application resolves tool paths from this directory by default (see the `embedded_bin_dir` configuration setting in the [Configuration Specification](../configuration/Specification.md)).

### 4.7 Health Check

The Dockerfile includes a `HEALTHCHECK` instruction:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD ["/usr/bin/chef-migration-metrics", "healthcheck"]
```

The `healthcheck` subcommand performs a lightweight HTTP GET against the application's health endpoint (`/api/v1/admin/status`) from inside the container and exits 0 if the response status is 200 with `"status": "healthy"`.

---

## 5. Docker Compose

### 5.1 Purpose

The Docker Compose file provides a single-command local development and evaluation environment. It starts the application, a PostgreSQL database, and optionally a reverse proxy, all pre-wired together.

### 5.2 File Location

```
deploy/
└── docker-compose/
    ├── docker-compose.yml          # Compose file
    ├── config.yml                  # Example application configuration for local use
    ├── .env.example                # Example environment variables
    └── README.md                   # Quick-start instructions
```

### 5.3 Services

#### `app` — Chef Migration Metrics

| Property | Value |
|----------|-------|
| Image | `chef-migration-metrics:latest` (or build from local Dockerfile) |
| Ports | `8080:8080` |
| Config mount | `./config.yml:/etc/chef-migration-metrics/config.yml:ro` |
| Keys mount | `./keys/:/etc/chef-migration-metrics/keys/:ro` |
| Depends on | `db` (with health check condition) |
| Restart | `unless-stopped` |
| Environment | `DATABASE_URL` pointing to the `db` service |

#### `db` — PostgreSQL

| Property | Value |
|----------|-------|
| Image | `postgres:16-bookworm` |
| Ports | `5432:5432` (exposed for local debugging; not required in production) |
| Volumes | Named volume `pgdata` for data persistence across restarts |
| Environment | `POSTGRES_DB=chef_migration_metrics`, `POSTGRES_USER`, `POSTGRES_PASSWORD` from `.env` |
| Health check | `pg_isready -U $POSTGRES_USER -d $POSTGRES_DB` |

### 5.4 docker-compose.yml

```yaml
version: "3.9"

services:
  db:
    image: postgres:16-bookworm
    restart: unless-stopped
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

  app:
    image: ${APP_IMAGE:-chef-migration-metrics:latest}
    build:
      context: ../../
      dockerfile: Dockerfile
      args:
        VERSION: ${VERSION:-dev}
        GIT_COMMIT: ${GIT_COMMIT:-unknown}
        BUILD_DATE: ${BUILD_DATE:-unknown}
    restart: unless-stopped
    ports:
      - "${APP_PORT:-8080}:8080"
    environment:
      DATABASE_URL: "postgres://${POSTGRES_USER:-chef_migration_metrics}:${POSTGRES_PASSWORD}@db:5432/${POSTGRES_DB:-chef_migration_metrics}?sslmode=disable"
      LDAP_BIND_PASSWORD: ${LDAP_BIND_PASSWORD:-}
    volumes:
      - ./config.yml:/etc/chef-migration-metrics/config.yml:ro
      - ./keys/:/etc/chef-migration-metrics/keys/:ro
      - cookbook_data:/var/lib/chef-migration-metrics
    depends_on:
      db:
        condition: service_healthy

volumes:
  pgdata:
    driver: local
  cookbook_data:
    driver: local
```

### 5.5 Environment File

The `.env.example` file documents all configurable environment variables:

```
# PostgreSQL
POSTGRES_DB=chef_migration_metrics
POSTGRES_USER=chef_migration_metrics
POSTGRES_PASSWORD=changeme
POSTGRES_PORT=5432

# Application
APP_IMAGE=chef-migration-metrics:latest
APP_PORT=8080
VERSION=dev
GIT_COMMIT=unknown
BUILD_DATE=unknown

# Secrets (optional — override config file values)
LDAP_BIND_PASSWORD=
```

### 5.6 Usage

```bash
cd deploy/docker-compose
cp .env.example .env
# Edit .env and config.yml for your environment
# Place Chef API keys in ./keys/

docker compose up -d

# View logs
docker compose logs -f app

# Stop
docker compose down

# Stop and remove data
docker compose down -v
```

---

## 6. Helm Chart

### 6.1 Purpose

The Helm chart provides a production-grade Kubernetes deployment for Chef Migration Metrics. It supports flexible configuration, secret management, persistent storage, ingress, and horizontal scaling considerations.

### 6.2 Chart Location

```
deploy/
└── helm/
    └── chef-migration-metrics/
        ├── Chart.yaml
        ├── values.yaml
        ├── templates/
        │   ├── _helpers.tpl
        │   ├── deployment.yaml
        │   ├── service.yaml
        │   ├── ingress.yaml
        │   ├── configmap.yaml
        │   ├── secret.yaml
        │   ├── serviceaccount.yaml
        │   ├── hpa.yaml
        │   ├── pvc.yaml
        │   ├── NOTES.txt
        │   └── tests/
        │       └── test-connection.yaml
        └── README.md
```

### 6.3 Chart.yaml

```yaml
apiVersion: v2
name: chef-migration-metrics
description: A Helm chart for deploying Chef Migration Metrics on Kubernetes
type: application
version: 0.1.0            # Chart version — incremented independently of app version
appVersion: "1.0.0"       # Application version — matches the container image tag
home: https://github.com/trickyearlobe-chef/chef-migration-metrics
sources:
  - https://github.com/trickyearlobe-chef/chef-migration-metrics
maintainers:
  - name: trickyearlobe
keywords:
  - chef
  - migration
  - metrics
  - upgrade
```

### 6.4 values.yaml

```yaml
# -- Number of application replicas.
# NOTE: The background collection job includes a single-run constraint
# (see data-collection specification). When running multiple replicas,
# only one replica will execute collection/analysis jobs at a time.
# Additional replicas serve dashboard traffic and API requests.
replicaCount: 1

image:
  # -- Container image repository
  repository: ghcr.io/trickyearlobe-chef/chef-migration-metrics
  # -- Image pull policy
  pullPolicy: IfNotPresent
  # -- Image tag (defaults to chart appVersion)
  tag: ""

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

serviceAccount:
  # -- Create a ServiceAccount
  create: true
  # -- Annotations for the ServiceAccount
  annotations: {}
  # -- ServiceAccount name (auto-generated if not set)
  name: ""

podAnnotations: {}

podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: false    # git operations require writable fs
  capabilities:
    drop:
      - ALL

# -- Application configuration (rendered into a ConfigMap and mounted as config.yml)
config:
  organisations: []
  #   - name: myorg-production
  #     chef_server_url: https://chef.example.com
  #     org_name: myorg-production
  #     client_name: chef-migration-metrics
  #     client_key_path: /etc/chef-migration-metrics/keys/myorg-production.pem

  target_chef_versions: []
  #   - "18.5.0"
  #   - "19.0.0"

  git_base_urls: []
  #   - https://github.com/myorg

  collection:
    schedule: "0 * * * *"

  concurrency:
    organisation_collection: 5
    node_page_fetching: 10
    git_pull: 10
    cookstyle_scan: 8
    test_kitchen_run: 4
    readiness_evaluation: 20

  readiness:
    min_free_disk_mb: 2048

  server:
    listen_address: "0.0.0.0"
    port: 8080
    tls:
      mode: "off"               # "off" | "static" | "acme"
      # --- Static certificate settings (mode: static) ---
      cert_path: ""
      key_path: ""
      ca_path: ""               # Optional: CA bundle for mutual TLS (mTLS)
      min_version: "1.2"        # Minimum TLS version: "1.2" or "1.3"
      http_redirect_port: 0     # Optional: HTTP-to-HTTPS redirect listener port
      # --- ACME settings (mode: acme) ---
      acme:
        domains: []
        email: ""
        ca_url: "https://acme-v02.api.letsencrypt.org/directory"
        challenge: "http-01"    # "http-01" | "tls-alpn-01" | "dns-01"
        dns_provider: ""
        dns_provider_config: {}
        storage_path: "/var/lib/chef-migration-metrics/acme"
        renew_before_days: 30
        agree_to_tos: false
        trusted_roots: ""
    graceful_shutdown_seconds: 30

  frontend:
    base_path: "/"

  logging:
    level: INFO
    retention_days: 90

  auth:
    providers:
      - type: local

# -- Existing ConfigMap name to use instead of the chart-managed one.
# When set, the chart does not create a ConfigMap and mounts this one instead.
existingConfigMap: ""

# -- Database connection URL. Overrides config.datastore.url via DATABASE_URL env var.
# If using the bundled PostgreSQL subchart, this is auto-configured.
databaseUrl: ""

# -- Existing Kubernetes Secret containing sensitive environment variables.
# The secret should contain keys such as DATABASE_URL, LDAP_BIND_PASSWORD, etc.
existingSecret: ""

# -- Inline secret values (only used if existingSecret is not set).
# These are rendered into a chart-managed Secret.
secrets:
  databaseUrl: ""
  ldapBindPassword: ""

# -- Chef API private keys. Each key maps to a file in /etc/chef-migration-metrics/keys/.
# Provide either inline PEM content or reference an existing secret.
chefKeys:
  # -- Existing Kubernetes Secret containing Chef API private keys.
  # Each key in the secret becomes a file in the keys directory.
  existingSecret: ""
  # -- Inline key data (only used if existingSecret is not set).
  # keys:
  #   myorg-production.pem: |
  #     -----BEGIN RSA PRIVATE KEY-----
  #     ...
  #     -----END RSA PRIVATE KEY-----
  keys: {}

service:
  # -- Service type
  type: ClusterIP
  # -- Service port
  port: 80
  # -- Target port on the container
  targetPort: 8080

ingress:
  # -- Enable Ingress
  enabled: false
  # -- Ingress class name
  className: ""
  # -- Ingress annotations
  annotations: {}
    # kubernetes.io/ingress.class: nginx
    # cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: chef-migration-metrics.local
      paths:
        - path: /
          pathType: Prefix
  tls: []
  #  - secretName: chef-migration-metrics-tls
  #    hosts:
  #      - chef-migration-metrics.local

# -- TLS certificate Secret for native TLS (server.tls.mode: static).
# Not needed when TLS is terminated at the Ingress or when using ACME mode.
tlsSecret:
  # -- Existing Kubernetes TLS Secret (e.g. managed by cert-manager).
  # Must contain tls.crt and tls.key. Mounted to /etc/chef-migration-metrics/tls/.
  existingSecret: ""
  # -- Inline PEM certificate and key (only used if existingSecret is not set).
  # For production, use existingSecret instead.
  cert: ""
  key: ""

# -- ACME certificate storage (server.tls.mode: acme).
# Persistent volume for ACME account keys, issued certificates, and metadata.
# Only used when server.tls.mode is "acme".
acmeStorage:
  # -- Storage class for the ACME PVC
  storageClass: ""
  # -- Access modes
  accessModes:
    - ReadWriteOnce
  # -- Volume size (ACME data is small — 64Mi is typically sufficient)
  size: 64Mi
  # -- Use an existing PVC
  existingClaim: ""

# -- Persistent volume for git clones and cookbook downloads
persistence:
  # -- Enable persistent storage
  enabled: true
  # -- Storage class (empty string uses the cluster default)
  storageClass: ""
  # -- Access mode
  accessModes:
    - ReadWriteOnce
  # -- Volume size
  size: 10Gi
  # -- Existing PVC name (overrides chart-managed PVC)
  existingClaim: ""

resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: "2"
    memory: 1Gi

# -- Horizontal Pod Autoscaler
autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 5
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 80

# -- Liveness probe
livenessProbe:
  httpGet:
    path: /api/v1/admin/status
    port: http
  initialDelaySeconds: 60
  periodSeconds: 30
  timeoutSeconds: 5
  failureThreshold: 3

# -- Readiness probe
readinessProbe:
  httpGet:
    path: /api/v1/admin/status
    port: http
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 3
  failureThreshold: 3

# -- Node selector
nodeSelector: {}

# -- Tolerations
tolerations: []

# -- Affinity rules
affinity: {}

# -- PostgreSQL subchart (Bitnami). Enable to deploy PostgreSQL alongside the application.
postgresql:
  # -- Deploy PostgreSQL as a subchart
  enabled: true
  auth:
    # -- PostgreSQL database name
    database: chef_migration_metrics
    # -- PostgreSQL username
    username: chef_migration_metrics
    # -- PostgreSQL password (use existingSecret in production)
    password: ""
    # -- Existing secret containing the PostgreSQL password (key: password)
    existingSecret: ""
  primary:
    persistence:
      enabled: true
      size: 20Gi
```

### 6.5 Key Templates

#### Deployment

The Deployment template renders the application container with:

- Config file mounted from ConfigMap at `/etc/chef-migration-metrics/config.yml`
- Chef API keys mounted from Secret at `/etc/chef-migration-metrics/keys/`
- Sensitive environment variables (`DATABASE_URL`, `LDAP_BIND_PASSWORD`) injected from Secret via `envFrom` or individual `env` entries
- Persistent volume mounted at `/var/lib/chef-migration-metrics` for git working directories
- Liveness and readiness probes against the health endpoint
- Security context enforcing non-root execution

When the PostgreSQL subchart is enabled and no explicit `databaseUrl` is provided, the chart auto-constructs the `DATABASE_URL` from the subchart's service name and credentials.

#### ConfigMap

The ConfigMap renders `values.config` as a YAML config file. The `datastore.url` field is omitted from the ConfigMap because it is injected via the `DATABASE_URL` environment variable from a Secret.

#### Secret

The chart-managed Secret stores:

- `DATABASE_URL` (from `secrets.databaseUrl` or auto-constructed from the PostgreSQL subchart)
- `LDAP_BIND_PASSWORD` (from `secrets.ldapBindPassword`)
- Chef API private keys (from `chefKeys.keys`, unless `chefKeys.existingSecret` is set)

In production, operators should use `existingSecret` and `chefKeys.existingSecret` to reference secrets managed externally (e.g. via Sealed Secrets, External Secrets Operator, or Vault).

#### Ingress

The Ingress template is conditionally rendered when `ingress.enabled` is `true`. It supports:

- Any Ingress class (nginx, traefik, ALB, etc.) via `className` and `annotations`
- TLS termination via `tls` block with cert-manager integration
- Multiple host rules and path configurations

> **Note on TLS in Kubernetes:** When deploying behind an Ingress controller, TLS is typically terminated at the Ingress level and the application runs with `server.tls.mode: off`. The application's native TLS support (`static` or `acme` mode) is most useful for non-Kubernetes deployments, Docker Compose, or when end-to-end encryption to the pod is required. If native TLS is used, the Deployment template must mount the certificate files (for `static` mode) or provide a persistent volume for the ACME storage directory (for `acme` mode).

#### HPA

The HorizontalPodAutoscaler is conditionally rendered when `autoscaling.enabled` is `true`. It scales based on CPU and/or memory utilisation.

> **Note on replicas and the collection job:** The background collection job has a single-run constraint enforced via a database-level advisory lock. When multiple replicas are running, only one will execute the collection/analysis pipeline. The remaining replicas serve dashboard and API traffic. This makes horizontal scaling safe for the read path while ensuring data collection remains serialised.

#### PVC

The PersistentVolumeClaim provides durable storage for git clones and cookbook downloads at `/var/lib/chef-migration-metrics`. Without persistence, these are re-cloned on every pod restart.

When `persistence.existingClaim` is set, the chart uses the referenced PVC instead of creating a new one.

#### TLS Secret (static mode)

When `server.tls.mode` is `static`, the certificate and key must be made available to the pod. The chart supports two approaches:

1. **`tlsSecret.existingSecret`** — reference an existing Kubernetes TLS Secret (e.g. one managed by cert-manager). The Secret's `tls.crt` and `tls.key` are mounted into `/etc/chef-migration-metrics/tls/`.
2. **`tlsSecret.cert` / `tlsSecret.key`** — inline PEM content rendered into a chart-managed Secret. For production, `existingSecret` is recommended.

#### ACME Storage PVC

When `server.tls.mode` is `acme`, a separate PVC is created for the ACME storage directory (`acme.storage_path`). This PVC stores ACME account registrations, issued certificates, and private keys. It must survive pod restarts to avoid re-registration and CA rate limit exhaustion. The PVC is conditionally rendered only when ACME mode is active.

### 6.6 PostgreSQL Subchart

The chart includes the [Bitnami PostgreSQL chart](https://github.com/bitnami/charts/tree/main/bitnami/postgresql) as an optional dependency declared in `Chart.yaml`:

```yaml
dependencies:
  - name: postgresql
    version: "~15"
    repository: https://charts.bitnami.com/bitnami
    condition: postgresql.enabled
```

When `postgresql.enabled` is `true` (the default), a PostgreSQL instance is deployed alongside the application. For production, operators may disable the subchart and point `databaseUrl` or `secrets.databaseUrl` at an externally managed PostgreSQL instance (e.g. RDS, Cloud SQL, or an existing cluster database).

### 6.7 Usage

```bash
# Add the Bitnami repository for the PostgreSQL dependency
helm repo add bitnami https://charts.bitnami.com/bitnami

# Build chart dependencies
cd deploy/helm/chef-migration-metrics
helm dependency build

# Install with default values (bundled PostgreSQL, local auth)
helm install chef-migration-metrics . \
  --namespace chef-migration-metrics \
  --create-namespace \
  --set postgresql.auth.password=changeme

# Install with custom values file
helm install chef-migration-metrics . \
  --namespace chef-migration-metrics \
  --create-namespace \
  -f my-values.yaml

# Upgrade
helm upgrade chef-migration-metrics . \
  --namespace chef-migration-metrics \
  -f my-values.yaml

# Uninstall
helm uninstall chef-migration-metrics --namespace chef-migration-metrics
```

### 6.8 Helm Tests

The chart includes a test pod (`templates/tests/test-connection.yaml`) that performs a basic HTTP GET against the application's health endpoint to verify the deployment is functional:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: "{{ include "chef-migration-metrics.fullname" . }}-test-connection"
  annotations:
    "helm.sh/hook": test
spec:
  containers:
    - name: wget
      image: busybox:latest
      command: ['wget']
      args: ['--spider', '--timeout=5', 'http://{{ include "chef-migration-metrics.fullname" . }}:{{ .Values.service.port }}/api/v1/admin/status']
  restartPolicy: Never
```

---

## 7. Multi-Replica Considerations

When deploying multiple replicas (via `replicaCount` > 1 or HPA), the following constraints apply:

| Concern | Approach |
|---------|----------|
| **Collection job serialisation** | The background collection job acquires a PostgreSQL advisory lock before starting. Only one replica can hold the lock at a time. Other replicas skip the collection tick gracefully. |
| **Git working directory** | With `ReadWriteOnce` PVC, only one pod can mount the volume. For multi-replica deployments needing shared git state, use a `ReadWriteMany` storage class or run git operations only on a single designated replica (e.g. via a separate Deployment or CronJob). |
| **Session affinity** | Sessions are stored server-side in PostgreSQL, so any replica can serve any authenticated request. No sticky sessions are required. |
| **Database migrations** | Migrations use a database-level lock (`golang-migrate/migrate` advisory lock). Only one replica runs migrations on startup; others wait or skip. |

---

## 8. CI/CD Integration

### 8.1 Build Pipeline

The CI pipeline (e.g. GitHub Actions) should include the following stages:

| Stage | Steps |
|-------|-------|
| **Lint** | `golangci-lint`, `npm run lint` (frontend), `helm lint` |
| **Test** | Go unit tests, frontend unit tests |
| **Build** | Compile binary, build frontend, embed assets |
| **Package** | Build RPM (`make package-rpm`), DEB (`make package-deb`), container image (`make package-docker`) |
| **Publish** | Push container image to registry, upload RPM/DEB to release artifacts |
| **Helm** | Package Helm chart (`helm package`), push to chart repository or OCI registry |

### 8.2 Release Workflow

- Releases are triggered by pushing a git tag matching `v*` (e.g. `v1.2.0`).
- The version is extracted from the tag and injected into the binary, package metadata, container image labels, and Helm chart `appVersion`.
- RPM and DEB packages are attached to the GitHub Release as assets.
- The container image is pushed to the container registry with appropriate tags (see section 4.4).
- The Helm chart is packaged and published to a chart repository or OCI-compatible registry.

---

## 9. nFPM Configuration

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

### 9.1 Building the Embedded Ruby Environment

The embedded Ruby environment is built during the `make build-embedded` step (or as part of `make package-all`) into `./build/embedded/`. The build process:

1. Uses a Docker container (`ruby:3.2-bookworm`) to install gems into an isolated prefix, ensuring a consistent build regardless of the host system.
2. Installs `cookstyle`, `test-kitchen`, and `kitchen-dokken` gems (and their transitive dependencies) with `--no-document`.
3. Creates binstubs (`cookstyle`, `kitchen`) with shebangs pointing to `/opt/chef-migration-metrics/embedded/bin/ruby`.
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

## 10. Repository Layout for Packaging Files

```
deploy/
├── docker-compose/
│   ├── docker-compose.yml
│   ├── config.yml
│   ├── .env.example
│   └── README.md
├── helm/
│   └── chef-migration-metrics/
│       ├── Chart.yaml
│       ├── values.yaml
│       ├── README.md
│       └── templates/
│           ├── _helpers.tpl
│           ├── deployment.yaml
│           ├── service.yaml
│           ├── ingress.yaml
│           ├── configmap.yaml
│           ├── secret.yaml
│           ├── serviceaccount.yaml
│           ├── hpa.yaml
│           ├── pvc.yaml
│           ├── NOTES.txt
│           └── tests/
│               └── test-connection.yaml
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

Dockerfile                                  # Multi-stage build with embedded Ruby (root of repository)
Makefile                                    # Build, test, lint, and package targets
nfpm.yaml                                   # nFPM configuration for RPM and DEB builds
```

> **Note:** `Dockerfile.analysis` has been removed. The base `Dockerfile` now includes a Ruby build stage that embeds CookStyle and Test Kitchen directly. All images ship with full analysis capability.

---

## Related Specifications

- [Top-level Specification](../Specification.md)
- [Configuration Specification](../configuration/Specification.md)
- [Analysis Specification](../analysis/Specification.md) — startup validation for external tools
- [Data Collection Specification](../data-collection/Specification.md) — background job serialisation
- [Web API Specification](../web-api/Specification.md) — health endpoint used by probes
- [Datastore Specification](../datastore/Specification.md) — advisory locks for multi-replica