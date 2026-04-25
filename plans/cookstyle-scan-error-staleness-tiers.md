# Plan: CookStyle Scan Error Fix + Two-Tier Staleness

## Goal

Fix the CookStyle scan error display bug and implement two-tier node staleness. Both are from customer feedback.

## Specs to Read

- `customer-feedback.md` — bug descriptions (already read)
- `staleness-tiers.md` — full spec (already read)
- `node-list-enhancements.md` — check-status icons spec (already read, not in scope)

## Part 1 — CookStyle Scan Error Distinction

Quick bug fix. Backend already stores `error_message` and `process_stderr`; frontend ignores them.

### Steps

1. Add `error_message` and `process_stderr` to `CookstyleResult` TS type (`types/cookbooks.ts`)
2. Write tests for a new `CookstyleStatusBadge` component (three states: passed, failed, scan-error)
3. Implement `CookstyleStatusBadge` — shows "Scan Error" (amber) when `error_message` is present, with expandable stderr
4. Update `GitRepoDetailPage.tsx` rendering (~L572-606) to use the new component
5. Update `customer-feedback.md` — mark bug as resolved
6. Commit

### Acceptance Criteria

- Scan error shows amber "Scan Error" badge, not red "Failed" with "0 offences"
- Error message text is visible (expandable or inline)
- Normal pass/fail rendering unchanged
- Tests pass

## Part 2 — Two-Tier Staleness

### Steps

#### Backend — Config

7. Add `StaleNodeWarningHours` and `StaleNodeCriticalDays` to `CollectionConfig` (`config.go`)
8. Add defaults (72h warning, 7d critical) and backward compat for `StaleNodeThresholdDays`
9. Add validation (warning < critical)
10. Write config tests

#### Backend — Query-Time Tier Computation

11. Add `StalenessTier` type (`fresh`/`warning`/`critical`) and computation function
12. Write tests for tier computation (boundary cases, zero ohai_time)
13. Add `staleness_tier` and `ohai_time_age_hours` to node list/detail query results
14. Update node list SQL to include `ohai_time` so tier can be computed
15. Update `NodeSnapshotFilter` to accept tier values (`fresh`/`warning`/`critical`) alongside legacy `true`/`false`

#### Backend — Dashboard + Snapshots

16. Update stale trend handler to return `warning_nodes` + `critical_nodes` alongside existing fields
17. Update collector snapshot payload to include `warning_nodes`/`critical_nodes`
18. Update readiness: critical nodes excluded from readiness, warning nodes assessed but flagged

#### Frontend — Types + Badge

19. Add `staleness_tier` to node TS types
20. Write tests for updated `StaleBadge` (three tiers: fresh, warning amber, critical red)
21. Implement three-tier `StaleBadge` with age formatting per spec
22. Update `NodesPage` to pass tier to badge and use tier filter

#### Frontend — Dashboard + Trend

23. Update `StaleTrendPoint` type with `warning_nodes`/`critical_nodes`
24. Update `StaleTrendCard` to render three series (green/amber/red)
25. Update stale filter dropdown: All / Fresh / Warning / Critical

#### Frontend — Admin Config

26. Update admin collection page with two threshold inputs
27. Commit

#### Wrap-Up

28. Update `customer-feedback.md` — mark staleness bug/enhancement resolved
29. Update `todo-visualisation.md` and `todo-tech-debt.md` if applicable
30. Delete this plan

## Acceptance Criteria

- Config: two thresholds, backward compat with old single threshold
- Nodes: three tiers computed at query time from ohai_time
- Badge: amber "Missing (2d 4h)" vs red "Gone (45d)"
- Dashboard trend: three series
- Filter: four options (all/fresh/warning/critical)
- Metric snapshots: include warning/critical counts
- All existing tests still pass
- New tests for tier computation, badge rendering, config validation