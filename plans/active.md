# Active — fix node-list filter URL desync + phantom drill-down filter

Branch: `fix/node-filter-url-desync`. Reported: from the dashboard readiness bar
"blocked" drill-down, the node list shows "Clear (2)" but only 1 chip (readiness);
clearing leaves only fresh nodes; toggling the global staleness filter has no
effect; a refresh is needed for the full list — and it's inconsistent (a race).

## Root causes (confirmed)

1. **Global filter URL clobber (the freshness desync + inconsistency).**
   `GlobalFilterContext` owns `target_chef_version` + `stale_tiers` in the URL and
   merges politely (functional `setSearchParams`). NodesPage's mount effect does
   `setSearchParams({}, {replace:true})` — a full wipe that races with / destroys
   those global params, while the global `staleTiers` React state persists →
   state/URL desync. Refresh "fixes" it by re-reading the wiped URL (→ no
   `stale_tiers` → all nodes); toggling/clearing behave inconsistently.
2. **Phantom `target_version` filter ("2 applied, 1 visible").**
   The dashboard readiness links go to `/nodes?readiness=blocked&target_version=X`.
   `target_version` maps to NodesPage's deployment `targetVersionFilter`, which has
   NO visible control → counted (2) but invisible (1 chip). It is also the wrong
   filter for readiness (readiness is scoped by the global target, not the
   deployment `target_version`).

## Fixes

- **A — NodesPage clears only its own params.** The once-on-mount drill-down
  cleanup deletes only the page-owned params (readiness, target_version,
  chef_version, platform, environment, role, policy_name, policy_group,
  migration_state, target_converge_status) via the functional updater, PRESERVING
  `target_chef_version`/`stale_tiers`. No more global-state desync.
- **B — readiness drill-down drops `target_version`.** `StatusCards` ready/blocked
  links go to `/nodes?readiness=ready|blocked` (no `target_version`); readiness is
  scoped by the global target. Removes the phantom invisible filter.

## TDD
- NodesPage: mounting at `?readiness=blocked&stale_tiers=fresh,warning` removes
  `readiness` but keeps `stale_tiers` after the cleanup effect.
- StatusCards: ready/blocked links href = `/nodes?readiness=…` with no `target_version`.
- Full frontend suite + tsc + lint green; rebuild binary + verify live.

## Follow-up (note, not in scope)
- Multi-target: a per-version drill-down can't re-scope the global target because
  `GlobalFilterContext` reads the URL only at mount (doesn't watch changes). For
  single-target deployments this is a non-issue. If wanted, add a URL→state
  watcher so drill-down links can set `target_chef_version`.
- `target_version` (deployment) filter is invisible when set via the
  DeploymentCards converge drill-down too — give it a visible chip.
