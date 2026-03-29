# Plan: Test Kitchen Driver Abstraction Specs

## Goal

Update specifications to support a pluggable Test Kitchen driver architecture, replacing the current Docker-only (kitchen-dokken) assumption. The immediate driver is VMware vCenter (with vRA planned), but the spec must be generic enough to pivot to EC2, Azure, or any other shipped driver via config change alone.

## Context

- Nuclia feature request: `773c9e4aaee84833b5cd9b371f5949fb`
- Packaging already ships 10 kitchen drivers (vcenter, vra, ec2, azurerm, google, vagrant, etc.)
- Current analysis spec hardcodes `kitchen-dokken` / Docker flow
- Customer's git repos have `.kitchen.yml` configured for LibVirt — cannot modify them
- VMware team provides temporary vCenter access; long-term target is vRA
- Platform image names in LibVirt won't match target driver image identifiers
- Need to validate that tested platforms match actual production usage

## Specs to Read

- `.claude/specifications/analysis.md` — Test Kitchen invocation (L87–235), startup validation (L680–699)
- `.claude/specifications/configuration.md` — analysis_tools section (L158–192)
- `.claude/specifications/secrets-storage.md` — credential types (L27–46), validation (L466–497)
- `.claude/specifications/datastore.md` — credentials table (L49–104), git_repo_test_kitchen_results (L344–379)
- `.claude/specifications/packaging.md` — embedded gems / kitchen drivers (L431–489)
- `.claude/specifications/data-collection.md` — collected node attributes (L50–72)

## Steps

1. ~~Create new spec `.claude/specifications/test-kitchen-drivers.md`~~ ✅ Done — covers:
   - Generic driver model (opaque settings bags, built-in profiles, custom profile)
   - Driver override via `.kitchen.local.yml` overlay
   - Credential injection using existing `generic` type (no per-driver credential types)
   - Platform image mapping (single `image` field mapped by profile)
   - Platform coverage analysis (kitchen platforms vs production node platforms)
   - Driver migration examples (vCenter→vRA, vCenter→EC2)

2. ~~Update `analysis.md`~~ ✅ Done — generalised Test Kitchen invocation:
   - Overlay generation supports any driver via profile + opaque settings
   - Reference test-kitchen-drivers.md for driver/credential/platform details
   - Startup validation for non-dokken drivers (credential checks replace Docker check)
   - Platform coverage gap reporting as post-analysis step

3. ~~Update `configuration.md`~~ ✅ Done — extended `analysis_tools.test_kitchen`:
   - `driver` setting (profile name)
   - `driver_settings` / `driver_secrets` maps
   - `platform_map` list
   - `image_field_name` for custom profiles
   - Cross-reference test-kitchen-drivers.md for full schema

4. ~~Update `datastore.md`~~ ✅ Done:
   - `driver` and `platform_name` columns added to `git_repo_test_kitchen_results`
   - `cookbook_platform_coverage` table added
   - Cross-reference test-kitchen-drivers.md for JSONB structure

5. ~~Create `todo-test-kitchen-drivers.md`~~ ✅ Done

6. ~~Commit each spec update individually~~ ✅ Done (4 commits)

## Acceptance Criteria

- New test-kitchen-drivers spec is driver-agnostic — adding a new driver is a config change
- No per-driver credential types — uses existing `generic` type
- No per-driver code paths in overlay generation — profiles + opaque settings bags
- Analysis spec no longer assumes Docker/dokken as the only driver
- Configuration spec has complete `test_kitchen` section with driver selection and platform mapping
- Datastore spec has platform coverage table and extended TK results columns
- Platform coverage analysis is specified (kitchen platforms vs node platforms)
- vRA and EC2 migration paths documented as config-only changes
- All existing spec content preserved — changes are additive