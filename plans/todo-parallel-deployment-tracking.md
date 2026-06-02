# Parallel Deployment Tracking — ToDo

Spec: `specifications/parallel-deployment-tracking.md`

### Open Questions

*None — all resolved.*

### Implementation

- [ ] Add `chef_migration` automatic attributes to partial-search fields in data collection (path: `automatic.chef_migration.*`)
- [ ] Schema migration: add migration tracking columns to `nodes` table
- [ ] Persist migration state and speculative converge result per node during collection
- [ ] Node list: add `migration_state` badge column (`omnibus_only` / `hab_dormant` / `hab_active`)
- [ ] Node list: add speculative converge result column (success / fail / not yet run)
- [ ] Node list: highlight "Ready to Activate" nodes (`hab_dormant` + last status `success`)
- [ ] Node list: filter by `migration_state` and `target_converge_status`
- [ ] Node detail: show migration state, active/dormant versions, latest speculative converge result
- [ ] Dashboard: trend graph — "dormant installed" vs "speculative converge passing" over time
