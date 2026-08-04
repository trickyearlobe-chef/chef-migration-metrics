# Packaging — CI/CD Integration, Repository Layout for Packaging Files

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
