# Task Summary: TLS Static Mode Implementation

**Date:** 2026-03-06
**Scope:** `internal/tls/`, `cmd/chef-migration-metrics/main.go`, todo updates

---

## What Was Done

Implemented **TLS Static Mode** (`server.tls.mode: static`) end-to-end — the hard blocker for production deployment. This is the first of three listening modes (off, static, acme) and covers operator-managed certificates.

### New Package: `internal/tls/`

Two source files, two test files, zero new external dependencies:

| File | Purpose |
|------|---------|
| `certmanager.go` | Certificate loading, validation, hot-reload, file watcher, `crypto/tls.Config` construction |
| `listener.go` | HTTPS server, HTTP→HTTPS redirect server, HSTS middleware, plain HTTP helper |
| `certmanager_test.go` | 38 tests — loading, expiry warnings, mTLS, reload, concurrency, file watcher, symlinks |
| `listener_test.go` | 38 tests — HSTS, redirect handler, config defaults, integration (actual port binding + TLS handshake) |

### Key Capabilities Delivered

- **Certificate loading & validation** — `tls.LoadX509KeyPair` with leaf parsing, cert+key pair validation, file existence checks
- **Startup warnings** — expired cert (WARN, not fatal), near-expiry (<7 days), key file permissions >0600
- **Hot-reload via SIGHUP** — `main.go` signal handler calls `CertManager.Reload()`; failed reload keeps previous cert, logs ERROR
- **Filesystem watcher** — polling-based (no `fsnotify` dependency), detects both mtime changes and symlink re-pointing (Kubernetes Secret rotation); configurable poll interval, defaults to 30s
- **Mutual TLS (mTLS)** — `WithCAPath()` loads client CA bundle, sets `RequireAndVerifyClientCert`
- **Min TLS version enforcement** — `"1.2"` or `"1.3"` only; curated ECDHE+AEAD cipher suite list for TLS 1.2
- **HTTP→HTTPS redirect listener** — `http_redirect_port` config; serves only 301 redirects, no API/health/assets
- **HSTS middleware** — `Strict-Transport-Security: max-age=31536000; includeSubDomains` on all TLS responses; also honours `X-Forwarded-Proto: https`
- **Plain HTTP helper** — `NewPlainListener()` so `main.go` uses a consistent API for both modes

### `main.go` Changes

Replaced the monolithic HTTP server block with a TLS-aware `switch` on `cfg.Server.TLS.Mode`:

- `"static"` → creates `apptls.Listener`, starts file watcher, serves HTTPS
- `"acme"` → fails fast with "not yet implemented" (clean gate for future work)
- `"off"` (default) → uses `apptls.NewPlainListener` (functionally identical to previous code)

Signal handling expanded: `SIGHUP` triggers certificate reload in TLS mode (no-op in plain HTTP mode). `SIGINT`/`SIGTERM` continue to trigger graceful shutdown.

Shutdown path now branches: `tlsListener.Shutdown()` (stops redirect server first, then HTTPS, then cert watcher) vs `plainSrv.Shutdown()`.

Removed unused `"net"` import. Added `apptls` import alias for `internal/tls` (avoids collision with stdlib `crypto/tls`).

### Design Decisions

- **No `fsnotify` dependency** — used polling + `os.Lstat`/`filepath.EvalSymlinks` instead. Avoids CGO/inotify complexity, works on all platforms, handles Kubernetes symlink-based Secret rotation correctly. 30s poll interval is negligible overhead.
- **`GetCertificate` callback pattern** — `tls.Config` uses `GetCertificate` rather than static `Certificates` field, enabling zero-downtime cert swap without rebuilding the listener.
- **LogFunc callback** — same pattern as `webapi.Router` to avoid circular imports with `internal/logging`. Bridged to `logging.ScopeTLS` in `main.go`.
- **1-second delay after detecting file change** — allows Kubernetes atomic rename operations to complete before attempting reload.

---

## Todo Progress

Updated `todo/configuration.md`: **15 tasks marked done** (all TLS Static Mode implementation items). The remaining 25 unchecked items in that section are ACME-related and can be tackled independently.

| Area | Before | After |
|------|--------|-------|
| `todo/configuration.md` | 43/83 (51%) | 58/83 (69%) |

---

## Test Coverage

76 tests total (38 certmanager + 38 listener), all passing:

- Unit tests run in `-short` mode (no network)
- Integration tests (`TestListener_Serve*`) bind to real ports, perform actual TLS handshakes with self-signed certs, verify HSTS headers, redirect behaviour, and hot-reload via `GetCertificate`
- Concurrency test: 100 concurrent `GetCertificate` readers + 10 concurrent reloaders — no races
- File watcher test: writes new cert files, verifies automatic reload within poll interval

---

## Files Changed

| File | Change |
|------|--------|
| `internal/tls/certmanager.go` | **New** — CertManager, file watcher, CA pool loader, TLS helpers |
| `internal/tls/listener.go` | **New** — Listener, redirect handler, HSTS middleware, plain HTTP helper |
| `internal/tls/certmanager_test.go` | **New** — 38 tests |
| `internal/tls/listener_test.go` | **New** — 38 tests |
| `cmd/chef-migration-metrics/main.go` | **Modified** — TLS-aware server startup, SIGHUP handler, shutdown branching |
| `.claude/Structure.md` | **Modified** — updated `internal/tls/` description |
| `.claude/specifications/todo/configuration.md` | **Modified** — 15 tasks marked done |

---

## What's Next

Recommended priorities for production readiness:

1. **`healthcheck` CLI with `--insecure` flag** — the healthcheck subcommand currently assumes HTTP; needs TLS skip-verify support
2. **Docker Compose / Helm updates** — mount TLS cert/key volumes, set `server.tls.mode: static` in example configs
3. **ACME mode** — second TLS mode, much larger scope (CertMagic integration, DNS providers, storage, renewal)
4. **`certificate_expiry_warning` notification event** — wire into the notification dispatch system