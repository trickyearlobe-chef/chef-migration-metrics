# Habitat Migration Tracking — ToDo

Spec: `specifications/habitat-migration-tracking.md`

### Open Questions (resolve before implementation)

- [ ] Confirm ohai attribute tier: `normal['chef_migration']` or `automatic['chef_migration']`?

### Implementation (blocked on open questions)

- [ ] Add `chef_migration` attributes to partial-search fields in data collection
- [ ] Schema migration: add migration tracking columns to `nodes` table
- [ ] Persist migration state and speculative converge result per node during collection
- [ ] Dashboard: fleet-wide `migration_state` breakdown (counts + trend)
- [ ] Node list: add `migration_state` and speculative converge result columns
- [ ] Node list: filter by `migration_state` and `target_converge_status`
- [ ] Dashboard: headline metric (% hab_dormant nodes with successful speculative converge)
