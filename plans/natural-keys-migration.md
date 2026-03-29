# Plan — B0 Natural Keys Migration

## Goal

Replace synthetic UUID primary keys with composite natural keys across all
tables, Go structs, web API responses, and frontend types.

## Specification

`.claude/specifications/natural-keys-migration.md`

## Approach

The migration touches every layer. Execute bottom-up (schema → datastore →
collector → webapi → frontend) so each layer compiles before the next changes.
Split into commits by tier/layer to keep each change reviewable.

## Steps

### Phase 1 — SQL Migration

- [ ] 1a. Write `migrations/0009_natural_keys.up.sql` (Tier 4→1 order)
- [ ] 1b. Write `migrations/0009_natural_keys.down.sql`
- [ ] 1c. Verify migration applies cleanly with `go test ./...`

### Phase 2 — Go Datastore (Tier 1 root entities)

- [ ] 2a. `credentials` — remove `ID` field, update queries to PK on `name`
- [ ] 2b. `organisations` — remove `ID` field, all methods use `name`
- [ ] 2c. `users` — remove `ID` field, PK on `username`
- [ ] 2d. `owners` — remove `ID` field, PK on `name`
- [ ] 2e. `git_repos` — remove `ID` field, PK on `(name, git_repo_url)`
- [ ] 2f. Update/remove all `GetXxxByID` methods for Tier 1 entities
- [ ] 2g. Fix tests — commit

### Phase 3 — Go Datastore (Tier 2 dependent entities)

- [ ] 3a. `collection_runs` — `organisation_name` replaces `organisation_id`
- [ ] 3b. `server_cookbooks` — `organisation_name` replaces `organisation_id`; remove `GetServerCookbookIDMap`
- [ ] 3c. `node_snapshots` — `organisation_name` replaces `organisation_id`
- [ ] 3d. `sessions` — keep UUID PK; `username` replaces `user_id`
- [ ] 3e. `ownership_assignments` — `owner_name`/`organisation_name` replace UUID FKs
- [ ] 3f. `role_dependencies` — `organisation_name` replaces `organisation_id`
- [ ] 3g. `metric_snapshots` — BIGSERIAL PK; `organisation_name` replaces UUID FK
- [ ] 3h. `cookbook_usage_analysis` + `cookbook_usage_detail` — natural keys
- [ ] 3i. `log_entries` — BIGSERIAL PK; `collection_run_org` replaces UUID FK
- [ ] 3j. `git_repo_committers` — remove `ID`; PK on `(git_repo_url, author_email)`
- [ ] 3k. `ownership_audit_log` — BIGSERIAL PK
- [ ] 3l. Fix tests — commit

### Phase 4 — Go Datastore (Tier 3–4 entities)

- [ ] 4a. `node_readiness` — PK `(organisation_name, node_name, target_chef_version)`
- [ ] 4b. `server_cookbook_cookstyle_results` — composite natural PK
- [ ] 4c. `server_cookbook_complexity` — composite natural PK
- [ ] 4d. `git_repo_cookstyle_results` — composite natural PK
- [ ] 4e. `git_repo_complexity` — composite natural PK
- [ ] 4f. `git_repo_test_kitchen_results` — composite natural PK
- [ ] 4g. `cookbook_usage_detail` — composite natural PK
- [ ] 4h. `cookbook_platform_coverage` — PK on `cookbook_name`
- [ ] 4i. `server_cookbook_autocorrect_previews` — composite natural PK
- [ ] 4j. `git_repo_autocorrect_previews` — composite natural PK
- [ ] 4k. Fix tests — commit

### Phase 5 — Collector Pipeline

- [ ] 5a. Replace all `organisation.ID` usage with `organisation.Name`
- [ ] 5b. Replace `node_snapshot.ID` with `(orgName, nodeName)` in readiness
- [ ] 5c. Remove `GetServerCookbookIDMap` usage; upsert by natural key
- [ ] 5d. Fix tests — commit

### Phase 6 — Web API Handlers

- [ ] 6a. Remove UUID `id` fields from all JSON response structs
- [ ] 6b. `GET /api/v1/logs/:id` → accepts `int64`
- [ ] 6c. `DELETE /owners/:name/assignments/:id` → composite key params
- [ ] 6d. Remove `collection_run_id` from trend responses
- [ ] 6e. Fix tests — commit

### Phase 7 — Frontend

- [ ] 7a. Update `types.ts` — remove UUID fields, add natural key fields
- [ ] 7b. Update React keys from `item.id` to composite natural keys
- [ ] 7c. Update `api.ts` — log detail uses number, assignment delete uses composite
- [ ] 7d. Update committer selection tracking to composite keys
- [ ] 7e. Fix tests — commit

### Phase 8 — Cleanup

- [ ] 8a. Run full test suite (`go test`, `go build`, `golangci-lint`, `npm test`, `npm run build`)
- [ ] 8b. Update datastore spec to reflect natural key schema
- [ ] 8c. Remove B0 from `todo-tech-debt.md`
- [ ] 8d. Delete this plan

## Acceptance Criteria

- No UUID primary keys remain except `sessions` and `export_jobs`.
- No `pgcrypto` usage except for `sessions` and `export_jobs`.
- `go test ./...` passes (all packages).
- `go build ./...` clean.
- `golangci-lint run ./...` — 0 issues.
- `npm run build` clean.
- `npm test` passes.
- No `ID string` fields on entity structs (except Session, ExportJob).
- API responses contain no UUID `id` fields (except session tokens and export job IDs).
- Frontend has no UUID type fields (except session/export).