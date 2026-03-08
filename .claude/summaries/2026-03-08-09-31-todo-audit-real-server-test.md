# TODO Audit — Real Chef Server Testing Readiness

**Date:** 2026-03-08
**Purpose:** Full audit of all 12 TODO files to determine what is needed to test the current functionality against a real Chef Infra Server.
**Status:** Audit complete — core pipeline is ready, only config + database setup needed.

---

## Audit Summary

### What's Already Done (Core Pipeline — Fully Built and Wired)

The entire collection → analysis → dashboard pipeline is implemented and wired end-to-end in `cmd/chef-migration-metrics/main.go`. The binary compiles cleanly.

| Area | Status | Key Details |
|------|--------|-------------|
| Configuration | ✅ Complete | YAML parsing, env var overrides, validation (117 tests) |
| Database + Migrations | ✅ Complete | 6 migration files, auto-run on startup, custom runner |
| Structured Logging | ✅ Complete | Stdout + DB writer, scoped, severity levels (93 tests) |
| Secrets Management | ✅ Complete | Master key, encryption, rotation, key file perms (331 tests) |
| Organisation Sync | ✅ Complete | Config → database sync on startup |
| Chef API Client | ✅ Complete | RSA v1.3 signing, partial search, cookbook/role fetching (87 tests) |
| Node Collection | ✅ Complete | Concurrent pagination, multi-org parallel, stale detection |
| Cookbook Fetching | ✅ Complete | Chef server download + git clone/pull, failure handling |
| Role Dependency Graph | ✅ Complete | Fetch, parse run_lists, build directed graph, persist |
| Cookbook Usage Analysis | ✅ Complete | Active set, per-version aggregation, Policyfile support (51 tests) |
| CookStyle Scanning | ✅ Complete | Parallel scans, version profiles, skip unchanged (34 tests) |
| Test Kitchen Integration | ✅ Complete | Parallel runs, skip unchanged HEAD, per-phase output (75+ tests) |
| Autocorrect Previews | ✅ Complete | Temp copy, cookstyle --auto-correct, unified diff (52 tests) |
| Complexity Scoring | ✅ Complete | Weighted scores, blast radius, labels (41 tests) |
| Readiness Evaluation | ✅ Complete | Per-node per-target, disk space, blocking reasons (77 tests) |
| Collection Scheduler | ✅ Complete | Cron-based, skip-if-running, panic recovery, resume |
| Web API / Router | ✅ Complete | All dashboard, nodes, cookbooks, remediation, graph, export, logs endpoints (275 tests) |
| Frontend SPA | ✅ Complete | React 18 + Vite + Tailwind, all views, filters, trend charts |
| TLS (static mode) | ✅ Complete | Cert reload, mTLS, HSTS, SIGHUP, fsnotify (79 tests) |
| Export System | ✅ Complete | CSV, JSON, Chef search query, async/sync, cleanup ticker |
| Embedded Tool Resolution | ✅ Complete | cookstyle, kitchen, docker, git — PATH fallback (21 tests) |
| **Total Tests** | **1,666 passing** | **Across 11 packages, 0 failures** |

### What's NOT Done But Does NOT Block Testing

| Area | Status | Why It Doesn't Block |
|------|--------|---------------------|
| Authentication/Authorisation | Not started | App runs without auth in dev mode |
| Notifications (webhook, email) | Not started | Collection/analysis works without notifications |
| ACME TLS | Not started | Use `mode: off` for dev testing |
| Helm chart | Not started | Use Docker Compose or bare binary |
| RPM/DEB packaging | Mostly not started | Run from source |
| Elasticsearch/NDJSON export | Not started | Dashboard works without ELK |
| Documentation | Not started | Developer-led testing for now |
| Functional test suite (`make functional-test`) | Not wired — no `TestFunctional` functions exist yet | Run the actual app instead |
| Stale cookbook detection tests | Not started | Logic is implemented and wired, just untested |
| Dependency graph traversal tests | Not started | Graph building works, traversal is a frontend concern |
| Notification tests | Not started | Notifications not implemented yet |
| Export NDJSON tests | Not started | Elasticsearch export not implemented yet |
| Dashboard performance with many thousands of nodes | Not verified | Functional correctness first |
| Log retention purge from UI trigger | Not started | Purge runs automatically in the collection cycle |
| Several dependency graph UI enhancements | Partial | Graph renders and is interactive, colour-coding by compatibility status not yet done |

---

## Next Steps

These are the actions required to test the current functionality against a real Chef Infra Server:

- [ ] **Create `deploy/pkg/config.yml`** — `make run` and `make dev` both pass `--config deploy/pkg/config.yml` but this file does not exist. Create a local dev config with real Chef server organisation details (chef_server_url, org_name, client_name, client_key_path), target_chef_versions, a datastore URL pointing at a local/Docker Postgres, and `server.tls.mode: off`. The `deploy/docker-compose/config.yml` is a good starting template but needs adjustment for non-Docker paths (e.g. remove embedded_bin_dir or point it at a local install, set a real datastore.url instead of relying on env var injection). Use `logging.level: DEBUG` for maximum visibility on the first run.

- [ ] **Stand up a PostgreSQL instance** — The app requires PostgreSQL. Quickest option: `cd deploy/docker-compose && docker compose up db -d` which gives Postgres on `localhost:5432` (needs `.env` file with at minimum `POSTGRES_PASSWORD` set). Alternatively use an existing local Postgres and create the database manually. The app auto-runs all 6 migrations on first startup.

- [ ] **Build the frontend assets** — Without a built frontend, the app serves a plain-text placeholder. Run `make build-frontend` (or `make build` which includes it) to produce `frontend/dist/` with the React SPA. The API endpoints work regardless, but the dashboard UI requires this step.

- [ ] **Run the application** — `make run` (builds binary then runs it) or `make dev` (uses `go run` for faster iteration). Watch the structured logs — with DEBUG level you'll see every Chef API call, cookbook download, and analysis step. The collection won't fire until the cron schedule triggers. Set `schedule: "* * * * *"` temporarily for immediate testing, or wait for the configured interval.

- [ ] **Verify the end-to-end flow** — Confirm the app connects to the Chef server, collects nodes, fetches cookbooks, runs analysis (if cookstyle/kitchen are available on PATH), evaluates readiness, and serves the dashboard at `http://localhost:8080`. Check the log viewer at `/logs` for any errors.

---

## File Locations Referenced

- `cmd/chef-migration-metrics/main.go` — application entrypoint, all wiring
- `deploy/docker-compose/config.yml` — Docker Compose example config (template for local dev config)
- `deploy/docker-compose/docker-compose.yml` — Docker Compose with `db` and `app` services
- `deploy/docker-compose/.env.example` — env var template for Docker Compose
- `deploy/pkg/config.yml` — **does not exist yet**, needed by `make run` / `make dev`
- `Makefile` — `run`, `dev`, `build`, `build-frontend`, `functional-test` targets
- `migrations/` — 6 migration pairs (0001–0006), auto-applied on startup
- `.claude/specifications/todo/` — 12 TODO files covering all components