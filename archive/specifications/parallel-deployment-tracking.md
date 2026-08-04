# Parallel Deployment Tracking — Component Specification

> **TL;DR** — Track parallel deployment of a target Chef Client version alongside the currently active version. Collect `chef_migration` ohai attributes to show per-node deployment state and nightly speculative-converge results. Key dashboard metric: how many nodes already run the target version cleanly. The mechanism is version-agnostic and applies to CC19, CC20, and future upgrades.

## Context

The migration cookbook supports running two Chef Client versions side-by-side on a node: one **active** (runs all normal converges) and one **staged** (installed but dormant). The cookbook controls which is active.

The deployment strategy:

1. **Broad staged rollout** — deploy the target CC version fleet-wide in staged/dormant state; the current version continues running all normal converges
2. **Nightly speculative converge** — once per evening, each node runs a test converge using the staged version to detect failures without affecting production
3. **Evidence-based activation** — nodes where the speculative converge consistently succeeds can be switched to active

This complements TK-based cookbook compatibility analysis (theoretical) with real-world convergence evidence (empirical).

> **Note on internals:** The side-by-side install mechanism is implemented via Habitat, but this is an internal detail of the migration cookbook. The UI must not expose Habitat-specific terminology — speak only in terms of "active version", "staged version", and "deployment state".

## New Ohai Attributes

All attributes live under the `chef_migration` key in node ohai data. Source: the migration cookbook running on each node.

### Installation State

| Attribute | Type | Description |
|-----------|------|-------------|
| `chef_migration['active_chef_version']` | String | Version of the currently active chef-client (e.g. `16.18.30`, `19.3.15`) |
| `chef_migration['dormant_installed']` | Boolean | Whether a staged (non-active) chef-client binary is present |
| `chef_migration['dormant_chef_version']` | String / nil | Version of the staged binary; `nil` if none installed |
| `chef_migration['migration_state']` | String | One of `omnibus_only`, `hab_dormant`, `hab_active` (cookbook-internal values — map to UI labels below) |

**`migration_state` values and UI display labels:**

| Raw value | UI label | Meaning |
|-----------|----------|---------|
| `omnibus_only` | **—** (not surfaced) | Only the active version installed; no staged version present. Retired from the UI: not offered as a deployment filter option and rendered as "—" in badges/labels/exports. Its absence can't be distinguished from a node the cookbook never reached (or aborted on before writing attributes), and it isn't a valid state for later hab→hab migrations. |
| `hab_dormant` | **Staged** | Target version installed but not active; the current version still runs converges (i.e. the node is still on the current/omnibus runtime — only `hab_active` has switched) |
| `hab_active` | **Activated** | Target version is now the active chef-client |

### Speculative Converge Results

Updated nightly by the staged version test run on each node.

| Attribute | Type | Description |
|-----------|------|-------------|
| `chef_migration['target_version']` | String / nil | CC version used for the speculative converge |
| `chef_migration['target_execution_time']` | String / nil | Timestamp of the speculative converge run |
| `chef_migration['target_converge_status']` | String / nil | Result: `success` or `fail` |

## Open Questions

*None — all resolved.*

### Resolved

- **Attribute tier** — confirmed `automatic['chef_migration']`. The migration cookbook writes to automatic attributes via ohai plugin, so partial-search path is `automatic.chef_migration.*`.

## Behaviour

### Node Readiness

A node is considered **ready to activate** when:
- `migration_state` is `hab_dormant` (target version is staged)
- `target_converge_status` is `success` (last speculative converge passed)

### Speculative Converge Status

`target_converge_status` is either `success` or `fail`. Only the latest result per node is stored — no rolling history.

## What Needs to Change

### Data Collection

- Add `chef_migration` attributes to the Chef Server partial-search fields fetched per node
- Path: `automatic.chef_migration.*` (ohai plugin writes to automatic attributes)

### Datastore

- Add columns to the `nodes` table for all 7 attributes
- Schema migration required

### Node List

- Add deployment state column using UI labels (Staged / Activated; other states render as "—")
- Add speculative converge result column (`success` / `fail` / not yet run)
- Mark nodes as **Ready to Activate** where state is Staged + last converge succeeded
- Filter by deployment state (**Staged** / **Activated** only) and speculative converge result

### Node Detail View

- Show deployment state (UI label), active version, staged version
- Show latest speculative converge: status, version tested, execution time
- Highlight **Ready to Activate** prominently when criteria met

### Dashboard Trend Graph

Two series over time:

1. **Staged or activated** — count of nodes with the target version present (Staged + Activated)
2. **Speculative converge passing** — count of nodes with `target_converge_status = success`

The gap between the two lines represents nodes where the target version is installed but the nightly test converge is still failing (cookbook or config issues to resolve).

## Related Specs

- `specifications/data-collection.md` — partial search fields and collection pipeline
- `specifications/visualisation.md` — dashboard layout and filter contracts
