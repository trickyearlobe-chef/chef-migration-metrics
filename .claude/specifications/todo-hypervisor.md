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

- [ ] Create `vm_tracking` table (UUID PK)
- [ ] Columns: vm_name, hypervisor_id, cookbook_name, suite_name, platform_name, batch_id, status, created_at, expected_destroy_at, actual_destroy_at, updated_at
- [ ] Index on `status`
- [ ] Index on `vm_name`
- [ ] Up and down migration scripts

## Datastore Layer

- [ ] `TrackedVM` type
- [ ] `InsertTrackedVM` — create new VM tracking record
- [ ] `GetTrackedVM` — lookup by ID
- [ ] `GetTrackedVMByName` — lookup by vm_name
- [ ] `UpdateTrackedVMStatus` — update status and optional hypervisor_id
- [ ] `ListTrackedVMs` — all tracked VMs
- [ ] `ListTrackedVMsFiltered` — filter by status
- [ ] `ListOrphanedVMs` — VMs past TTL that are not destroyed
- [ ] `MarkVMDestroyed` — set status=destroyed, actual_destroy_at=now()
- [ ] `DeleteTrackedVM` — remove tracking record
- [ ] Validation tests for param checks

## Config Additions

- [ ] Add `hypervisor_type` to `TestKitchenConfig` (string: vcenter, proxmox, or empty)
- [ ] Add `vm_ttl_hours` to `TestKitchenConfig` (int, default 4)
- [ ] Add `vm_name_prefix` to `TestKitchenConfig` (string, default "cmm")
- [ ] Add `max_concurrent_vms` to `TestKitchenConfig` (int, default 10)
- [ ] Validation: vm_ttl_hours > 0, max_concurrent_vms > 0
- [ ] Tests for new config fields and validation

## Proxmox Backend (Minimal POC)

- [ ] HTTP client for Proxmox VE REST API
- [ ] Session/token auth (`PVEAPIToken` header)
- [ ] `ListTemplates` — GET `/api2/json/nodes/{node}/qemu`, filter `template: 1`
- [ ] `ListManagedVMs` — GET `/api2/json/nodes/{node}/qemu`, filter by name prefix
- [ ] `DestroyVM` — POST stop + DELETE `/api2/json/nodes/{node}/qemu/{vmid}`
- [ ] Tests with `httptest` mock server

## vCenter Backend

- [ ] HTTP client for vSphere REST API
- [ ] Session auth — POST `/api/session`
- [ ] `ListTemplates` — GET `/api/vcenter/vm` with `filter.is_template=true`
- [ ] `ListManagedVMs` — GET `/api/vcenter/vm` with name prefix filter
- [ ] `DestroyVM` — POST power off + DELETE `/api/vcenter/vm/{vm}`
- [ ] Session caching and renewal
- [ ] Tests with `httptest` mock server

## Orphan Detection

- [ ] `DetectOrphans` — query DB for VMs past TTL, cross-reference with hypervisor
- [ ] `CleanupOrphans` — destroy orphaned VMs, update DB status
- [ ] Handle hypervisor unreachable gracefully (log, don't crash)
- [ ] Tests with mock hypervisor and mock DB

## API Endpoints

- [ ] `GET /api/v1/hypervisor/templates` — list templates from configured hypervisor
- [ ] `GET /api/v1/hypervisor/vms` — list tracked VMs from DB
- [ ] `POST /api/v1/hypervisor/vms/:id/destroy` — force-destroy a VM (admin)
- [ ] `POST /api/v1/hypervisor/cleanup` — destroy all orphaned VMs (admin)
- [ ] Add methods to `DataStore` interface in `store.go`
- [ ] Register routes in `router.go`
- [ ] Handler tests for all endpoints