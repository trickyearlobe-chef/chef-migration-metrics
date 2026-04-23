# Plan: Node Kitchens (Phase 4)

## Goal

Implement on-demand, per-node configuration testing: select a node, expand its run_list, assemble cookbooks, generate a synthetic kitchen project, execute converge/verify/destroy on the target hypervisor, and store results.

## Specs to Read

- `.claude/specifications/kitchen-refactor.md` §Node Kitchens (L77-185)
- `.claude/specifications/node-kitchen-archive.md` §2 Archive Contents, §4 Implementation Design
- `.claude/specifications/project-conventions.md`

## Ordered Steps

### 1. DB Migration — `node_kitchen_runs` table

- Migration `0014_node_kitchen_runs.up.sql` / `.down.sql`
- Columns per spec: id (UUID PK — no natural key, runs are ephemeral), node_name, organisation_name, target_chef_version, cookbook_source, platform_name, template_used, run_list (JSONB), cookbook_versions (JSONB), converge_passed, verify_passed, converge_output, verify_output, destroy_output, duration_seconds, error_message, started_at, completed_at, vm_tracking_id (FK nullable), created_at
- Unique constraint: `(node_name, organisation_name, target_chef_version, cookbook_source)` — upsert latest
- Datastore CRUD in `internal/datastore/node_kitchen_runs.go`
- Tests in `internal/datastore/node_kitchen_runs_test.go` (validation, scan helpers)

### 2. Run_list Expansion — `internal/nodekitchen/runlist.go`

- Parse run_list entries: `role[X]`, `recipe[Y]`, `recipe[Y::Z]`
- Expand roles recursively via `chefapi.Client.GetRole()`
- Collect cookbook names from expanded recipe list
- Resolve transitive deps from `server_cookbooks.dependencies` JSONB
- Return: ordered run_list ([]string) + complete cookbook set (map[name]version)
- Tests: mock Chef API, verify expansion, cycle detection, missing role error

### 3. Cookbook Assembly — `internal/nodekitchen/assembly.go`

- Three modes: `server`, `git`, `hybrid`
- Server mode: download via `chefapi.Client.GetCookbookVersionManifest()` + `DownloadFileContent()`
- Git mode: copy from local git clone dirs
- Hybrid: git where available, server for the rest
- Concurrent downloads bounded by `ConcurrencyConfig.CookbookDownload`
- Write cookbooks into temp working dir `cookbooks/` subdirectory
- Write roles into `roles/` as JSON files
- Tests: mock interfaces, verify directory layout

### 4. Synthetic Kitchen Config — `internal/nodekitchen/config_gen.go`

- Generate `.kitchen.yml`: one platform (node's OS mapped via platform matching), one suite (node's full run_list), `chef_zero` provisioner with target version
- Generate `.kitchen.local.yml` overlay: reuse `KitchenScanner.buildOverlay` logic (extract shared helper or call directly)
- Platform mapping via `config.MatchPlatform()`
- Credential injection via existing `ResolveKitchenCredentials` + `InjectCredentialEnvVars`
- Tests: verify generated YAML structure, platform mapping integration

### 5. Node Kitchen Runner — `internal/nodekitchen/runner.go`

- Orchestrates: validate inputs → fetch node snapshot → expand run_list → resolve cookbooks → create temp dir → assemble cookbooks/roles → generate configs → execute kitchen (converge → verify → destroy) → store result → cleanup
- Reuse `KitchenExecutor` interface from `analysis/kitchen.go`
- VM tracking: insert TrackedVM record before execution, update on completion
- Timeout from `TestKitchenConfig.EffectiveTimeoutMinutes()`
- Context-aware cancellation
- Tests: mock all dependencies, verify orchestration flow

### 6. API Handlers — `internal/webapi/handle_node_kitchen.go`

- `POST /api/v1/kitchen/node-run` — trigger run (async via goroutine, return 202 with run ID)
- `GET /api/v1/kitchen/node-runs` — list results (filter by node, org, status)
- `GET /api/v1/kitchen/node-runs/:id` — detail with output
- `DELETE /api/v1/kitchen/node-runs/:id` — delete result
- Input validation, error mapping
- Tests: handler unit tests with mock datastore

### 7. Frontend — Node Kitchen UI

- `frontend/src/api.ts` — new API functions
- `frontend/src/types.ts` — `NodeKitchenRun` type
- Node detail page: "Test Node" button → modal with cookbook source selector + target version → trigger run → poll for result
- Node kitchen results list page (or section in admin TK page)
- Tests: type coverage

### 8. Wire Up & Integration

- Register routes in `internal/webapi/router.go`
- Wire `NodeKitchenRunner` in `main.go` or wherever services are composed
- Update todo files

## Acceptance Criteria

- User can select a node, choose cookbook source (server/git/hybrid), trigger a kitchen run
- Run_list is expanded correctly (roles → recipes → cookbooks with transitive deps)
- Cookbooks assembled from chosen source into temp directory
- Synthetic `.kitchen.yml` + `.kitchen.local.yml` generated with correct platform mapping
- Kitchen converge/verify/destroy executed with proper credentials
- Results stored in DB and retrievable via API
- VM tracked during execution
- All new code has tests; `go test ./...` passes; frontend tests pass