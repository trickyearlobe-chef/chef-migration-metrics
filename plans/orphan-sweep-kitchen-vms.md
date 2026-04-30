# Fix: Orphan Sweep for Test Kitchen VMs

## Problem

Test Kitchen creates VMs named `kitchen-<suite>-<platform>-<hex>`. The orphan sweep only detects VMs matching CMM's naming convention (`cmm-*` with embedded unix timestamp). At scale, orphaned kitchen VMs accumulate undetected.

## Approach — Two-Part Fix

### Part 1: Fix naming at source (Proxmox + vCenter)

Modify kitchen-proxmox driver and overlay generation so VMs get CMM-compatible names:
- Proxmox: change `generate_vm_name` to use unix timestamp instead of `SecureRandom.hex(4)`
- Overlay: inject `vm_name_prefix` (proxmox) / `vm_name` (vcenter) with configured CMM prefix

### Part 2: Sweep fallback for pre-existing VMs

Add uptime-based age detection as fallback for VMs that don't match the timestamp convention (legacy `kitchen-*` VMs).

## Specs to Read

- `.claude/specifications/kitchen-run-queue.md`
- `.claude/specifications/test-kitchen-drivers.md`

## Steps

1. Modify `kitchen-proxmox` driver: timestamp in name instead of hex
2. Inject `vm_name_prefix` into overlay for proxmox driver
3. Inject `vm_name` into overlay for vcenter driver
4. Add `Uptime` field to `ManagedVM` struct
5. Populate `Uptime` in `ProxmoxClient.ListManagedVMs`
6. Broaden `ListManagedVMs` to also match `kitchen-` prefix (for legacy VMs)
7. Update `SweepOrphanVMs`: when `ParseVMName` fails but name starts `kitchen-`, use Uptime for age
8. Update tests
9. Add tech debt entry for cloud driver naming

## Acceptance Criteria

- New VMs get names like `cmm-config-amazonlinux-2-1745970000`
- Sweep detects stale `kitchen-*` VMs using uptime fallback
- Non-kitchen VMs (`homeassistant`, `nexus`) untouched
- Templates still excluded
- Age threshold still respected
