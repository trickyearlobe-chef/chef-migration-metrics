# UI Polish — ToDo

Spec: `ui-polish-phase7.md`

## Trend Cards

- [x] Add explanatory subtitle to Node Readiness Trend card

## Cookbooks List

- [x] Add tooltip/footnote to TK column
- [x] Add `tk_status` filter to cookbook list
- [x] Backend: add `tk_status` filter param to cookbook list endpoint
- [x] Make TK column sortable on cookbooks list

## Roles List

- [x] Make TK column sortable on roles list
- [x] Make TK column filterable on roles list

## Navigation Restructure

### Kitchen

- [x] Collapse 4 nav items into hub with Hypervisor|Analysis|Batches|Queue|Settings tabs

### Admin section regrouping

- [x] Move Credentials before Organisations in SETTINGS
- [x] Move Users and Authentication from ADMIN to SETTINGS

### Performance

- [x] Merge System Stats + Performance into one page with Overview|Performance tabs

### Other

- [x] CS/TK column header tooltips fleet-wide
- [x] Staleness button styled as proper dropdown control
- [x] Ownership empty state links to settings
- [x] Remediation: removed duplicate Target Chef Version dropdown

## Git Repos Sort

- [x] Backend: fix `last_fetched` sort case
- [x] Backend: add `git_url` sort case

## Follow-up cleanup (UI Revamp Phase 1 divergences — resolved 2026-06-22)

Phase 1 shipped, then diverged from the original *planning note* (not the spec).
Reconciled 2026-06-22: the 2026-06-19 audit was stale on both items. Recording the
decisions per CLAUDE.md ("never silently diverge").

- [x] System Health sub-tabs — **accepted actual** (`Overview | Performance |
  Status`, `AdminSystemHealthPage.tsx`). The 4-tab `Overview | API | Database |
  Actions` split was only ever a planning note; the spec (`system-health-frontend.md`)
  never mandated it. Actual covers the same semantics: Overview = system stats
  (DB/runtime/tables, the "Database" content), Performance = API metrics (the "API"
  content), Status = operational status (new), and Actions stayed a top-level admin
  nav item by design. No spec divergence; no code change.
- [x] Orphaned-but-live Kitchen sub-routes — **already redirected** (audit was
  stale). `/admin/kitchen-batches`, `-queue`, `-analysis` → `/admin/test-kitchen`
  hub tabs; `/admin/config/concurrency`, `-analysis-tools` →
  `/admin/test-kitchen?tab=settings` (`App.tsx:276-317`, added 2026-06-02). Stale
  bookmarks land in the right hub. They have no nav link by design (the hub is the
  nav entry); the redirects are the intended cleanup, so nothing further to do.
