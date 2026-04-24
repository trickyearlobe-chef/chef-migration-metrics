# Plan — Manual Kitchen Triggers & Install Method

## Goal

Enable manual single-instance kitchen runs (both Node Kitchen and Git Kitchen) via UI buttons, add per-image install method config (`download` vs `baked_in`), and fix provisioner naming for WS26 compatibility.

## Specs to Read

- `.claude/specifications/kitchen-refactor.md` — overlay generation, provisioner logic
- `.claude/specifications/kitchen-analyser.md` — platform/suite discovery
- `.claude/specifications/configuration.md` — config schema conventions
- `.claude/specifications/project-conventions.md` — Go/frontend conventions

## Changes

### 1. Provisioner naming: `chef_ice` → `chef-ice`

String change in three overlay generators:
- `internal/analysis/kitchen.go` `buildOverlay`
- `internal/batch/kitchen_runner.go` `buildInstanceOverlay`
- `internal/nodekitchen/config_gen.go` (if it generates provisioner blocks)

Update tests that assert the overlay content.

### 2. Per-image install method + `chef_client_path`

Config additions to `ImageEntry`:
- `install_method`: `"download"` (default) | `"baked_in"`
- `chef_client_path`: string, used when `baked_in` (e.g. `/usr/bin/chef-client`)

Overlay logic per image:
- `download` (current default): `product_version` / `download_url` as today
- `baked_in`: emit `require_chef_omnibus: false` + `chef_client_path: <path>`

Validation:
- `baked_in` requires `chef_client_path` to be non-empty
- `download` ignores `chef_client_path` (warn if set)

Config store + admin UI: add fields to image config section.

### 3. Manual git kitchen trigger

API: `POST /api/v1/kitchen/git-run`

Request body:
```
{
  "git_repo_name": "nginx",
  "target_chef_version": "19.2.12",
  "platform_name": "ubuntu-22.04",
  "suite_name": "default"
}
```

Handler:
- Validate required fields
- Look up git repo in DB (must exist, clone_status = cloned)
- Look up latest commit SHA from git repo record
- Use `batch.KitchenRunner.RunInstance` in a goroutine
- Upsert result into `git_kitchen_results` (no batch_id)
- Return 202 Accepted

Router: wire via `WithGitKitchenRunner` option (reuse `batch.KitchenRunner`).

### 4. Frontend — Git repo detail page trigger

On `GitRepoDetailPage.tsx`, add a "Run Kitchen Test" section:
- Target version dropdown (from `fetchFilterTargetChefVersions`)
- Platform dropdown (from kitchen analysis data for this cookbook)
- Suite dropdown (from kitchen analysis data for this cookbook)
- "Run Test" button → calls `POST /api/v1/kitchen/git-run`
- Status message + link to results

### 5. Frontend — Node detail page

Already has `NodeKitchenSection` with trigger form. Verify it works with the newly wired runner. No changes expected.

## Spec Updates

Update `kitchen-refactor.md`:
- Provisioner name `chef-ice` (was `chef_ice`)
- Per-image `install_method` and `chef_client_path`
- Manual git kitchen trigger endpoint

Update `configuration.md`:
- `ImageEntry` new fields

## Ordered Steps

1. Update spec with new fields and naming
2. Config: add `InstallMethod` + `ChefClientPath` to `ImageEntry`, validation, defaults
3. Overlay: change `chef_ice` → `chef-ice`, add install method logic
4. API: `POST /api/v1/kitchen/git-run` handler + tests
5. Frontend: git repo detail trigger section
6. Update todos, commit per logical unit

## Acceptance Criteria

- `chef-ice` emitted in overlays for major >= 19
- `baked_in` images emit `require_chef_omnibus: false` + `chef_client_path`
- `download` images behave as before (backward compatible)
- Manual git kitchen trigger fires a single instance, stores result
- Git repo detail page has a working "Run Kitchen Test" button
- All existing tests pass, new tests for changed behaviour
- No new tech debt