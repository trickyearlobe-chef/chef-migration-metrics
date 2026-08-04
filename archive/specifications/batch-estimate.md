# Batch Estimate — Specification

## Problem

The batch estimate (shown when viewing or previewing a batch) always reports 0 VMs. The resolver is constructed without an analysis provider, so the VM count is never computed. Even if wired, the formula `len(platforms)` ignores suites, platform map status, and exclusions — making it inaccurate compared to what actually runs.

More broadly, the resolver's optional provider wiring is incomplete: resolution-time data (analysis, results) is not available during preview, causing filter and estimate fidelity to diverge from the run path.

Operators need an accurate preview before committing to a batch run that may consume limited hypervisor and DHCP resources for hours.

## Goal

The batch estimate must report the exact number of VMs that would be created if the batch were run right now, using the same planning logic as the execution path.

## Contract

**Estimated VMs = count of instances with status `mapped` after `PlanRepo` expansion.**

This accounts for:
- Suite × platform cross-product expansion
- Suite-level include/exclude lists
- Platform map status (unmapped, skip=true)
- Per-repo user exclusions (`kitchen_instance_exclusions` table)

Instances with any non-mapped status (`unmapped`, `skipped`, `excluded`, `user_excluded`) are NOT counted toward the VM estimate.

## Platform Filter Semantics

Batch filters include an optional `platforms` field. This selects **repos** whose analysis data contains at least one matching platform. It does NOT constrain which instances are planned within those repos.

A batch filtered to `ubuntu*` will resolve only repos that have an Ubuntu platform in their analysis, but the estimate and run will still include all mapped instances for those repos (including non-Ubuntu platforms). This is the current behaviour and is intentional — it prevents operators from accidentally skipping suites that test cross-platform interaction.

## Resolver Provider Wiring

The `batch.Resolver` accepts optional providers for platform filtering (`KitchenAnalysisProvider`) and previous-status filtering (`TestKitchenResultProvider`). Both preview and run paths MUST construct the resolver with concrete providers wired:

- `KitchenAnalysisProvider` — required when `platforms` filter is set (otherwise repos with matching platforms are incorrectly excluded)
- `TestKitchenResultProvider` — required when `previous_status` filter is set (otherwise no repos match)

If these providers are not wired, the resolver cannot apply the corresponding filters, causing the resolved set to diverge from expectations. The fix must wire both providers in `resolveBatch`.

## Data Dependencies

The estimate requires:

| Data | Source | Failure mode |
|------|--------|--------------|
| Kitchen analysis result | `GetKitchenAnalysisResultByName(repoName)` | Repo skipped, logged as warning |
| Platform map | `liveConfig().AnalysisTools.TestKitchen.PlatformMap` | Global; if empty, all instances are unmapped |
| Instance exclusions | `loadInstanceExclusions(repoName)` | Repo skipped, logged as warning |

Failure modes match the run path exactly: if analysis or exclusions cannot be loaded, the repo is skipped (not planned, not executed). The estimate must not silently fall back to "assume none" when the run path would skip the repo.

## Resolution Flow

1. Apply batch filters to the git repo list (existing resolver logic — unchanged).
2. Cap at `max_count` if set.
3. For each resolved cookbook:
   a. Load analysis result from DB. If unavailable → mark `no_analysis`, skip.
   b. Load instance exclusions. If error → mark `exclusion_error`, skip.
   c. Call `PlanRepo(analysis, platformMap, exclusions...)`.
   d. If plan errors → mark `plan_error`, skip.
   e. Count instances by status.
4. Sum `mapped` counts across planned cookbooks = total estimated VMs.

## Architecture

The planning/estimation logic lives in the `batch` package as a `Planner` (or equivalent) that accepts data-access interfaces. The `webapi` layer wires concrete implementations but does not contain planning logic. Both the estimate endpoint and the run path use the same `batch` planner.

## Response Shape

### `BatchEstimate` (backend → frontend)

```
{
  "total_cookbooks": 30,
  "total_estimated_vms": 42,
  "skipped_cookbooks": 3,
  "per_platform": {
    "ubuntu-24.04": 18,
    "centos-stream-9": 14,
    "windows-2022": 10
  },
  "cookbooks": [
    {
      "name": "example_cookbook",
      "git_repo_url": "ssh://git@git.example.com:7999/cookbooks/example_cookbook",
      "planning_status": "planned",
      "planning_note": "",
      "estimated_vms": 4,
      "total_instances": 6,
      "unmapped": 1,
      "skipped": 0,
      "excluded": 1,
      "user_excluded": 0,
      "platforms": ["ubuntu-24.04", "centos-stream-9"],
      "suites": ["default", "ha-cluster"]
    },
    {
      "name": "unscanned_cookbook",
      "git_repo_url": "ssh://git@git.example.com:7999/cookbooks/unscanned_cookbook",
      "planning_status": "no_analysis",
      "planning_note": "no kitchen analysis data available",
      "estimated_vms": 0,
      "total_instances": 0,
      "unmapped": 0,
      "skipped": 0,
      "excluded": 0,
      "user_excluded": 0,
      "platforms": [],
      "suites": []
    }
  ]
}
```

### Field definitions

| Field | Type | Description |
|-------|------|-------------|
| `total_cookbooks` | int | Cookbooks resolved by filters, after `max_count` cap |
| `total_estimated_vms` | int | Sum of `mapped` instances across all planned cookbooks |
| `skipped_cookbooks` | int | Subset of `total_cookbooks` that could not be planned |
| `per_platform` | map | Mapped instance count grouped by platform name |
| `cookbooks[].planning_status` | string | One of: `planned`, `no_analysis`, `exclusion_error`, `plan_error` |
| `cookbooks[].planning_note` | string | Informational, not contract-stable. Empty when `planned`. |
| `cookbooks[].estimated_vms` | int | Mapped instances for this cookbook (0 if not planned) |
| `cookbooks[].total_instances` | int | Total suite×platform combinations from `PlanRepo`. 0 if not planned OR if analysis has no suites/platforms. |
| `cookbooks[].unmapped` | int | Instances with no platform map entry |
| `cookbooks[].skipped` | int | Instances where platform map has `skip=true` |
| `cookbooks[].excluded` | int | Instances excluded by suite include/exclude lists |
| `cookbooks[].user_excluded` | int | Instances excluded by operator via exclusions table |
| `cookbooks[].platforms` | []string | Platform names from analysis. Empty if `planning_status != planned`. |
| `cookbooks[].suites` | []string | Suite names from analysis. Empty if `planning_status != planned`. |

## Edge Cases

### No analysis data for a repo

The repo appears in `cookbooks[]` with `planning_status: "no_analysis"` and `estimated_vms: 0`. It is counted in `skipped_cookbooks`. The frontend renders it distinctly (e.g. greyed row with the planning note).

Note: if a `platforms` filter is active, repos without analysis data are excluded during resolution (before planning). In that case they never appear in `cookbooks[]` at all. The `no_analysis` status only applies to repos that passed all resolution filters but then failed at the planning stage.

### Planned but empty (no suites or platforms in analysis)

A repo whose analysis contains zero suites or zero platforms will be planned successfully by `PlanRepo` (returns empty instances). This results in `planning_status: "planned"`, `total_instances: 0`, `estimated_vms: 0`. This is distinct from "not planned" — the analysis was loaded and parsed, it just had nothing to expand. No warning indicator is needed; the operator can see the cookbook's analysis page for details.

### All instances unmapped or excluded

The cookbook appears with `planning_status: "planned"`, `estimated_vms: 0`, `total_instances > 0`. The breakdown fields explain why. The frontend should surface this so the operator knows the batch will skip this repo at run time.

### Data freshness

Both the estimate and run path read from the same source: persisted analysis snapshots in the DB and the live runtime config. Neither reads directly from the git repo. If analysis data is stale (repo's kitchen.yml changed since the last scan), both paths see the same stale data. The estimate is computed on demand and not cached, so it always reflects the latest DB state at query time.

### Platform map changes between estimate and run

If the admin changes the platform map between previewing and running a batch, the run uses the updated map. The estimate was a point-in-time snapshot. This is acceptable — operators should re-preview if they change configuration.

## Consistency with Run Path

The estimate MUST use the same code path as `prepareBatchInstances`:
- Same `PlanRepo` function
- Same platform map source (`liveConfig()`)
- Same exclusions source (`loadInstanceExclusions`)
- Same failure semantics (skip repo on any load/plan error)

This is enforced by extracting the planning logic into a shared service in the `batch` package that both the estimate endpoint and the run path call.

## Frontend Changes

- Display `skipped_cookbooks` count when > 0 (e.g. "3 cookbooks have no analysis data")
- Per-cookbook table: show `planning_status` badge for non-planned repos
- Cookbooks with `estimated_vms: 0` and `total_instances > 0` show a warning indicator
- `per_platform` breakdown uses mapped instance counts (already rendered, just needs accurate data)
- Existing fields (`total_cookbooks`, `total_estimated_vms`, `cookbooks[].estimated_vms`) retain their names for backward compatibility; new fields are additive

## Backward Compatibility

- All existing fields retain their names and JSON keys
- New fields (`planning_status`, `planning_note`, `total_instances`, `unmapped`, `skipped`, `excluded`, `user_excluded`, `skipped_cookbooks`) are additive
- Frontend should handle missing new fields gracefully (older API versions)
- `total_cookbooks` definition is tightened (post-filter, post-max_count) — this matches the previous implementation's count which was already post-filter

## Performance

`PlanRepo` is a pure in-memory expansion (JSON unmarshal + loop). The expensive part is loading analysis data per repo. For 30 cookbooks this is 30 DB queries — acceptable for a preview endpoint. For very large batches (100+ cookbooks), consider a batch-load query that fetches all analysis results in one round-trip.

## Out of Scope

- Caching the estimate (it's cheap enough to compute on demand)
- Predicting DHCP availability or hypervisor capacity
- Time estimates for batch completion
- Changing platform filter semantics (repo-level selection is intentional)
