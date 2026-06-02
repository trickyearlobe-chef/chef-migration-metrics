# Data Visualisation — ToDo

## Dashboard

- [ ] Ensure dashboard performs acceptably with many thousands of nodes

## Dependency Graph View

- [~] Colour-code cookbook nodes by compatibility status (green=compatible, red=incompatible, grey=untested, amber=CookStyle-only) — ForceGraph component now supports `compatibility_status` colouring; role dependency graph uses it. Org-level dependency graph still needs backend to supply compatibility data on nodes.
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

- [ ] Implement log retention purge based on configured retention period