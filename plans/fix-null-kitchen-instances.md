# Fix: Null Kitchen Instances Crash

## Goal

Fix "Cannot read properties of null (reading 'length')" when opening TK tab for a repo with no kitchen suites/platforms.

## Root Cause

`PlanRepo()` returns `Instances: nil` when suites or platforms are empty. Go serializes nil slices as JSON `null`. The frontend then crashes on `plan.instances.length`.

## Changes

1. `internal/gitkitchen/planner.go`: Initialize `Instances` to `[]PlannedInstance{}` so JSON serializes as `[]`
2. `frontend/src/components/GitKitchenSection.tsx`: Add defensive null guard `!plan.instances`
3. `internal/gitkitchen/planner_test.go`: Add test asserting empty-suites returns `[]` not null

## Acceptance Criteria

- TK tab shows nothing (returns null from component) for repos with no kitchen instances
- JSON response contains `"instances": []` not `"instances": null`
- All existing tests pass
