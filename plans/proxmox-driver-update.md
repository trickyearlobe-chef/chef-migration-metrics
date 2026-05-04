# Proxmox Driver Config Update

## Goal

Update CMM to work with the updated kitchen-proxmox driver (v0.4+) which uses `template_name` instead of `template_id`, auto-selects nodes, and handles VMID allocation internally.

## Specs to Read

- `.claude/specifications/kitchen-refactor.md` (hypervisor section)
- `.claude/specifications/test-kitchen-drivers.md` (overlay generation)

## Changes

1. **`internal/analysis/driver_profiles.go`** — change proxmox `ImageFieldName` from `"template_id"` to `"template_name"`
2. **`internal/hypervisor/proxmox.go`** — switch from single-node `/nodes/{node}/qemu` to cluster-wide `/cluster/resources?type=vm`. Make `node` optional. For `DestroyVM`, resolve node from cluster resources.
3. **`internal/hypervisor/factory.go`** — remove the hard requirement for `node`/`proxmox_node` in Proxmox config
4. **`internal/gitkitchen/overlay.go`** — no code change needed (already uses `profile.ImageFieldName`)
5. **Frontend** — no code change needed (already stores template name as image ID)
6. **Tests** — update `driver_profiles_test.go`, `proxmox_test.go`, `factory_test.go`, `overlay_test.go`

## Acceptance Criteria

- `template_name: <name>` emitted in Proxmox overlay (not `template_id: <id>`)
- `node` is no longer required in driver_settings for Proxmox
- `ListTemplates` returns templates from all cluster nodes
- `DestroyVM` works without a pinned node (resolves node from VMID)
- `ListManagedVMs` returns VMs from all nodes
- All existing tests pass with updated assertions
