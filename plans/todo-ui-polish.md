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

## Navigation Restructure

### Kitchen: collapse 4 nav items into 1 with sub-tabs

Currently "Test Kitchen", "Kitchen Analysis", "Kitchen Batches", "Kitchen Queue" are 4 separate
nav items under ADMIN. Replace with a single "Test Kitchen" nav item containing sub-tabs:
- **Config** (currently "Test Kitchen" — driver/platform setup)
- **Analysis** (currently "Kitchen Analysis" — platform discovery)
- **Batches** (currently "Kitchen Batches" — batch run management)
- **Queue** (currently "Kitchen Queue" — real-time queue status)

Also pull "Analysis Tools" and "Concurrency" from SETTINGS into this same page (as a **Settings**
sub-tab), since they directly configure kitchen behaviour.

### Admin section regrouping

The ADMIN/SETTINGS split scatters closely related items. Proposed moves:

- Move **Credentials** from ADMIN to SETTINGS (it's pure config, not operational)
- Move **Credentials** and **Organisations** to be adjacent in SETTINGS (you need a credential
  before you can create an org — the page itself says so)
- Group **Authentication** and **Users** together in SETTINGS (two sides of access control)

### Performance: split into sub-tabs or clarify naming

"System Stats" (disk/CPU/memory/Go heap) and "Performance" (API latency, SQL queries, table
health) both appear when investigating problems but have non-obvious names. Options:
- Rename to "Health" and "Diagnostics" to better communicate the distinction, or
- Merge into one page with sub-tabs: Overview | API | Database | Actions (Preferred)

### Other minor navigation issues

- [ ] "CS" and "TK" column headers and badges need tooltips everywhere they appear (CookStyle,
  Test Kitchen)
- [ ] "Staleness" button in the global header should look like a control (button/dropdown style),
  not plain text next to the proper dropdowns
- [ ] Ownership empty state: replace "Retry" dead-end with a link to the relevant settings page
  and an explanation of what needs to be enabled
- [ ] Remediation page: remove its own "Target Chef Version" dropdown — it duplicates the global
  header dropdown and the two are independent, which is confusing
