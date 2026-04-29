# Kitchen Instance Exclusions — Implementation Plan

## Goal

Implement the `kitchen-instance-exclusions` spec: DB table, planner integration, API endpoints, and frontend UI.

## Specs to Read

- `.claude/specifications/kitchen-instance-exclusions.md`

## Steps

1. Create branch `feature/kitchen-instance-exclusions`
2. DB migration 0022: `kitchen_instance_exclusions` table
3. Datastore CRUD methods (Create, List, Delete)
4. Add `DataStore` interface methods + mock stubs
5. Planner: add `InstanceStatusUserExcluded`, accept exclusions param, update `PlanResult`
6. Planner tests
7. API handler: GET/POST/DELETE endpoints
8. API handler tests
9. Router wiring
10. Frontend: types, API client functions
11. Frontend: exclusion badge + "Exclude" action on git kitchen results
12. Frontend: exclusion list/remove UI
13. Run all tests, verify clean build

## Acceptance Criteria

- Exclusions persisted in DB with reason and author
- Planner marks matching instances as `user_excluded`
- API supports CRUD with admin-only writes
- UI shows excluded status with reason; allows creating/removing exclusions
- All existing tests continue to pass
