# Todo — Node Kitchens (Phase 4)

## DB Migration & Datastore

- [ ] Migration `0014_node_kitchen_runs.up.sql` / `.down.sql`
- [ ] `internal/datastore/node_kitchen_runs.go` — types, CRUD, scan helpers
- [ ] `internal/datastore/node_kitchen_runs_test.go` — validation + scan tests

## Run_list Expansion

- [x] `internal/nodekitchen/runlist.go` — parse entries, expand roles, collect cookbooks, resolve transitive deps
- [x] `internal/nodekitchen/runlist_test.go` — mock Chef API, cycle detection, missing role

## Cookbook Assembly

- [ ] `internal/nodekitchen/assembly.go` — server/git/hybrid download, concurrent fetch, write to temp dir
- [ ] `internal/nodekitchen/assembly_test.go` — mock interfaces, verify layout

## Synthetic Kitchen Config

- [ ] `internal/nodekitchen/config_gen.go` — generate `.kitchen.yml` + `.kitchen.local.yml`
- [ ] `internal/nodekitchen/config_gen_test.go` — YAML structure, platform mapping

## Runner

- [ ] `internal/nodekitchen/runner.go` — orchestration: validate → expand → assemble → generate → execute → store → cleanup
- [ ] `internal/nodekitchen/runner_test.go` — mock all deps, verify flow

## API

- [ ] `internal/webapi/handle_node_kitchen.go` — POST trigger, GET list, GET detail, DELETE
- [ ] `internal/webapi/handle_node_kitchen_test.go` — handler unit tests

## Frontend

- [ ] `frontend/src/types.ts` — `NodeKitchenRun` type
- [ ] `frontend/src/api.ts` — API functions for node kitchen endpoints
- [ ] Node kitchen UI — trigger button, source selector, results view
- [ ] Frontend tests

## Integration

- [ ] Register routes in router
- [ ] Wire runner in service composition
- [ ] Update `.gitignore` / `.dockerignore` if needed