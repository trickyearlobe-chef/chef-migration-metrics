# Data Visualisation — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Dashboard

- [ ] Ensure dashboard performs acceptably with many thousands of nodes

## Dependency Graph View

- [ ] Colour-code cookbook nodes by compatibility status (green=compatible, red=incompatible, grey=untested, amber=CookStyle-only) — nodes are currently coloured by type only (role=blue, cookbook=green); compatibility status is not fetched or applied
- [ ] Support filtering by compatibility status (show only paths involving incompatible/untested cookbooks) — not implemented; would require fetching compatibility data and joining with graph nodes
- [ ] Implement lazy loading or level-of-detail rendering for large graphs
- [ ] Link role nodes to node list filtered by that role — not yet wired (would link to `/nodes?role=ROLE_NAME`)

## Remediation Guidance View

- [ ] Include prominent notice that auto-correct is preview only — tool does not modify cookbook source — not yet rendered in `AutocorrectPreviewCard`

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

## Log Viewer

- [ ] Implement log retention purge based on configured retention period