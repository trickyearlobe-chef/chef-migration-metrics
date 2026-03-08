# Todo Audit and Next Steps

**Date:** 2026-03-06
**Component:** Project-wide todo audit, deduplication, and planning

## Context

Audited all 12 `todo/*.md` files against the actual codebase to find stale entries. Also deduplicated `ToDo.md` from a 780-line verbatim copy of all component files down to a ~45-line summary index.

## What Was Done

### ToDo.md Deduplication

- Replaced 780-line master `specifications/ToDo.md` (which duplicated every task from every component file) with a compact summary index containing a progress table, links to component files, and a shell snippet for regenerating counts.
- Removed the "update both files" rule — component files are now the single source of truth.
- Added a rule to `Claude.md` (Token Economy section) instructing future threads to refresh the summary table after completing tasks.

### Stale Items Fixed (26 tasks flipped from `[ ]` to `[x]`)

**`todo/logging.md`** — 3 items:
- `log.Printf` replacement in main.go — already done (structured logger used everywhere; only intentional DB-writer fallback remains)
- `DBWriter` wiring at startup — done via `DatastoreAdapter` in main.go
- `PurgeLogEntriesOlderThanDays` in collection cycle — done in `Collector.Run()`

**`todo/secrets-storage.md`** — 14 items:
- `RotateMasterKey` wired into startup — fully implemented in main.go
- Rotation logging (INFO on completion, ERROR on failure) — done
- 6 startup validation items (master key check, credential decrypt validation, key file permissions) — all wired in main.go
- 5 logging items (`ScopeSecrets` exists, rotation/decryption/permission logging all done)

**`todo/packaging.md`** — 9 ELK stack items:
- docker-compose.yml, logstash pipeline conf, index template JSON, .env.example, README.md — all exist and are comprehensive
- All Logstash configuration (NDJSON reading, doc_id extraction, single index, security disabled, shared volume) — done

### Stale Annotations Fixed

- Test counts: "627 tests across 4 packages" → **756 tests across 6 packages** (secrets 331, config 117, logging 93, collector 79, chefapi 74, datastore 62)
- Logging test count: 133 → 93 (tests were refactored)
- Datastore test count: 92 → 62
- Go dependencies annotation: added `gopkg.in/yaml.v3`
- Fixed "not yet created" notes for `internal/config/` and `internal/logging/` (both exist with substantial test suites)

## Final State

### Progress Summary

| Area | Done | Total | % |
|------|-----:|------:|--:|
| Specification | 32 | 32 | 100% |
| Project setup | 15 | 16 | 93% |
| Data collection | 38 | 67 | 56% |
| Analysis | 0 | 61 | 0% |
| Visualisation | 0 | 86 | 0% |
| Logging | 11 | 12 | 91% |
| Auth | 0 | 5 | 0% |
| Configuration | 43 | 83 | 51% |
| Secrets storage | 85 | 150 | 56% |
| Packaging | 18 | 97 | 18% |
| Testing | 4 | 40 | 10% |
| Documentation | 0 | 25 | 0% |
| **Total** | **246** | **674** | **36%** |

### What Exists and Works

- **App startup pipeline**: config loading → DB connect → migrations → org sync → secrets validation/rotation → collection scheduler → HTTP server → signal handling
- **Chef API client**: RSA-signed requests, partial search, concurrent pagination, role fetching (74 tests)
- **Data collection**: multi-org parallel collection, node snapshots, cookbook inventory upsert, cookbook-node usage linkage, stale node/cookbook detection (79 collector tests)
- **Config**: full YAML schema with defaults, env var overrides, validation (117 tests)
- **Logging**: structured logger with scopes, DB persistence, stdout writer, all wired into main.go (93 tests)
- **Secrets**: encryption, zeroing, credential store, resolver, rotation — all wired into startup (331 tests)
- **Datastore**: migration runner, organisations, collection runs, node snapshots, cookbooks, cookbook-node usage, log entries (62 tests)
- **CI/CD**: GitHub Actions for lint/test/build/package/publish, release workflow on `v*` tags
- **ELK stack**: docker-compose, Logstash pipeline, index template, comprehensive README

### What Doesn't Exist Yet

- `internal/analysis/` — no package yet
- `internal/webapi/` — no package yet (main.go has minimal inline health/version handlers)
- `internal/auth/` — no package yet
- `internal/notify/` — no package yet
- `internal/export/` — no package yet
- `internal/tls/` — no package yet
- `internal/elasticsearch/` — no package yet
- `frontend/` — directory exists in Structure.md but no React app created
- `Dockerfile` — referenced in Structure.md and CI but file doesn't exist yet
- `nfpm.yaml` — referenced in Structure.md and Makefile but file doesn't exist yet
- `deploy/docker-compose/` — app Docker Compose stack doesn't exist yet
- `deploy/pkg/` — systemd unit, env file, pre/post install scripts don't exist yet
- Helm chart templates — only `.helmignore` exists

## Recommended Next Steps (Priority Order)

### 1. Cookbook Usage Analysis (`todo/analysis.md` — first 8 items)
**Why first:** Highest-value next feature. Answers "what does our fleet look like?" All prerequisite data is already collected and persisted (node snapshots with cookbook lists, cookbook-node usage linkage). This is mostly SQL queries on existing tables plus a thin `internal/analysis/` Go package.

**Scope:** Determine which cookbooks are active, which versions are in use, by how many nodes, on which platforms, referenced by which roles and policies. Persist aggregated results.

**Specs to read:** `analysis/Specification.md` (§ Usage Analysis), reference `data-collection/Specification.md` and `datastore/Specification.md`.

### 2. Role Dependency Graph (`todo/data-collection.md` — 4 items)
**Why second:** Small, self-contained. Role data is already fetched (`GetRoles`, `GetRole` with `RunList`/`EnvRunLists`). Just need to parse run_list entries (simple regex), build the directed graph, and persist to `role_dependencies` table. Unlocks "which roles depend on incompatible cookbooks?" later.

### 3. Cookbook Content Fetching (`todo/data-collection.md` — Cookbook Fetching + Download Failure Handling sections)
**Why third:** Required before CookStyle or Test Kitchen can run. Currently the collection pipeline fetches cookbook *inventory* (names + versions) but not *content*. Need `GET /organizations/NAME/cookbooks/NAME/VERSION` with immutability skip and failure handling.

### 4. Web API Layer (`internal/webapi/`)
**Why fourth:** Unblocks the frontend. Need a basic router, read-only endpoints for nodes/cookbooks/usage stats/collection runs. Auth can come later.

### 5. CookStyle Integration (`todo/analysis.md` — Compatibility Testing section)
**Why fifth:** Once cookbook content is fetched, run CookStyle against them. This produces the compatibility data that powers the entire migration dashboard.

## Files Modified

- `.claude/specifications/ToDo.md` — replaced 780-line duplicate with ~45-line summary index
- `.claude/specifications/todo/logging.md` — 3 items marked done, test counts updated
- `.claude/specifications/todo/secrets-storage.md` — 14 items marked done, stale notes fixed, summary section updated
- `.claude/specifications/todo/packaging.md` — 9 ELK items marked done
- `.claude/specifications/todo/testing.md` — test count header updated
- `.claude/specifications/todo/project-setup.md` — dependency annotation updated
- `.claude/Claude.md` — updated Token Economy section (ToDo.md description and periodic refresh rule)