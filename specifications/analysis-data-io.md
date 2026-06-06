# Analysis — Data Inputs and Outputs

## Data Inputs

| Input | Source |
|-------|--------|
| Node attribute data (cookbooks, platform, disk, run_list, roles, policy_name, policy_group, ohai_time) | Data collection component |
| Git repository HEAD commit SHA and test suite presence | Data collection component |
| Chef server cookbook file manifest | Data collection component |
| Full cookbook inventory from Chef server | Data collection component |
| Role dependency graph | Data collection component |
| Configured target Chef Client versions | Configuration |
| Configured disk space thresholds (`install_size_mb`, `min_remaining_free_percent`, install paths) | Configuration |
| Configured stale node threshold | Configuration |
| Cop-to-documentation mapping (embedded) | Application binary |
| CookStyle binary | Embedded Ruby environment (`/opt/chef-migration-metrics/embedded/bin/cookstyle`) |
| Test Kitchen binary | Embedded Ruby environment (`/opt/chef-migration-metrics/embedded/bin/kitchen`) |
| Ruby interpreter | Embedded Ruby environment (`/opt/chef-migration-metrics/embedded/bin/ruby`) |

## Data Outputs

| Output | Consumers |
|--------|-----------|
| Cookbook usage statistics (active/unused flag, node counts, platform counts, policy references) | Datastore → Dashboard |
| Test Kitchen results (converge + test pass/fail, keyed by cookbook + Chef Client version + commit SHA) | Datastore → Dashboard |
| CookStyle results (keyed by org + cookbook name + version) | Datastore → Dashboard |
| Auto-correct previews (diff, correctable/remaining counts) | Datastore → Dashboard |
| Remediation guidance (enriched offenses with migration docs, replacement patterns) | Datastore → Dashboard |
| Cookbook complexity scores and blast radius (per cookbook per target version) | Datastore → Dashboard |
| Node readiness status, blocking reasons, and stale data flags (per node per target version) | Datastore → Dashboard |
| Platform coverage reports (tested/untested/gap per cookbook per git repo) | Datastore → Dashboard |
