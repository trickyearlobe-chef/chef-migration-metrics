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

## Historical Trending

- [ ] Implement log retention purge based on configured retention period