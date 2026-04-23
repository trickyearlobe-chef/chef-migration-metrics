# Hypervisor Integration — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Hypervisor Interface + Types

- [x] `Hypervisor` interface: `ListTemplates`, `ListManagedVMs`, `DestroyVM`
- [x] `Template` type (name, OS type, notes, last modified)
- [x] `ManagedVM` type (name, hypervisor ID, power state, creation time, resource usage)
- [x] `VMStatus` enum (`creating`, `running`, `destroying`, `destroyed`, `orphaned`)
- [x] `GenerateVMName` — build name from prefix, cookbook, suite, platform, timestamp
- [x] `ParseVMName` — extract components from a CMM-convention VM name
- [x] Tests for name generation and parsing

## DB Migration (0013)

- [x] Create `vm_tracking` table (UUID PK)
- [x] Columns: vm_name, hypervisor_id, cookbook_name, suite_name, platform_name, batch_id, status, created_at, expected_destroy_at, actual_destroy_at, updated_at
- [x] Index on `status`
- [x] Index on `vm_name`
- [x] Up and down migration scripts

## Datastore Layer

- [x] `TrackedVM` type
- [x] `InsertTrackedVM` — create new VM tracking record
- [x] `GetTrackedVM` — lookup by ID
- [x] `GetTrackedVMByName` — lookup by vm_name
- [x] `UpdateTrackedVMStatus` — update status and optional hypervisor_id
- [x] `ListTrackedVMs` — all tracked VMs
- [x] `ListTrackedVMsFiltered` — filter by status
- [x] `ListOrphanedVMs` — VMs past TTL that are not destroyed
- [x] `MarkVMDestroyed` — set status=destroyed, actual_destroy_at=now()
- [x] `DeleteTrackedVM` — remove tracking record
- [x] Validation tests for param checks

## Config Additions

- [x] Add `hypervisor_type` to `TestKitchenConfig` (string: vcenter, proxmox, or empty)
- [x] Add `vm_ttl_hours` to `TestKitchenConfig` (int, default 4)
- [x] Add `vm_name_prefix` to `TestKitchenConfig` (string, default "cmm")
- [x] Add `max_concurrent_vms` to `TestKitchenConfig` (int, default 10)
- [x] Validation: vm_ttl_hours > 0, max_concurrent_vms > 0
- [x] Tests for new config fields and validation

## Proxmox Backend (Minimal POC)

- [x] HTTP client for Proxmox VE REST API
- [x] Session/token auth (`PVEAPIToken` header)
- [x] `ListTemplates` — GET `/api2/json/nodes/{node}/qemu`, filter `template: 1`
- [x] `ListManagedVMs` — GET `/api2/json/nodes/{node}/qemu`, filter by name prefix
- [x] `DestroyVM` — POST stop + DELETE `/api2/json/nodes/{node}/qemu/{vmid}`
- [x] Tests with `httptest` mock server

## vCenter Backend

- [x] HTTP client for vSphere REST API
- [x] Session auth — POST `/api/session`
- [x] `ListTemplates` — GET `/api/vcenter/vm`
- [x] `ListManagedVMs` — GET `/api/vcenter/vm` with name prefix filter
- [x] `DestroyVM` — POST power off + DELETE `/api/vcenter/vm/{vm}`
- [x] Session caching and renewal
- [x] Tests with `httptest` mock server

## Orphan Detection

- [x] `DetectOrphans` — query DB for VMs past TTL, cross-reference with hypervisor
- [x] `CleanupOrphans` — destroy orphaned VMs, update DB status
- [x] Handle hypervisor unreachable gracefully (log, don't crash)
- [x] Tests with mock hypervisor and mock DB

## API Endpoints

- [x] `GET /api/v1/hypervisor/templates` — list templates from configured hypervisor
- [x] `GET /api/v1/hypervisor/vms` — list tracked VMs from DB
- [x] `POST /api/v1/hypervisor/vms/:id/destroy` — force-destroy a VM (admin)
- [x] `POST /api/v1/hypervisor/cleanup` — destroy all orphaned VMs (admin)
- [x] Add methods to `DataStore` interface in `store.go`
- [x] Register routes in `router.go`
- [x] Handler tests for all endpoints