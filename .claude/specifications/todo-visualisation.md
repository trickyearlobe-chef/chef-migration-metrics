# Data Visualisation — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Dashboard

- [x] Make version distribution bars clickable — navigates to nodes page filtered by Chef version
- [x] Make platform distribution bars clickable — navigates to nodes page filtered by platform (including version)
- [x] Make readiness ready/blocked counts and progress bar segments clickable — navigates to nodes page with readiness filter and target version pre-set
- [ ] Ensure dashboard performs acceptably with many thousands of nodes

## Node Detail

- [x] Add filesystem/disk detail sub-page (`/nodes/:org/:name/disks`) with full Ohai filesystem data
- [x] Add "View Filesystem Details" links from node detail info grid and disk space panel
- [x] Parse Ohai filesystem JSONB (by_mountpoint format) with cross-platform value handling
- [x] Filter virtual/pseudo filesystems by default with show_all toggle
- [x] Show Windows-specific columns (drive type, encryption) when applicable
- [x] Show inode data in expandable rows with warning icon when free inodes < 70%
- [x] Handle percent_used values with trailing % suffix from Linux Ohai data

## Node Filtering

- [x] Nodes page reads readiness, target_version, chef_version, and platform from URL search params to support dashboard click-through
- [x] Platform filter matches against combined platform + platform_version string for precise filtering
- [ ] Push node filters (environment, platform, chef_version, role, policy, stale) down to SQL WHERE clauses instead of in-memory filtering — current approach loads all nodes per organisation via ListNodeSnapshotsByOrganisation then filters in Go, which will not scale to 100k+ nodes

## Dependency Graph View

- [ ] Colour-code cookbook nodes by compatibility status (green=compatible, red=incompatible, grey=untested, amber=CookStyle-only) — nodes are currently coloured by type only (role=blue, cookbook=green); compatibility status is not fetched or applied
- [ ] Support filtering by compatibility status (show only paths involving incompatible/untested cookbooks) — not implemented; would require fetching compatibility data and joining with graph nodes
- [ ] Implement lazy loading or level-of-detail rendering for large graphs
- [ ] Link role nodes to node list filtered by that role — URL param infrastructure exists (nodes page reads query params), needs wiring in dependency graph component

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