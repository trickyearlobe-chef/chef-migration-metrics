# Todo — Node Kitchens (Phase 4)

## DB Migration & Datastore

- [x] Migration `0014_node_kitchen_runs.up.sql` / `.down.sql`
- [x] `internal/datastore/node_kitchen_runs.go` — types, CRUD, scan helpers
- [x] `internal/datastore/node_kitchen_runs_test.go` — validation + scan tests

## Run_list Expansion

- [x] `internal/nodekitchen/runlist.go` — parse entries, expand roles, collect cookbooks, resolve transitive deps
- [x] `internal/nodekitchen/runlist_test.go` — mock Chef API, cycle detection, missing role

## Cookbook Assembly

- [x] `internal/nodekitchen/assembly.go` — server/git/hybrid download, concurrent fetch, write to temp dir
- [x] `internal/nodekitchen/assembly_test.go` — mock interfaces, verify layout

## Synthetic Kitchen Config

- [x] `internal/nodekitchen/config_gen.go` — generate `.kitchen.yml` + `.kitchen.local.yml`
- [x] `internal/nodekitchen/config_gen_test.go` — YAML structure, platform mapping

## Runner

- [x] `internal/nodekitchen/runner.go` — orchestration: validate → expand → assemble → generate → execute → store → cleanup
- [x] `internal/nodekitchen/runner_test.go` — mock all deps, verify flow

## API

- [x] `internal/webapi/handle_node_kitchen.go` — POST trigger, GET list, GET detail, DELETE
- [x] `internal/webapi/handle_node_kitchen_test.go` — handler unit tests

## Frontend

- [x] `frontend/src/types.ts` — `NodeKitchenRun` type
- [x] `frontend/src/api.ts` — API functions for node kitchen endpoints
- [x] Node kitchen UI — trigger button, source selector, results view
- [x] Frontend tests

## Integration

- [x] Register routes in router
- [x] Wire runner in service composition (`main.go`)
- [x] Update `.gitignore` / `.dockerignore` if needed (checked — no changes needed)