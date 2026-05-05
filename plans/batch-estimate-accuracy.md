# Batch Estimate Accuracy

## Goal

Write a specification for accurate batch estimation that uses the same planning logic as the run path, replacing the current broken `len(platforms)` heuristic.

## Specs to read

- `.claude/specifications/bulk-kitchen-scanning.md` (done)
- `.claude/specifications/kitchen-run-queue.md` (done)
- `.claude/specifications/kitchen-instance-exclusions.md` (done)

## Steps

1. Write spec: `.claude/specifications/batch-estimate.md`
2. Get rubber-duck critique on the spec
3. Commit spec

## Acceptance criteria

- Spec defines the contract: estimate = count of `mapped` instances from `PlanRepo`
- Spec covers data dependencies (analysis, platform map, exclusions)
- Spec covers edge cases (no analysis, no mapped instances, stale data)
- Spec covers frontend response shape changes (if any)
- Spec is consistent with existing bulk-kitchen-scanning and kitchen-run-queue specs
