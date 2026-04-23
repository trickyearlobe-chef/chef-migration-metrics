# Plan: Hypervisor Integration (Phase 2)

## Goal

Define a hypervisor-agnostic interface for template discovery, VM inventory, and orphan cleanup. Implement Proxmox as minimal POC to prove the abstractions, then build the production vCenter backend. Add VM lifecycle tracking with TTL safety net.

## Specs

- `.claude/specifications/kitchen-refactor.md` §Hypervisor Integration, §Hypervisor Template Discovery, §Configuration Additions
- `.claude/specifications/project-conventions.md`

## Steps

### 1. Hypervisor Interface + Types

- File: `internal/hypervisor/hypervisor.go`
- `Hypervisor` interface: `ListTemplates`, `ListManagedVMs`, `DestroyVM`
- Types: `Template`, `ManagedVM`, `VMStatus` enum
- VM naming convention: `GenerateVMName`, `ParseVMName`

### 2. DB Migration (0013)

- `vm_tracking` table (UUID PK — no natural key for VMs)
- Columns per spec: vm_name, hypervisor_id, cookbook_name, suite_name, platform_name, batch_id, status, created_at, expected_destroy_at, actual_destroy_at
- Index on status, index on vm_name

### 3. Datastore Layer

- File: `internal/datastore/vm_tracking.go`
- Types: `TrackedVM`, `UpsertTrackedVMParams`
- Methods: `InsertTrackedVM`, `GetTrackedVM`, `UpdateTrackedVMStatus`, `ListTrackedVMs`, `ListTrackedVMsFiltered`, `ListOrphanedVMs`, `MarkVMDestroyed`, `DeleteTrackedVM`
- Tests: `internal/datastore/vm_tracking_test.go`

### 4. Config Additions

- Add to `TestKitchenConfig`: `hypervisor_type`, `vm_ttl_hours`, `vm_name_prefix`, `max_concurrent_vms`
- Validation: vm_ttl_hours > 0, max_concurrent_vms > 0
- Tests for new config fields

### 5. Proxmox Backend (Minimal POC)

- File: `internal/hypervisor/proxmox.go`
- HTTP client for Proxmox VE API (`/api2/json/nodes/{node}/qemu`)
- `ListTemplates`: filter VMs with `template: 1`
- `ListManagedVMs`: filter VMs matching CMM name prefix
- `DestroyVM`: `DELETE /api2/json/nodes/{node}/qemu/{vmid}`
- Tests with HTTP mock server: `internal/hypervisor/proxmox_test.go`

### 6. vCenter Backend

- File: `internal/hypervisor/vcenter.go`
- vSphere REST API client (use `net/http` + JSON, not govmomi — keep deps lean)
- Session auth: `POST /api/session`
- `ListTemplates`: `GET /api/vcenter/vm` with `filter.is_template=true`
- `ListManagedVMs`: `GET /api/vcenter/vm` with name prefix filter
- `DestroyVM`: `POST /api/vcenter/vm/{vm}/power?action=stop` then `DELETE /api/vcenter/vm/{vm}`
- Tests with HTTP mock server: `internal/hypervisor/vcenter_test.go`

### 7. Orphan Detection

- File: `internal/hypervisor/orphan.go`
- `DetectOrphans(ctx, db, hyp)`: query DB for VMs past TTL, cross-reference with hypervisor
- `CleanupOrphans(ctx, db, hyp)`: destroy orphaned VMs, update DB status
- Tests: `internal/hypervisor/orphan_test.go`

### 8. API Endpoints

- File: `internal/webapi/handle_hypervisor.go`
- `GET /api/v1/hypervisor/templates` — list templates from configured hypervisor
- `GET /api/v1/hypervisor/vms` — list tracked VMs (from DB, enriched with live state)
- `POST /api/v1/hypervisor/vms/:id/destroy` — force-destroy a VM
- `POST /api/v1/hypervisor/cleanup` — destroy all orphaned VMs
- Tests: `internal/webapi/handle_hypervisor_test.go`

### 9. Todo File

- Create `.claude/specifications/todo-hypervisor.md`

## Acceptance Criteria

- Hypervisor interface proven on Proxmox mock, production-ready for vCenter
- VM naming convention generates and parses correctly
- VM tracking table records full lifecycle
- Orphan detection flags VMs past TTL
- Cleanup endpoint destroys orphaned VMs and updates DB
- Templates browsable via API
- All tests pass, no regressions

## Commit Strategy

One commit per step. Interface + types first, then migration + datastore, then backends, then API.