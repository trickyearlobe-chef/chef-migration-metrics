# Data Visualisation — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Next Up

- [ ] Distinguish CookStyle scan errors from genuine pass/fail on git repo detail page — when CookStyle exits non-zero but produces no parsed results (e.g. due to corrupt `.rubocop_todo.yml`), show "Scan Error" with the stderr reason instead of "Failed" with "Offences: 0 | Deprecations: 0", which falsely implies a clean scan. The log entry `process_output` already captures the CookStyle stderr; surface it in the UI.
- [x] Fix sawtooth pattern on trend charts when viewing all orgs — backend now merges per-org metric snapshots by hour bucket before returning, so one data point per collection cycle instead of one per (org, snapshot_time).

## Dashboard

- [ ] Ensure dashboard performs acceptably with many thousands of nodes

## Dependency Graph View

- [ ] Colour-code cookbook nodes by compatibility status (green=compatible, red=incompatible, grey=untested, amber=CookStyle-only) — nodes are currently coloured by type only (role=blue, cookbook=green); compatibility status is not fetched or applied
- [ ] Support filtering by compatibility status (show only paths involving incompatible/untested cookbooks) — not implemented; would require fetching compatibility data and joining with graph nodes
- [ ] Implement lazy loading or level-of-detail rendering for large graphs
- [ ] Link role nodes to node list filtered by that role — URL param infrastructure exists (nodes page reads query params), needs wiring in dependency graph component

## Remediation Guidance View

- [ ] Include prominent notice that auto-correct is preview only — tool does not modify cookbook source — not yet rendered in `AutocorrectPreviewCard`

## Notifications *(future — `internal/notify/` not yet implemented)*

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

- [x] Store timestamped metric snapshots during each collection run
- [x] Aggregate per-org snapshots into single data points per collection cycle for all-orgs view

## Log Viewer

- [ ] Implement log retention purge based on configured retention period