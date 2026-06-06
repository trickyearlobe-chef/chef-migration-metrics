# Packaging — Build Artifacts

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

- The `User-Agent` header for Chef API requests (see [Chef API specification](chef-api.md))
- The `/api/v1/admin/status` endpoint response
- Package metadata (RPM, DEB)
- The `--version` CLI flag
