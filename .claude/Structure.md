# Project Structure

Quick-reference layout. One-line descriptions only — read the source or specs for detail.

---

## Top-Level

```
chef-migration-metrics/
├── cmd/chef-migration-metrics/main.go  # CLI entrypoint — config, DB, migrations, HTTP server, signals
├── internal/
│   ├── analysis/                   # Cookbook usage, CookStyle, Test Kitchen, readiness evaluation
│   ├── chefapi/                    # Chef Infra Server API client and RSA auth signing
│   ├── collector/                  # Periodic collection pipeline (nodes, cookbooks, roles, git, scheduling)
│   ├── config/                     # Configuration parsing, validation, env var overrides
│   ├── datastore/                  # PostgreSQL repositories, migrations runner, connection pool
│   ├── elasticsearch/              # NDJSON file writer and high-water-mark tracking
│   ├── embedded/                   # Embedded tool resolution (CookStyle, TK, Ruby, git, Docker)
│   ├── frontend/                   # Frontend FS provider — embed registration, disk fallback, nil detection
│   ├── export/                     # CSV/JSON/NDJSON export generation
│   ├── logging/                    # Structured logging — Logger, writers (stdout, DB, memory)
│   ├── models/                     # Shared domain types
│   ├── notify/                     # Webhook and email notification dispatch
│   ├── remediation/                # Auto-correct previews, cop-to-docs mapping, complexity scoring
│   ├── secrets/                    # Credential encryption, storage, resolution, rotation
│   ├── tls/                        # TLS listener setup, cert manager, HSTS, HTTP→HTTPS redirect, file watcher
│   ├── webapi/                     # HTTP router, REST handlers, WebSocket EventHub, response helpers
│   └── auth/                       # Authentication providers (local, LDAP, SAML) and RBAC
├── migrations/                     # Sequential numbered SQL migration files (0001–0005)
├── frontend/                       # React application + Go embed package (frontendfs)
│   ├── embed.go                    # //go:embed all:dist — bakes built SPA into the Go binary
│   ├── embed_test.go               # Tests for the embedded FS
│   └── dist/                       # Vite build output (gitignored; placeholder created by Makefile)
├── deploy/                         # Packaging and deployment artifacts (see below)
├── Makefile                        # Build, test, lint, and package targets
├── Dockerfile                      # Multi-stage container build
├── nfpm.yaml                       # nFPM config for RPM/DEB packages
├── go.mod / go.sum                 # Go module definition
├── LICENSE                         # Apache 2.0
└── README.md                       # Project overview
```

## .github/

```
.github/workflows/
├── ci.yml                          # Lint, test, build on push/PR
├── release.yml                     # Tag-triggered RPM/DEB/container build and publish
└── helm.yml                        # Helm chart lint and package
```

## .claude/

```
.claude/
├── Claude.md                       # Development rules and token economy guidance
├── Structure.md                    # This file
├── summaries/                      # Recent task summaries (≤8 active; read only the latest at thread start)
│   ├── 2026-tls-static-mode.md                # TLS static certificate mode
│   ├── 2026-webapi-foundation-websocket.md    # WebAPI foundation and WebSocket EventHub
│   ├── 2026-wire-main-and-close-todos.md      # Wire main.go to webapi router, close todo items
│   ├── 2026-wire-usage-analysis-into-collection.md  # Wire usage analysis into collection pipeline
│   ├── 2026-03-08-01-35-data-exports.md       # Data Exports — generators, handlers, cleanup, frontend ExportButton
│   ├── 2026-03-08-01-48-data-exports-wiring.md # Data Exports wiring — handler tests, filterNodes refactor, cleanup ticker, ExportButton in pages
│   ├── 2026-03-08-01-56-project-completion-estimate.md # Project completion estimate — ~35–39 threads remaining across 8 work areas
│   ├── 2026-03-08-02-11-data-collection-complete.md # Data Collection complete — checkpoint/resume + dashboard failed cookbook display
│   └── archive/                               # Older summaries — historical reference only
└── specifications/
    ├── Specification.md            # Top-level project spec and component index
    ├── ToDo.md                     # Progress table index — tasks live in todo/*.md
    ├── todo/                       # Per-component task checklists
    ├── analysis/Specification.md
    ├── auth/Specification.md
    ├── chef-api/Specification.md
    ├── configuration/Specification.md
    ├── data-collection/Specification.md
    ├── datastore/Specification.md
    ├── elasticsearch/Specification.md
    ├── logging/Specification.md
    ├── packaging/Specification.md
    ├── secrets-storage/Specification.md
    ├── tls/Specification.md
    ├── visualisation/Specification.md
    └── web-api/Specification.md
```

## deploy/

```
deploy/
├── docker-compose/                 # App + PostgreSQL for local development
│   ├── docker-compose.yml          # Service definitions, volumes, networking
│   ├── config.yml                  # Example application configuration
│   ├── .env.example                # Documented environment variables — copy to .env
│   └── README.md                   # Quick-start and operational instructions
├── elk/                            # Elasticsearch + Logstash + Kibana testing stack
├── helm/chef-migration-metrics/    # Helm chart (templates, values, README)
└── pkg/                            # RPM/DEB assets (systemd unit, config, install scripts)
```

## build/ (generated — not checked in)

```
build/
├── chef-migration-metrics          # Compiled Go binary
└── embedded/                       # Self-contained Ruby env (cookstyle, kitchen, gems)
```

## Installed Layout (RPM/DEB/Container)

```
/usr/bin/chef-migration-metrics                         # Binary
/etc/chef-migration-metrics/config.yml                  # Configuration
/etc/chef-migration-metrics/keys/                       # Chef API private keys
/var/lib/chef-migration-metrics/                        # Git clones and cookbook data
/opt/chef-migration-metrics/embedded/                   # Ruby + CookStyle + Test Kitchen
```

---

## Updating This Document

When a file or directory is added, moved, or removed, update this document in the same change. Keep entries to one line — detail belongs in specs and source files.