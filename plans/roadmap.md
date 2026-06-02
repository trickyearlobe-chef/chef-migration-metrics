# Implementation Roadmap

Three phases in priority order. Start a clean thread per phase.

---

## Phase 1 — UI Revamp

**Todos:** `plans/todo-ui-polish.md`
**Specs:** `specifications/ui-polish-phase7.md`, `specifications/visualisation.md`

### Navigation restructure
- Collapse 4 Kitchen nav items into 1 with sub-tabs (Config / Analysis / Batches / Queue / Settings)
- Pull Analysis Tools and Concurrency into the Kitchen Settings sub-tab
- Regroup SETTINGS: Credentials adjacent to Organisations, Authentication adjacent to Users, Credentials moved from ADMIN to SETTINGS
- Merge System Stats + Performance into one page with sub-tabs (Overview | API | Database | Actions)

### Minor polish
- CS/TK tooltips fleet-wide
- Staleness button styling in global header
- Ownership empty state → link to settings
- Remediation page: remove duplicate Target Chef Version dropdown
- Trend Cards, Cookbooks list, Roles list items from todo-ui-polish.md

### Before starting
- Verify ACME conditional UI on Server & TLS page (was in Static mode during audit — unconfirmed)

---

## Phase 2 — Disk Space Analysis

**Todos:** `plans/todo-configuration.md` (Upgrade Readiness item)
**Specs:** `specifications/configuration.md` (Upgrade Readiness section), `specifications/analysis.md` (Disk Space Evaluation section)

### Changes needed
- Backend: split `min_free_disk_mb` into `install_size_mb_linux` (3072) and `install_size_mb_windows` (6144) in config struct and defaults
- Backend: add `install_path_linux`, `install_path_windows`, `min_remaining_free_percent` to readiness config
- Backend: update disk space evaluation to use configurable paths and dual threshold (absolute + percentage)
- UI: add Upgrade Readiness settings page/section with prominent non-default path warning (cookbook assumptions + Windows knife bootstrap config dir issue)

---

## Phase 3 — Parallel Deployment Tracking

**Todos:** `plans/todo-parallel-deployment-tracking.md`
**Spec:** `specifications/parallel-deployment-tracking.md`

### Before starting
- Resolve open question: are `chef_migration` attributes `normal` or `automatic` tier?

### Changes needed
- Backend: add `chef_migration` fields to partial search, persist to nodes table (schema migration)
- Node list: deployment state badge, speculative converge result, Ready to Activate highlight, filters
- Node detail: active/staged versions, latest speculative converge result
- Dashboard: trend graph — "staged or activated" vs "speculative converge passing"
