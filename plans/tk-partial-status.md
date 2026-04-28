# TK Partial Status

## Goal

Flag git repos as "partial" when they have a mix of passed and failed Test Kitchen results. Update the dashboard summary card to show partial count with click-through to git repo list.

## Changes

### Backend (`handle_git_repos.go`)
- TK status logic: passed > 0 AND failed > 0 → "partial"
- Update filter to accept "partial" value
- Update response response struct comment

### Backend (`handle_dashboard_compatibility.go`)
- Add `partial_repos` / `PartialRepos` to `tkSummary` and `perVersion`
- Classify repos with mixed results as partial (not failed)

### Frontend (`types/dashboard.ts`)
- Add `partial_repos` to `TestKitchenCompatibilitySummary`

### Frontend (`StatusCards.tsx`)
- Add orange/amber segment for partial in stacked bar
- Add partial legend item with link to `?tk_status=partial`

### Frontend (`GitReposPage.tsx`)
- Handle "partial" in badge variant mapping
- Support "partial" in tk_status filter

## Acceptance Criteria
- Repo with 3 passed + 2 failed → tk_status = "partial"
- Repo with 0 passed + 2 failed → tk_status = "failed"
- Repo with 3 passed + 0 failed → tk_status = "passed"
- Dashboard shows partial count in stacked bar
- Click-through filters git repo list correctly
- All tests pass
