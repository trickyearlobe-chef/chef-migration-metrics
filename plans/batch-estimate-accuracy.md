# Batch Estimate Accuracy

## Goal

Implement accurate batch estimation using the same `PlanRepo` planning logic as the run path.

## Specs

- `.claude/specifications/batch-estimate.md` ✓ (written and reviewed)
- `.claude/specifications/bulk-kitchen-scanning.md`
- `.claude/specifications/kitchen-instance-exclusions.md`

## Steps

1. ~~Write spec~~ ✓
2. Write batch planner tests (TDD)
3. Implement `batch.Planner`
4. Wire resolver providers in webapi
5. Update frontend types
6. Update frontend UI

## Acceptance criteria

- Estimate uses `PlanRepo` and counts only `mapped` instances
- Response includes `planning_status`, breakdown counts per cookbook
- Resolver wired with analysis + results providers
- All existing tests still pass
- New planner tests cover: planned, no_analysis, exclusion_error, plan_error, empty

