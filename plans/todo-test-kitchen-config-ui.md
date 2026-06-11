# Test Kitchen Configuration UI — ToDo

- [ ] End-to-end manual test: save config → trigger collection → verify overlay generated correctly
- [ ] Credential reference warnings on PUT (currently a no-op placeholder)
- [ ] Per-platform driver_settings key-value editor (currently JSON textarea — acceptable for v1)
- [ ] Driver backends `vra`, `ec2`, `vagrant` are offered in the UI dropdown
  (`AdminTestKitchenPage.tsx:23`) but unimplemented — `hypervisor.NewFromConfig`
  only wires `vcenter` + `proxmox` (returns "unknown type" otherwise). Intentional
  roadmap placeholder (confirmed 2026-06-11). Implement the hypervisor backends
  (template discovery, VM inventory, orphan sweep) before presenting them as
  ready. Overlay profile stubs already exist in `analysis/driver_profiles.go`.