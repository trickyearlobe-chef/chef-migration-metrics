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

## Admin section layout — its own branch, after the cookstyle scope work merges

Raised by the product owner 2026-08-09 while testing the CookStyle tab split.
Deliberately NOT done on `feature/cookstyle-scan-scope`: it touches admin areas
that branch has no other reason to move, and mixing it in would make the scope
work harder to review and to revert.

**The division does not hold up.** Most side-tabs contain settings, and yet
there is also a separate Settings heading alongside them. So "is this a setting?"
does not tell you where to look, which is the only question the split is there
to answer.

**Some things belong together, separated by a top tab rather than a side tab.**
Importing owners and finding duplicate owners are two views of one job and should
sit in one side tab with tabs across the top — the shape the CookStyle hub and
the Test Kitchen hub already use.

**"Duplicate owners" is probably the wrong name.** "Deduplication" says what the
screen is for; the current name says what it lists.

Worth settling the rule before moving anything: what earns a side tab, what earns
a top tab, and where settings live. Otherwise this is rearranging rather than
fixing, and the next person moves it back.
