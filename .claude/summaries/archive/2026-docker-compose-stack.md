# Docker Compose Local Development Stack

**Date:** 2026-03-06
**Components:** `deploy/docker-compose/`

## Context

The backend is essentially complete — all analysis, collection, remediation, secrets, webapi handlers, and main.go wiring are done. The most recent summary recommended implementing cookbook content extraction to disk as the #1 next step, but upon investigation that was already fully implemented (todo already marked `[x]`). The previous summary's "Known Gaps" section was written before the extraction code existed.

Looking at the overall project state (50% complete), the next most impactful small task was creating the Docker Compose local development stack. This is a prerequisite for anyone wanting to evaluate or develop against the application — it provides a single-command way to stand up the app with PostgreSQL.

## What Was Done

### 1. Created `deploy/docker-compose/docker-compose.yml`

Two services:

- **`db`** — `postgres:16-bookworm` with named `pgdata` volume, `pg_isready` health check (5s interval, 10 retries), all credentials configurable via `.env` with sensible defaults, port exposed for local debugging.
- **`app`** — `chef-migration-metrics:latest` (or built from root `Dockerfile` via `build:` block), `DATABASE_URL` constructed from `.env` variables and injected as environment variable, `config.yml` and `keys/` mounted read-only, `cookbook_data` named volume for persistent git clones and cache, depends on `db` with `service_healthy` condition.

Follows the spec in `packaging/Specification.md` § 5.4 exactly.

### 2. Created `deploy/docker-compose/.env.example`

Documented environment variables: `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` (required — uses Compose `?` error syntax), `POSTGRES_PORT`, `APP_IMAGE`, `APP_PORT`, build args (`VERSION`, `GIT_COMMIT`, `BUILD_DATE`), `LDAP_BIND_PASSWORD`, and `CMM_CREDENTIAL_ENCRYPTION_KEY`.

### 3. Created `deploy/docker-compose/config.yml`

Example application configuration pre-configured for Docker Compose:
- No `datastore.url` — `DATABASE_URL` env override takes precedence
- `analysis_tools.embedded_bin_dir` set to `/opt/chef-migration-metrics/embedded/bin`
- `server.listen_address` set to `0.0.0.0`
- Example organisation with `client_key_path` pointing to `/etc/chef-migration-metrics/keys/`
- All optional sections (elasticsearch, notifications, SMTP, exports, auth) present but commented out

### 4. Created `deploy/docker-compose/README.md`

222-line README covering:
- Prerequisites
- 4-step quick start
- Services and files tables
- Environment variable reference
- Application configuration guidance
- Chef API key mounting instructions with security note
- Common operations (logs, health check, rebuild, psql, stop, reset)
- Volumes explanation
- Port conflict resolution
- ELK stack integration pointer
- Troubleshooting section (missing password, app exits, DNS, permissions)

### 5. Housekeeping

- Updated `Structure.md` — expanded `deploy/` section with Docker Compose file listing
- Updated `todo/packaging.md` — marked 4 Docker Compose items as `[x]` with implementation details
- Updated `ToDo.md` — packaging 18→22/97 (22%), total 336→341/675 (50%)
- Archived 2 oldest summaries (`2026-node-readiness-evaluation.md`, `2026-enrich-offenses-and-pipeline-wiring.md`) to keep active count ≤ 8

## Final State

- `go build ./...` — clean
- `go vet ./...` — clean
- No Go code changes — this was purely deployment/documentation
- Docker Compose stack cannot be verified end-to-end yet (requires a `Dockerfile` at root, which does not exist yet)

### Progress Overview

| Area | Done | Total | % | Change |
|------|-----:|------:|--:|--------|
| Packaging | 22 | 97 | **22%** | was 18% |
| **Total** | **341** | **675** | **50%** | was 49% |

## Known Gaps

- **No root `Dockerfile` yet** — `docker compose up -d` will fail because there is no `Dockerfile` to build from. The `app` service has a `build:` block pointing to `../../Dockerfile`. This is the next blocking item for Docker Compose to actually work.
- **`deploy/docker-compose/keys/` directory** — not created (git-ignored anyway). README instructs users to `mkdir -p keys`.
- **Docker Compose verification items** — 3 todo items remain for actually testing the stack (`docker compose up`, app connects to DB, `docker compose down -v`).

## Files Created

- `deploy/docker-compose/docker-compose.yml`
- `deploy/docker-compose/.env.example`
- `deploy/docker-compose/config.yml`
- `deploy/docker-compose/README.md`

## Files Modified

- `.claude/Structure.md` — expanded deploy/docker-compose section
- `.claude/specifications/todo/packaging.md` — 4 items marked done
- `.claude/specifications/ToDo.md` — updated counts

## Recommended Next Steps

### 1. Create the multi-stage `Dockerfile` (medium ~30k tokens)
- This is the highest-impact next task — it unblocks Docker Compose, container image publishing, and CI/CD packaging
- Three stages: Go build, Ruby/embedded build, runtime (Debian slim)
- Must produce a static binary with `CGO_ENABLED=0`, copy embedded Ruby tree, create non-root user, add HEALTHCHECK
- **Spec:** `packaging/Specification.md` (§ 3. Container Image, § 1. Build Artifacts)
- **Todo:** `todo/packaging.md` — 9 container image items

### 2. Create systemd unit + nFPM config for RPM/DEB (medium ~20k tokens)
- `deploy/pkg/` directory with systemd service file, default config, env file, pre/post install scripts
- `nfpm.yaml` at project root
- **Spec:** `packaging/Specification.md` (§ 2. RPM/DEB Packages)
- **Todo:** `todo/packaging.md` — 8 RPM items + 3 DEB items

### 3. Update testing todo to reflect existing coverage (small ~5k tokens)
- Many testing todo items (cookbook usage analysis, readiness, complexity scoring, etc.) already have comprehensive tests but aren't marked done
- Quick audit: mark items as done where tests exist, update counts
- **Todo:** `todo/testing.md`

### 4. Configuration — TLS subsystem (medium ~25k tokens)
- `server.tls.mode` parsing, static cert loading, ACME integration
- **Spec:** `configuration/Specification.md` (§ TLS), `tls/Specification.md`
- **Todo:** `todo/configuration.md` (40 remaining TLS items)