# System Health — Configuration

## Configuration

New keys under the top-level `system_health` section in config YAML:

```yaml
system_health:
  # Paths whose mount points are checked for disk space.
  # Multiple paths on the same underlying filesystem are de-duplicated.
  # Default: [storage.data_dir, storage.cookbook_cache_dir,
  #           storage.git_cookbook_dir, exports.output_directory]
  disk_paths:
    - "/data"
    - "/var/lib/cmm/exports"
  disk_used_warning_percent: 80          # default 80
  disk_used_critical_percent: 90         # default 90
  cpu_load_warning_per_cpu: 2.0          # default 2.0
  cpu_load_critical_per_cpu: 4.0         # default 4.0
  mem_used_warning_percent: 80           # default 80
  mem_used_critical_percent: 90          # default 90
  pause_collection_on_critical: true     # default true
```

Config struct in `internal/config/`:

```go
type SystemHealthConfig struct {
    DiskPaths                  []string `yaml:"disk_paths"`
    DiskUsedWarningPercent     float64  `yaml:"disk_used_warning_percent"`
    DiskUsedCriticalPercent    float64  `yaml:"disk_used_critical_percent"`
    CPULoadWarningPerCPU       float64  `yaml:"cpu_load_warning_per_cpu"`
    CPULoadCriticalPerCPU      float64  `yaml:"cpu_load_critical_per_cpu"`
    MemUsedWarningPercent      float64  `yaml:"mem_used_warning_percent"`
    MemUsedCriticalPercent     float64  `yaml:"mem_used_critical_percent"`
    PauseCollectionOnCritical  *bool    `yaml:"pause_collection_on_critical"`
}
```

Defaults are applied via `setDefaults()`. When `DiskPaths` is empty, it
defaults to `[data_dir, cookbook_cache_dir, git_cookbook_dir,
exports_output_dir]` — covering all directories that CMM writes to.
Paths that resolve to the same filesystem are automatically
de-duplicated at probe time. `PauseCollectionOnCritical` defaults to
`true` (nil-pointer-means-true pattern).

Users who know their local Postgres data directory can add it to
`disk_paths` manually. Auto-detecting Postgres storage paths is fragile
(Docker volumes, symlinks, separate WAL mounts, tablespaces) and is out
of scope.

Environment variable overrides follow the `CMM_` prefix convention:

| Variable                                  | Config key                        |
|-------------------------------------------|-----------------------------------|
| `CMM_SYSTEM_HEALTH_DISK_PATHS`            | `system_health.disk_paths`        |
| `CMM_SYSTEM_HEALTH_DISK_USED_WARNING`     | `disk_used_warning_percent`       |
| `CMM_SYSTEM_HEALTH_DISK_USED_CRITICAL`    | `disk_used_critical_percent`      |
| `CMM_SYSTEM_HEALTH_CPU_LOAD_WARNING`      | `cpu_load_warning_per_cpu`        |
| `CMM_SYSTEM_HEALTH_CPU_LOAD_CRITICAL`     | `cpu_load_critical_per_cpu`       |
| `CMM_SYSTEM_HEALTH_MEM_USED_WARNING`      | `mem_used_warning_percent`        |
| `CMM_SYSTEM_HEALTH_MEM_USED_CRITICAL`     | `mem_used_critical_percent`       |
| `CMM_SYSTEM_HEALTH_PAUSE_COLLECTION`      | `pause_collection_on_critical`    |
