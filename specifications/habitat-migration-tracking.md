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

## Behaviour

### Node Readiness

A node is considered **ready to activate** when:
- `migration_state` is `hab_dormant` (CC19 is installed)
- `target_converge_status` is `success` (last speculative converge passed)

These nodes can be switched to `hab_active` with confidence.

### Speculative Converge Status

`target_converge_status` is either `success` or `fail`. Only the latest result per node is stored — no rolling history.

## What Needs to Change

### Data Collection

- Add `chef_migration` attributes to the Chef Server partial-search fields fetched per node
- Map to the correct ohai path once attribute tier is confirmed (see open questions)

### Datastore

- Add columns to the `nodes` table for all 7 attributes
- Schema migration required

### Node List

- Add `migration_state` column (badge: `omnibus_only` / `hab_dormant` / `hab_active`)
- Add speculative converge result column (`success` / `fail` / not yet run)
- Mark nodes as **Ready to Activate** where `hab_dormant` + `success`
- Filter by `migration_state` and `target_converge_status`

### Node Detail View

- Show current `migration_state`, `active_chef_version`, `dormant_chef_version`
- Show latest speculative converge: status, version tested, execution time
- Highlight **Ready to Activate** prominently when criteria met

### Dashboard Trend Graph

Two series over time:

1. **Dormant installed** — count of nodes in `hab_dormant` or `hab_active` state (CC19 is present)
2. **Speculative converge passing** — count of nodes with `target_converge_status = success`

The gap between the two lines represents nodes where CC19 is installed but the nightly test converge is still failing (cookbook or config issues to resolve).

## Related Specs

- `specifications/data-collection.md` — partial search fields and collection pipeline
- `specifications/visualisation.md` — dashboard layout and filter contracts
