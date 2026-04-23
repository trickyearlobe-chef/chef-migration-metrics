# Plan: Test Kitchen Refactoring

## Goal

Refactor Test Kitchen support from a single-mode bulk scanner into a multi-component system with: config discovery/analysis, hypervisor integration (template & VM management), two distinct execution modes (Git Kitchens and Node Kitchens), and batch control with safety guardrails.

## Context

The current implementation runs TK as part of the collection cycle, generates `.kitchen.local.yml` overlays, and stores one result per cookbook. A customer with ~2000 git cookbooks running TK on LibVirt needs to migrate to kitchen-vcenter at scale. Key gaps: no per-instance results, no knowledge of what's in the existing kitchen configs, no VM lifecycle management, no batch control, no way to test a node's full run_list end-to-end.

Real-world kitchen configs examined: ~1990 cookbooks, ~80 distinct platform names mapping to ~8 canonical OS targets, 100% Vagrant driver, custom `x-custom-*` extensions for image/size selection, SSH key auth for Linux, WinRM with hardcoded creds for Windows. Some cookbooks have `.kitchen.local.yml` files (not in the sample set — impact unknown).

## Specs to Read

- `.claude/specifications/kitchen-analyser.md` (new — write first)
- `.claude/specifications/kitchen-refactor.md` (new — the main spec)
- `.claude/specifications/test-kitchen-drivers.md` (existing — partially superseded)
- `.claude/specifications/test-kitchen-mvp.md` (existing — superseded by this work)
- `.claude/specifications/node-kitchen-archive.md` (existing — Node Kitchens evolves this concept)
- `.claude/specifications/analysis.md` (existing — TK execution model)

## Phases

### Phase 1: Kitchen Analyser

Foundational. Scans cloned git repos, parses kitchen configs (including `.kitchen.local.yml` merges), catalogues platforms/drivers/suites/transport per cookbook. Stores results in DB. Exposes via API. Seeds the platform mapping UI with real data.

**Steps:**
1. Write spec: `kitchen-analyser.md`
2. Design DB schema for discovered kitchen configs
3. Write tests for YAML parsing, config merging, platform extraction
4. Implement analyser (runs after git clone, before TK execution)
5. API endpoint: `GET /api/v1/kitchen/analysis` — discovered platforms with counts, per-cookbook details
6. Frontend: platform discovery view in admin UI

**Acceptance:** User can see all unique platform names, driver configs, suite counts, and `.kitchen.local.yml` presence across the estate.

### Phase 2: Hypervisor Integration

Template discovery and VM lifecycle management. Hypervisor-agnostic interface — vCenter is the real target. Proxmox is used only as a minimal proof-of-concept to validate the interface design before committing to vCenter implementation. Keep Proxmox effort to the bare minimum needed to prove the abstractions work.

**Steps:**
1. Define hypervisor integration interface in `kitchen-refactor.md` spec
2. Minimal Proxmox backend — just `ListTemplates` and `ListManagedVMs` to prove the interface. No polish, no edge cases.
3. VM naming convention: `cmm-{cookbook}-{suite}-{platform}-{timestamp}`
4. VM tracking table in DB (created, expected_destroy, actual_destroy, orphan flag)
5. Orphan detection: query hypervisor for VMs matching CMM pattern, cross-reference with DB
6. API endpoints: `GET /api/v1/hypervisor/templates`, `GET /api/v1/hypervisor/vms`, `POST /api/v1/hypervisor/vms/:id/destroy`, `POST /api/v1/hypervisor/cleanup`
7. vCenter backend — the production-quality implementation. This is where the real effort goes.

**Acceptance:** Hypervisor interface proven on Proxmox, then fully implemented for vCenter. User can browse templates, see running CMM VMs, and clean up orphans.

### Phase 3: Platform Mapping UI (Enhanced)

Connect analyser output to hypervisor templates. The existing platform mapping config is static YAML — this makes it data-driven.

**Steps:**
1. UI shows discovered platforms (from analyser) alongside available templates (from hypervisor)
2. User maps each discovered platform name to a template (or marks "skip")
3. Regex/glob support for mapping groups: `rhel7*` → `rhel7-template`
4. Transport config per mapping (SSH vs WinRM, credential references)
5. Validation: flag unmapped platforms, warn about platform names that appear in many cookbooks
6. Store mappings in `runtime_settings` (existing mechanism)

**Acceptance:** User can set up complete platform mappings without editing YAML, seeded from real discovery data.

### Phase 4: Node Kitchens

End-to-end node configuration testing. Low volume, on-demand, manually triggered. First real hypervisor integration test.

**Steps:**
1. Write Node Kitchen section of `kitchen-refactor.md` spec
2. Node selection: by name, org, platform from `node_snapshots`
3. Run_list expansion: roles → recipes → cookbooks (from `role_dependencies` + Chef Server API)
4. Cookbook resolution: "as-is" from Chef Server OR "known-good" from git repos (user choice)
5. Synthetic kitchen config generation (one suite = node's full run_list, one platform = node's OS)
6. Cookbook assembly into temp working directory
7. Overlay generation with platform mapping + credential injection
8. Execute: converge → verify (if tests exist) → destroy
9. Result storage linked to node
10. UI: trigger from node detail page, view results

**Acceptance:** User can select a node, choose cookbook source, trigger a kitchen run, and see pass/fail results. VM is created and destroyed on the target hypervisor.

### Phase 5: Batch Definition & Git Kitchen Controls

Controls for safe bulk operation. Must be in place before large-scale Git Kitchen runs.

**Steps:**
1. Batch definition model: name, filters, limits, dry-run flag
2. Cookbook filters: by name/pattern, by platform, by owner, by previous result status
3. Cookbook exclusions: persistent skip flag + reason on cookbook record
4. Max concurrent VMs limit (separate from general TK concurrency)
5. Max batch size limit ("run at most N")
6. Dry-run mode: resolve what would run, show the list, don't execute
7. Subset selection UI
8. Batch history: track each batch run (start, end, counts, filters used)

**Acceptance:** User can define a batch that runs a specific subset, preview it in dry-run, execute it, and see results grouped by batch.

### Phase 6: Git Kitchens (Per-Instance)

Refactor bulk cookbook testing for per-instance granularity and batch control.

**Steps:**
1. Schema change: per-instance results table `(cookbook, target_version, platform, suite)` 
2. Handle `.kitchen.local.yml` conflicts (merge strategy or alternate overlay path)
3. Chef version via provisioner: override `require_chef_omnibus` in overlay
4. Integrate with batch definitions from Phase 5
5. Gradual ramp-up workflow: small subset → review results → expand
6. Post-batch orphan sweep
7. Updated dashboard: per-instance results, batch progress, platform breakdown

**Acceptance:** User can run a filtered batch of cookbooks, see per-platform/per-suite results, and safely ramp up from 10 to 100 to 2000 cookbooks.

## Relationship to Existing Specs

| Existing Spec | Status |
|---|---|
| `test-kitchen-drivers.md` | Partially superseded. Driver profiles, credential model, overlay generation still valid. Platform mapping enhanced. Per-instance results added. |
| `test-kitchen-mvp.md` | Superseded. The MVP validation approach is replaced by the phased plan above (Node Kitchens as low-volume validation). |
| `node-kitchen-archive.md` | Evolved. The "downloadable archive" concept is replaced by "live execution" (Node Kitchens). Cookbook resolution logic reused. |
| `test-kitchen-config-ui.md` | Extended. Platform mapping UI enhanced with analyser data and template discovery. |

## Global Acceptance Criteria

- Kitchen Analyser can catalogue 2000+ cookbooks and present platform summary
- Platform mappings are seeded from real data, not manual config
- Node Kitchen can test a single node's full config on vCenter/Proxmox
- Git Kitchen can run batches with subset selection and per-instance results
- Orphan VMs are tracked and cleanable
- No bulk runs without batch controls in place