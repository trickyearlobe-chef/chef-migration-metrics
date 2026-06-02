# UI Polish — ToDo

Spec: `ui-polish-phase7.md`

## Trend Cards

- [ ] Add explanatory subtitle to Node Readiness Trend card stating what "ready" means (e.g. "Nodes with all cookbooks compatible and sufficient disk space")

## Cookbooks List

- [ ] Add tooltip/footnote to TK column: "Test Kitchen results from matching Git repository. Dash means no Git repo found."
- [ ] Add `tk_status` filter to cookbook list (multi-select: passed, failed, partial, untested, no-repo)
- [ ] Backend: add `tk_status` filter param to cookbook list endpoint
- [ ] Make TK column sortable on cookbooks list

## Roles List

- [ ] Make TK column sortable on roles list
- [ ] Make TK column filterable on roles list

## Git Repos List

- [ ] Add sort on Git URL column
- [ ] Add sort on Compatibility (CookStyle) column
- [ ] Add sort on TK Status column
- [ ] Add sort on TK Results column (by passed count or ratio)
- [ ] Add sort on Last Fetched column
