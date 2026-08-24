# Test Kitchen Configuration UI — ToDo

- [ ] Driver backends `vra`, `ec2`, `vagrant` are offered in the UI dropdown
  (`AdminTestKitchenPage.tsx:23`) but unimplemented — `hypervisor.NewFromConfig`
  only wires `vcenter` + `proxmox` (returns "unknown type" otherwise). Implement
  the hypervisor backends (template discovery, VM inventory, orphan sweep) before
  presenting them as ready.
