# Node Detail Fixes

## Goal

Fix TK evaluation bug, split CS/TK badges on detail, add dependency viz, cleanup dead code.

## Steps

1. Add TK to readiness evaluation (Source 3 in checkCookbookCompatibility)
2. Split CS/TK/Disk badges on node detail ReadinessCard
3. Dependency tree/graph visualisation
4. Clean up dead frontend status code

## Acceptance Criteria

- Nodes with mixed TK show correct kitchen_status
- Node detail shows separate Disk/CS/TK badges
- Dependency viz with per-cookbook status
- All tests pass
