# Test Kitchen Drivers — Database Changes

## Database Changes

### Modified: `git_repo_test_kitchen_results`

New column:

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `driver` | TEXT | Yes | `NULL` | Driver used for the test run. NULL for pre-existing rows (implies `dokken`). |
| `platform_name` | TEXT | Yes | `NULL` | Kitchen platform name for this result (enables per-platform result tracking). |

The existing unique constraint `(git_repo_id, target_chef_version, commit_sha)` is unchanged. A cookbook is tested with whichever driver is currently configured. When the driver changes, the next HEAD change triggers a retest.

### New Table: `cookbook_platform_coverage`

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `git_repo_id` | UUID | Yes | FK → `git_repos.id` (NULL for server-only cookbooks) |
| `cookbook_name` | TEXT | No | Cookbook name |
| `coverage_data` | JSONB | No | Coverage report (structure below) |
| `evaluated_at` | TIMESTAMPTZ | No | When coverage was last evaluated |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Foreign keys:** `git_repo_id` → `git_repos(id)` ON DELETE CASCADE

**Unique constraints:** `(cookbook_name)`

**Indexes:** `idx_cookbook_platform_coverage_cookbook_name` on `cookbook_name`

### Coverage Data JSONB Structure

```
{
  "kitchen_platforms": ["ubuntu-22.04", "centos-7"],
  "production_platforms": [
    {"platform": "ubuntu", "platform_version": "22.04", "platform_family": "debian", "node_count": 47},
    {"platform": "centos", "platform_version": "7.9.2009", "platform_family": "rhel", "node_count": 12},
    {"platform": "rocky", "platform_version": "9.3", "platform_family": "rhel", "node_count": 8}
  ],
  "tested_and_in_production": [
    {"kitchen_name": "ubuntu-22.04", "platform": "ubuntu", "platform_version": "22.04", "node_count": 47},
    {"kitchen_name": "centos-7", "platform": "centos", "platform_version": "7.9.2009", "node_count": 12}
  ],
  "tested_not_in_production": [],
  "in_production_not_tested": [
    {"platform": "rocky", "platform_version": "9.3", "platform_family": "rhel", "node_count": 8}
  ],
  "gap_count": 1,
  "total_production_nodes": 67,
  "covered_node_count": 59,
  "coverage_percentage": 88.1
}
```
