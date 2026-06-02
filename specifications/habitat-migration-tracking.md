# Habitat Migration Tracking — Component Specification

> **TL;DR** — Track parallel deployment of CC19 via Habitat alongside existing omnibus CC13/16. Collect new `chef_migration` ohai attributes to show installation state (`omnibus_only` / `hab_dormant` / `hab_active`) and nightly speculative-converge results per node. Key dashboard metric: what percentage of the fleet can already run CC19 cleanly without cookbook fixes.

## Context

As of Chef Client 19, the Habitat packaging model allows multiple versions to coexist on a node — one omnibus install (CC13/16, active) and one or more Habitat-managed installs (CC19, dormant). The migration cookbook controls which is active.

The deployment strategy:

1. **Broad dormant rollout** — deploy CC19 via Habitat fleet-wide in dormant state; CC13/16 omnibus continues running all normal converges
2. **Nightly speculative converge** — once per evening, each node runs a test converge using CC19 to detect failures without affecting production
3. **Evidence-based activation** — nodes where the speculative converge consistently succeeds can be switched to `hab_active`, making CC19 the production runner

This complements TK-based cookbook compatibility analysis (theoretical) with real-world convergence evidence (empirical).

## New Ohai Attributes

All attributes live under the `chef_migration` key in node ohai data. Source: the migration cookbook running on each node.

### Installation State

| Attribute | Type | Description |
|-----------|------|-------------|
| `chef_migration['active_chef_version']` | String | Version of the currently active chef-client (e.g. `16.18.30`, `19.3.15`) |
| `chef_migration['dormant_installed']` | Boolean | Whether a dormant (non-active) Habitat chef-client binary is present |
| `chef_migration['dormant_chef_version']` | String / nil | Version of the dormant binary; `nil` if none installed |
| `chef_migration['migration_state']` | String | One of `omnibus_only`, `hab_dormant`, `hab_active` |

**`migration_state` lifecycle:**

```
omnibus_only  →  hab_dormant  →  hab_active
```

- `omnibus_only` — only the traditional omnibus install; no Habitat CC present
- `hab_dormant` — CC19 installed via Habitat but not active; CC13/16 runs converges
- `hab_active` — CC19 via Habitat is now the active chef-client

### Speculative Converge Results

Updated nightly by the CC19 test run on each node.

| Attribute | Type | Description |
|-----------|------|-------------|
| `chef_migration['target_version']` | String / nil | CC version used for the speculative converge |
| `chef_migration['target_execution_time']` | String / nil | Timestamp of the speculative converge run |
| `chef_migration['target_converge_status']` | String / nil | Result of the speculative converge |

## Open Questions

- **Attribute tier** — are these `normal['chef_migration']` or `automatic['chef_migration']`? Determines the Chef Server partial-search path used during collection.
- **`target_converge_status` values** — what are the possible values? (`success`/`failure`? more granular error categories? exit code?) Needed to design filtering and UI state.
- **History** — track only the latest speculative converge result per node, or retain a rolling history (e.g. last 7 nights)? History enables trend views but requires a separate table.
- **Key dashboard metric** — confirm that the headline number is: *"% of fleet in `hab_dormant` state with last `target_converge_status` = success"* (i.e. nodes ready to activate).

## What Needs to Change

### Data Collection

- Add `chef_migration` to the Chef Server partial-search fields fetched per node
- Map to the correct ohai path once attribute tier is confirmed (see open questions)

### Datastore

- Add columns (or a JSONB block) to the `nodes` table for all 7 attributes
- Schema migration required

### Dashboard / UI

- Fleet-wide breakdown by `migration_state` (counts + trend over time)
- Per-node migration state and speculative converge result in the node list
- Filter nodes by `migration_state` and `target_converge_status`
- Headline metric: nodes ready to activate (see open questions)

## Related Specs

- `specifications/data-collection.md` — partial search fields and collection pipeline
- `specifications/visualisation.md` — dashboard layout and filter contracts
