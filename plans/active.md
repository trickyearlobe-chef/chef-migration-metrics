# Active Plan — Phase 1: UI Revamp

Goal: navigation restructure + minor polish (roadmap Phase 1).
Source todos: `plans/todo-ui-polish.md`.
Specs: `specifications/ui-polish-phase7.md`, `specifications/visualisation.md`.

Branch: `feature/ui-revamp-phase1`.

Start each chunk in a fresh thread; read only this plan + the relevant spec/todo
sections for the chunk in hand.

## Before starting

- Verify ACME conditional UI on the Server & TLS page (was in Static mode during
  the audit — unconfirmed). Confirm the conditional renders before touching nav.

## Chunk 1 — Navigation restructure

Scope: app nav/layout + Kitchen pages + Settings/Admin grouping.
- Collapse the 4 Kitchen nav items into 1 with sub-tabs:
  Config / Analysis / Batches / Queue / Settings.
- Pull "Analysis Tools" and "Concurrency" into the Kitchen → Settings sub-tab.
- Regroup SETTINGS: Credentials adjacent to Organisations; Authentication
  adjacent to Users; move Credentials from ADMIN to SETTINGS.
- Merge System Stats + Performance into one page with sub-tabs
  (Overview | API | Database | Actions).
Acceptance: every previously-reachable page is still reachable via the new
structure; no dead routes; existing page tests updated, not deleted.

## Chunk 2 — Minor polish

Scope: pull the discrete items from `todo-ui-polish.md` (read that file in-thread).
- CS/TK tooltips fleet-wide.
- Staleness button styling in the global header.
- Ownership empty state → link to settings.
- Remediation page: remove the duplicate Target Chef Version dropdown
  (see tech-debt "Redundant Target Version Selector" — hide when <= 1 version).
- Trend Cards, Cookbooks list, Roles list items from `todo-ui-polish.md`.
Acceptance: each item ticked off in `todo-ui-polish.md` with its source line.

## Notes

- Phase 2 (Disk Space Analysis) and Phase 3 (Parallel Deployment Tracking) remain
  queued in `roadmap.md` — do not pull them in here.
