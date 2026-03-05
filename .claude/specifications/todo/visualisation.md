# Data Visualisation — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Dashboard

- [ ] Choose and set up web framework
- [ ] Implement Chef Client version distribution view with trend over time
- [ ] Implement cookbook compatibility status view (per cookbook, version, target Chef Client version)
- [ ] Implement confidence indicators — green for Test Kitchen pass (high), amber for CookStyle-only pass (medium), red for incompatible, grey for untested
- [ ] Implement cookbook complexity score display alongside compatibility status
- [ ] Implement stale cookbook indicator (badge/icon for cookbooks not updated in configured threshold)
- [ ] Implement node upgrade readiness summary (ready vs. blocked vs. stale counts)
- [ ] Implement stale node indicators with last check-in age display
- [ ] Implement per-node blocking reason detail view with complexity scores per blocking cookbook
- [ ] Implement interactive filters:
  - [ ] Filter by Chef server organisation
  - [ ] Filter by environment
  - [ ] Filter by role
  - [ ] Filter by Policyfile policy name
  - [ ] Filter by Policyfile policy group
  - [ ] Filter by platform / platform version
  - [ ] Filter by target Chef Client version
  - [ ] Filter by active/unused cookbook status
  - [ ] Filter by stale node status (all, stale, fresh)
  - [ ] Filter by complexity label (low, medium, high, critical)
- [ ] Implement drill-down from summary to node detail
- [ ] Implement drill-down from summary to cookbook detail
- [ ] Implement drill-down from blocking cookbook to remediation guidance
- [ ] Implement drill-down from dependency graph nodes to cookbook/role detail
- [ ] Ensure dashboard performs acceptably with many thousands of nodes

## Dependency Graph View

- [ ] Implement interactive directed graph rendering (roles and cookbooks as nodes, includes as edges)
- [ ] Colour-code cookbook nodes by compatibility status (green=compatible, red=incompatible, grey=untested, amber=CookStyle-only)
- [ ] Highlight incompatible cookbooks and the roles that depend on them
- [ ] Support filtering by specific cookbook (show subgraph involving that cookbook)
- [ ] Support filtering by specific role (show subgraph reachable from that role)
- [ ] Support filtering by compatibility status (show only paths involving incompatible/untested cookbooks)
- [ ] Implement search/filter for large graphs to focus on a subset
- [ ] Implement lazy loading or level-of-detail rendering for large graphs
- [ ] Implement alternative table view showing roles with direct and transitive cookbook dependencies
- [ ] Link cookbook nodes to cookbook detail view
- [ ] Link role nodes to node list filtered by that role

## Remediation Guidance View

- [ ] Implement remediation priority list — incompatible cookbooks sorted by priority score (complexity × blast radius)
- [ ] Display per-cookbook: complexity score/label, blast radius, auto-correctable vs. manual-fix count, top deprecations
- [ ] Implement auto-correct preview display with unified diff viewer
- [ ] Display auto-correct statistics (total offenses, correctable, remaining, files modified)
- [ ] Include prominent notice that auto-correct is preview only — tool does not modify cookbook source
- [ ] Implement migration documentation display per deprecation offense:
  - [ ] Human-readable description
  - [ ] Link to Chef migration docs
  - [ ] Chef version where deprecation was introduced/removed
  - [ ] Before/after replacement pattern code example
- [ ] Group deprecation offenses by cop name for consolidated view
- [ ] Implement effort estimation summary at top of remediation view:
  - [ ] Total cookbooks needing remediation
  - [ ] Estimated quick wins (auto-correct only)
  - [ ] Estimated manual fixes needed
  - [ ] Total blocked nodes and projected unblocked count

## Data Exports

- [ ] Implement ready node export (CSV, JSON, Chef search query string)
- [ ] Implement blocked node export (CSV, JSON) with blocking reasons and complexity scores
- [ ] Implement cookbook remediation report export (CSV, JSON)
- [ ] Ensure all exports respect currently active filters
- [ ] Implement synchronous export for small result sets
- [ ] Implement asynchronous export for large result sets (return job ID, poll for completion, download link)
- [ ] Implement export job status tracking in `export_jobs` table
- [ ] Implement export file retention and cleanup based on `exports.retention_hours`

## Notifications

- [ ] Implement webhook notification channel (HTTP POST with JSON payload)
- [ ] Implement email notification channel (SMTP)
- [ ] Implement notification trigger: cookbook status change
- [ ] Implement notification trigger: readiness milestone (configurable percentage thresholds)
- [ ] Implement notification trigger: new incompatible cookbook detected
- [ ] Implement notification trigger: collection failure
- [ ] Implement notification trigger: stale node threshold exceeded
- [ ] Implement notification filtering by organisation and cookbook
- [ ] Implement notification history display in the dashboard
- [ ] Implement notification delivery retry with configurable backoff
- [ ] Persist notification history to `notification_history` table

## Historical Trending

- [ ] Store timestamped metric snapshots during each collection run
- [ ] Implement trend charts for Chef Client version adoption over time
- [ ] Implement trend charts for node readiness counts over time
- [ ] Implement trend charts for aggregate complexity score over time
- [ ] Implement trend charts for stale node count over time

## Log Viewer

- [ ] Implement log viewer in the web UI
- [ ] Scope and display logs per collection job run (per organisation)
- [ ] Scope and display logs per cookbook git operation (clone/pull)
- [ ] Scope and display logs per Test Kitchen run (per cookbook + target Chef Client version)
- [ ] Scope and display logs per CookStyle scan (per cookbook version)
- [ ] Scope and display logs per notification dispatch (per channel)
- [ ] Scope and display logs per export job
- [ ] Implement log filtering by job type, organisation, cookbook name, and date/time
- [ ] Capture and store stdout/stderr from external processes (Test Kitchen, CookStyle, git)
- [ ] Implement log retention purge based on configured retention period