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

## Follow-up cleanup (UI Revamp Phase 1 divergences — audited 2026-06-19)

Phase 1 shipped, but the implementation diverged from the original plan. These are
NOT regressions — record the decision rather than silently differing (CLAUDE.md).
Active chunk: `plans/active.md` § "Chunk 2". Decide how to refactor, then do it.

- [ ] System Health sub-tabs: plan was `Overview | API | Database | Actions`;
  actual is `Overview | Performance | Status` (`AdminSystemHealthPage.tsx`). The
  API/Database split was never built (folded into Performance); `Actions` stayed a
  top-level admin nav item. Decide: accept actual + update plan/roadmap, or build
  the intended split. Touches the same hub as the new `/admin/status` Status tab.
- [ ] Orphaned-but-live Kitchen sub-routes: `/admin/kitchen-batches`,
  `/admin/kitchen-queue`, `/admin/kitchen-analysis`, `/admin/config/concurrency`,
  `/admin/config/analysis-tools` still resolve directly but have no nav link and no
  redirect. `/admin/performance` got a `<Navigate>` redirect to the hub
  (`App.tsx:376`); these did not. Decide: add matching redirects, or keep as deep
  links. A stale bookmark currently lands outside the new nav.
