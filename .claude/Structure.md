# Project Structure

This document describes the layout of the project repository and the purpose of every directory and file. It must be kept up to date as new files and directories are added.

---

## Top-Level

```
chef-migration-metrics/
├── .claude/                        # AI assistant context, rules, and specifications
├── .dockerignore                   # Files excluded from the Docker build context
├── .github/                        # GitHub Actions CI/CD workflows
├── .gitignore                      # Files excluded from version control
├── cmd/
│   └── chef-migration-metrics/     # main package — CLI entrypoint, flag parsing, startup
├── internal/                       # Application packages (not importable externally)
│   ├── chefapi/                    # Chef Infra Server API client, RSA signing, partial search
│   ├── collector/                  # Periodic collection job orchestration (nodes, cookbooks, roles)
│   ├── analysis/                   # Cookbook usage, CookStyle, Test Kitchen, readiness evaluation
│   ├── remediation/                # Auto-correct preview, cop mapping, complexity scoring
│   ├── datastore/                  # Database access layer — queries, migrations, connection pool
│   ├── webapi/                     # HTTP handlers, router, middleware (auth, CORS, pagination)
│   ├── auth/                       # Authentication providers (local, LDAP, SAML) and RBAC
│   ├── config/                     # Configuration parsing, validation, env var overrides
│   ├── tls/                        # TLS listener setup, ACME integration, cert reload
│   ├── export/                     # CSV/JSON/NDJSON export generation, async job runner
│   ├── notify/                     # Webhook and email notification dispatch
│   ├── secrets/                    # Credential encryption, storage, resolution, rotation, zeroing
│   ├── logging/                    # Structured logger, log scopes, retention
│   ├── elasticsearch/              # NDJSON file writer, high-water-mark tracking
│   ├── embedded/                   # Embedded tool resolution (CookStyle, TK, Ruby lookup)
│   └── models/                     # Shared domain types (Node, Cookbook, ReadinessResult, etc.)
├── frontend/                       # React application (separate npm project)
├── migrations/                     # Sequential numbered SQL migration files
├── deploy/                         # Packaging, orchestration, and deployment artifacts
├── Dockerfile                      # Multi-stage container image build (Go binary + embedded Ruby + React frontend)
├── LICENSE                         # Apache License, Version 2.0
├── Makefile                        # Build, test, lint, and package targets
├── README.md                       # Project overview and getting started guide
├── nfpm.yaml                       # nFPM configuration for RPM and DEB package builds
├── go.mod                          # Go module definition
└── go.sum                          # Go module checksums
```

---

## .github/

Contains GitHub Actions workflow definitions for continuous integration and release automation. Workflows build and publish container images to GitHub Container Registry (`ghcr.io`).

```
.github/
└── workflows/
    ├── ci.yml                      # CI workflow — lint, test, build multi-arch container image on
    │                               # push to main and pull requests. Pushes commit-SHA-tagged and
    │                               # "edge"-tagged images to GHCR on main branch pushes. PR builds
    │                               # verify the image builds but do not push.
    └── release.yml                 # Release workflow — triggered by v* tags (e.g. v1.2.0). Builds
                                    # multi-arch container image and pushes to GHCR with semantic
                                    # version tags (1.2.3, 1.2, 1, latest, <commit-sha>). Packages
                                    # the Helm chart and pushes to GHCR OCI registry. Builds RPM and
                                    # DEB packages for amd64/arm64. Creates a GitHub Release with
                                    # all packages and release notes attached.
```

### Workflow Summary

| Workflow | Trigger | Image Push? | Tags |
|----------|---------|-------------|------|
| `ci.yml` | Push to `main`, PRs | Yes (main only) | `<short-sha>`, `<long-sha>`, `edge` |
| `release.yml` | `v*` tag push | Yes | `<version>`, `<major>.<minor>`, `<major>`, `latest`, `<long-sha>` |

Both workflows build `linux/amd64` and `linux/arm64` images using Docker Buildx with QEMU emulation. GitHub Actions cache (`type=gha`) is used for layer caching across builds.

---

## .claude/

Contains all context, rules, and documentation used to guide AI-assisted development on this project. Not part of the shipped application. Each spec file has a TL;DR at the top — read that before loading the full file.

```
.claude/
├── Structure.md                    # This file — project layout and orientation guide
├── Claude.md                       # Rules, conventions, and token economy guidance
└── specifications/
    ├── Specification.md            # Top-level project spec — overview, scope, component index
    ├── ToDo.md                     # Master to-do list (large — prefer per-component files below)
    ├── todo/                       # Per-component to-do files (load only what you need)
    │   ├── analysis.md
    │   ├── auth.md
    │   ├── configuration.md
    │   ├── data-collection.md
    │   ├── documentation.md
    │   ├── logging.md
    │   ├── packaging.md
    │   ├── project-setup.md
    │   ├── secrets-storage.md
    │   ├── specification.md
    │   ├── testing.md
    │   └── visualisation.md
    ├── analysis/Specification.md   # Cookbook usage, compatibility testing, remediation, readiness
    ├── auth/Specification.md       # Local accounts, LDAP, SAML, RBAC
    ├── chef-api/Specification.md   # Chef Server API endpoints, auth, pagination, quirks
    ├── configuration/Specification.md  # Full YAML config schema, env var overrides, validation
    ├── data-collection/Specification.md  # Node collection, cookbook fetching, stale detection
    ├── datastore/Specification.md  # PostgreSQL schema, tables, indexes, retention
    ├── elasticsearch/Specification.md  # NDJSON export, Logstash pipeline, ELK stack
    ├── logging/Specification.md    # Structured logging, scopes, retention
    ├── packaging/Specification.md  # RPM, DEB, container, Docker Compose, Helm, embedded Ruby
    ├── secrets-storage/Specification.md  # Credential storage (DB/env/file), encryption, rotation, resolution
    ├── tls/Specification.md        # TLS modes (off/static/ACME), certificate lifecycle
    ├── visualisation/Specification.md  # Web dashboard, filters, exports, notifications
    └── web-api/Specification.md    # REST API endpoints, auth middleware, pagination
```

---

## deploy/

Contains all packaging, orchestration, and deployment artifacts. Not compiled into the application binary.

```
deploy/
├── docker-compose/
│   ├── docker-compose.yml          # Compose file — app + PostgreSQL for local development
│   ├── config.yml                  # Example application configuration for local use
│   ├── .env.example                # Example environment variables for Compose
│   └── README.md                   # Quick-start instructions for Docker Compose
├── elk/
│   ├── docker-compose.yml          # Compose file — Elasticsearch + Logstash + Kibana for testing
│   ├── .env.example                # Example environment variables for ELK stack
│   ├── README.md                   # Quick-start instructions for ELK testing stack
│   └── logstash/
│       └── pipeline/
│           ├── chef-migration-metrics.conf  # Logstash pipeline — reads NDJSON files, indexes
│           │                                # into single Elasticsearch index
│           └── chef-migration-metrics-template.json  # Elasticsearch index template — explicit
│                                                     # field mappings for all document types
├── helm/
│   └── chef-migration-metrics/
│       ├── .helmignore             # Files excluded from Helm chart packaging
│       ├── Chart.yaml              # Helm chart metadata and dependencies
│       ├── values.yaml             # Default chart values — image, config, secrets, ingress, etc.
│       ├── README.md               # Chart installation and configuration guide
│       └── templates/
│           ├── _helpers.tpl        # Template helper functions (naming, labels, selectors)
│           ├── deployment.yaml     # Application Deployment with probes, volumes, env
│           ├── service.yaml        # ClusterIP Service exposing the web API
│           ├── ingress.yaml        # Optional Ingress resource for external access
│           ├── configmap.yaml      # ConfigMap rendering values.config as config.yml
│           ├── secret.yaml         # Secret for DATABASE_URL, LDAP password, Chef keys
│           ├── serviceaccount.yaml # ServiceAccount for the application pods
│           ├── hpa.yaml            # Optional HorizontalPodAutoscaler
│           ├── pvc.yaml            # PersistentVolumeClaim for git clones and cookbook data
│           ├── NOTES.txt           # Post-install usage notes displayed by Helm
│           └── tests/
│               └── test-connection.yaml  # Helm test pod — health endpoint check
└── pkg/
    ├── config.yml                  # Default configuration file shipped in RPM/DEB packages
    ├── env-file                    # Default environment file for systemd EnvironmentFile
    ├── chef-migration-metrics.service  # systemd unit file for RPM/DEB
    └── scripts/
        ├── preinstall.sh           # Pre-install script — create service account
        ├── postinstall.sh          # Post-install script — set ownership, enable service
        └── preremove.sh            # Pre-remove script — stop and disable service
```

---

## build/ (generated — not checked in)

Build output directory created by `make` targets. Contains compiled artifacts and the embedded Ruby environment.

```
build/
├── chef-migration-metrics          # Compiled Go binary
└── embedded/                       # Embedded Ruby environment (built by make build-embedded)
    ├── bin/
    │   ├── ruby                    # Ruby interpreter
    │   ├── cookstyle               # CookStyle binstub (shebang: #!/opt/chef-migration-metrics/embedded/bin/ruby)
    │   └── kitchen                 # Test Kitchen binstub (shebang: #!/opt/chef-migration-metrics/embedded/bin/ruby)
    ├── lib/
    │   ├── libruby*                # Ruby shared libraries
    │   └── ruby/                   # Ruby standard library and installed gems
    │       └── gems/
    │           └── 3.2.0/          # Gems: cookstyle, test-kitchen, kitchen-dokken, and dependencies
    └── ...
```

---

## Installed Layout (RPM/DEB/Container)

When packaged and installed, the filesystem layout is:

```
/usr/bin/chef-migration-metrics                             # Application binary
/etc/chef-migration-metrics/config.yml                      # Configuration file (noreplace)
/etc/chef-migration-metrics/keys/                           # Chef API private key directory (0700)
/etc/sysconfig/chef-migration-metrics                       # Environment overrides — RPM (0640)
/etc/default/chef-migration-metrics                         # Environment overrides — DEB (0640)
/var/lib/chef-migration-metrics/                            # Working directory for git clones and cookbooks
/var/log/chef-migration-metrics/                            # Optional file-based log output
/usr/lib/systemd/system/chef-migration-metrics.service      # systemd unit file
/opt/chef-migration-metrics/embedded/                       # Self-contained Ruby environment
/opt/chef-migration-metrics/embedded/bin/ruby               # Embedded Ruby interpreter
/opt/chef-migration-metrics/embedded/bin/cookstyle           # Embedded CookStyle binary
/opt/chef-migration-metrics/embedded/bin/kitchen             # Embedded Test Kitchen binary
/opt/chef-migration-metrics/embedded/lib/                   # Ruby standard library and installed gems
```

---

## Specification Relationships

Each spec references other specs. Use the task-to-spec lookup table in `Claude.md` to decide which specs to load — do not follow every reference.

| Spec | References |
|------|-----------|
| `data-collection/` | chef-api, configuration, logging, datastore, web-api |
| `analysis/` | data-collection, visualisation, datastore, configuration, logging, packaging |
| `visualisation/` | analysis, data-collection, logging |
| `web-api/` | auth, visualisation, logging, configuration, datastore, secrets-storage |
| `datastore/` | data-collection, analysis, visualisation, logging, auth, configuration, secrets-storage |
| `elasticsearch/` | configuration, datastore, logging, packaging |
| `packaging/` | configuration, analysis, data-collection, web-api, datastore, chef-api, tls, secrets-storage |
| `tls/` | configuration, web-api, auth, packaging, logging, secrets-storage |
| `logging/` | visualisation, configuration, tls, secrets-storage |
| `auth/` | configuration, secrets-storage |
| `configuration/` | auth, web-api, packaging, analysis, tls, secrets-storage |
| `secrets-storage/` | configuration, datastore, web-api, chef-api, packaging, tls, auth, logging |
| `chef-api/` | configuration, datastore, web-api, secrets-storage |

---

## Updating This Document

Whenever a file or directory is added, moved, renamed, or removed, this document must be updated in the same change. The goal is that any engineer (or AI assistant) reading this file can immediately orient themselves within the project without having to explore the filesystem manually.