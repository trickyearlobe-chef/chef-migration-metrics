# System Health Monitoring

## Overview

The CMM application monitors the health of the host machine it runs on,
exposing CPU load, memory usage, and disk space metrics via a dedicated
admin API endpoint. The frontend renders these on an admin-only "System
Stats" page with visual alerts when thresholds are breached. The
collection scheduler uses the same checks as a circuit breaker — pausing
data collection when the host is under resource pressure.

## Goals

- Give administrators early warning when disk space or CPU load is
  dangerously high on the CMM host.
- Automatically pause collection runs when resources are critically low
  to prevent the application from filling the disk or starving other
  processes.
- Avoid external dependencies — use Go stdlib (`os`, `runtime`,
  `syscall`) for all metrics. No `gopsutil` or cgo.

## System Metrics Collected

| Metric              | Source                         | Unit       |
|---------------------|--------------------------------|------------|
| Disk total (per fs) | `syscall.Statfs` on each path  | bytes      |
| Disk free (per fs)  | `syscall.Statfs` on each path  | bytes      |
| Disk used % (per fs)| computed                       | float64 %  |
| CPU count           | `runtime.NumCPU()`             | int        |
| 1-min load average  | `/proc/loadavg` (Linux) or `syscall.Sysctl` (macOS) | float64 |
| Load per CPU        | load average / CPU count       | float64    |
| Memory total        | `/proc/meminfo` or `syscall`   | bytes      |
| Memory available    | `/proc/meminfo` or `syscall`   | bytes      |
| Memory used percent | computed                       | float64 %  |
| Database size       | `pg_database_size(current_database())` | bytes |
| Go heap in-use      | `runtime.ReadMemStats`         | bytes      |
| Go goroutine count  | `runtime.NumGoroutine()`       | int        |
| Uptime              | `time.Since(startTime)`        | duration   |

On unsupported platforms, load average and OS memory gracefully return
zero values with an `unsupported` flag rather than failing.

## Package Layout

All host-level metrics live in `internal/syshealth/`. The package is
self-contained and has no dependency on `internal/config/` or
`internal/datastore/`.

| File                | Purpose                                           |
|---------------------|---------------------------------------------------|
| `syshealth.go`      | `Snapshot()`, `collectDisks()`, types, alert logic |
| `disk_darwin.go`    | `diskUsageWithDevice()` — macOS via `Statfs` + Fsid |
| `disk_linux.go`     | `diskUsageWithDevice()` — Linux via `Statfs` + `Stat_t.Dev` |
| `disk_windows.go`   | `diskUsageWithDevice()` — stub (returns error)    |
| `cpu_darwin.go`     | Load average via `sysctl vm.loadavg`              |
| `cpu_linux.go`      | Load average via `/proc/loadavg`                  |
| `cpu_windows.go`    | Load average stub                                 |
| `memory_darwin.go`  | Memory via `sysctl` + `vm_stat`                   |
| `memory_linux.go`   | Memory via `/proc/meminfo`                         |
| `memory_windows.go` | Memory stub                                       |
| `syshealth_test.go` | Tests for `Snapshot()`, alerts, disk de-dup        |

### `DiskStats` struct

```go
type DiskStats struct {
    Path        string  `json:"path"`
    Device      uint64  `json:"-"`          // device ID for de-duplication (not serialised)
    TotalBytes  uint64  `json:"total_bytes"`
    FreeBytes   uint64  `json:"free_bytes"`
    UsedPercent float64 `json:"used_percent"`
}
```

### `Stats` struct

```go
type Stats struct {
    Timestamp    time.Time   `json:"timestamp"`
    Uptime       string      `json:"uptime"`

    // Disk — one entry per unique filesystem detected from the
    // configured paths. Multiple paths on the same device are
    // de-duplicated; the first path encountered is kept as the label.
    Disks        []DiskStats `json:"disks"`

    // CPU / Load
    CPUCount     int         `json:"cpu_count"`
    LoadAvg1     float64     `json:"load_avg_1"`
    LoadPerCPU   float64     `json:"load_per_cpu"`

    // OS Memory
    MemTotalBytes  uint64  `json:"mem_total_bytes"`
    MemAvailBytes  uint64  `json:"mem_avail_bytes"`
    MemUsedPercent float64 `json:"mem_used_percent"`

    // Go runtime
    GoHeapBytes    uint64  `json:"go_heap_bytes"`
    GoGoroutines   int     `json:"go_goroutines"`

    // Alerts
    Alerts       []Alert   `json:"alerts"`
}

type Alert struct {
    Level   string `json:"level"`   // "warning" or "critical"
    Metric  string `json:"metric"`  // "disk", "cpu", "memory"
    Message string `json:"message"`
}
```

### `Snapshot` function

```go
func Snapshot(diskPaths []string, thresholds Thresholds) Stats
```

The function is stateless and safe to call concurrently. It probes each
path in `diskPaths` for disk usage, de-duplicates by underlying device
ID (so two paths on the same filesystem produce only one `DiskStats`
entry), collects CPU/memory/runtime metrics, evaluates them against the
provided thresholds, and populates the `Alerts` slice. If `diskPaths` is
nil or empty, `["/"]` is used as a fallback.

### `diskUsageWithDevice` function (platform-specific)

```go
func diskUsageWithDevice(path string) (total, free, deviceID uint64, err error)
```

Returns total/free bytes **and** a device identifier used for
de-duplication:

| Platform | Device ID source |
|----------|------------------|
| macOS    | `Statfs_t.Fsid` (two int32 values packed into uint64) |
| Linux    | `Stat_t.Dev` from `syscall.Stat()` |
| Windows  | stub — returns error |

### `collectDisks` function

```go
func collectDisks(paths []string) []DiskStats
```

Iterates paths, calls `diskUsageWithDevice`, skips empty paths and
errors, de-duplicates by device ID (first path wins), and returns the
slice in first-seen order.

### `Thresholds` struct

```go
type Thresholds struct {
    DiskUsedWarningPercent  float64 // default 80
    DiskUsedCriticalPercent float64 // default 90
    CPULoadWarningPerCPU    float64 // default 2.0
    CPULoadCriticalPerCPU   float64 // default 4.0
    MemUsedWarningPercent   float64 // default 80
    MemUsedCriticalPercent  float64 // default 90
}
```

### `ShouldPauseCollection` function

```go
func ShouldPauseCollection(s Stats) bool
```

Returns true if **any** alert has level `"critical"`. The scheduler
checks this before starting a collection run.

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

## API Endpoint

### `GET /api/v1/admin/system-health`

Admin-only endpoint. Returns the current system health snapshot.

**Response (200):**

```json
{
  "timestamp": "2025-01-15T10:30:00Z",
  "uptime": "3d 14h 22m",

  "disks": [
    {
      "path": "/data",
      "total_bytes": 107374182400,
      "free_bytes": 21474836480,
      "used_percent": 80.0
    },
    {
      "path": "/var/lib/cmm/exports",
      "total_bytes": 53687091200,
      "free_bytes": 32212254720,
      "used_percent": 40.0
    }
  ],

  "cpu_count": 4,
  "load_avg_1": 3.2,
  "load_per_cpu": 0.8,

  "mem_total_bytes": 8589934592,
  "mem_avail_bytes": 2147483648,
  "mem_used_percent": 75.0,

  "go_heap_bytes": 52428800,
  "go_goroutines": 42,

  "database_size_bytes": 536870912,

  "alerts": [
    {
      "level": "warning",
      "metric": "disk",
      "message": "Disk usage at 80.0% on /data (20.0 GB free of 100.0 GB)"
    }
  ],

  "collection_paused": false,
  "thresholds": {
    "disk_used_warning_percent": 80,
    "disk_used_critical_percent": 90,
    "cpu_load_warning_per_cpu": 2.0,
    "cpu_load_critical_per_cpu": 4.0,
    "mem_used_warning_percent": 80,
    "mem_used_critical_percent": 90
  }
}
```

The handler calls `syshealth.Snapshot()`, adds `collection_paused` and
`thresholds` from config, and returns the combined response. The `disks`
and `alerts` arrays are guaranteed to be `[]` (never `null`) in JSON.

### Database size

The handler also queries `pg_database_size(current_database())` via the
`DataStore.DatabaseSize()` method and includes the result as
`database_size_bytes`. This works for both local and remote PostgreSQL
instances since it queries through the existing database connection.

The query is best-effort: if the database is unreachable or the query
fails, `database_size_bytes` falls back to `0` and the endpoint still
returns 200. If no `DataStore` is configured (nil), the field is `0`.

## Collection Circuit Breaker

The scheduler loop in `internal/collector/scheduler.go` checks system
health before each run:

```
timer fires →
  if collector.IsRunning() → skip (existing behaviour)
  else if config.PauseCollectionOnCritical && ShouldPauseCollection(snapshot) →
      log WARN "collection paused: <alert messages>"
      skip
  else → run collection
```

The check is lightweight (one `Statfs` call per unique disk path + one
`/proc/loadavg` read) and adds negligible overhead to the scheduling
loop.

The circuit breaker does **not** interrupt a run that is already in
progress. It only prevents new runs from starting.

## Frontend — Admin System Stats Page

### Route

`/admin/system-stats` — protected by `RequireAdmin`.

### Navigation

Added to `adminNavItems` in `AppLayout.tsx` with a server/heartbeat icon.

### Page Layout

The page auto-refreshes every 10 seconds via `setInterval`.

**Alert banner** — shown at the top when alerts are present:
- Warning alerts: amber background, warning icon
- Critical alerts: red background, exclamation icon
- Each alert displays the message text
- If collection is paused, a prominent notice says so

**Metric cards** — dynamic grid layout:

1. **Disk Usage** — one card per unique filesystem
   - Horizontal usage bar
   - "X.X GB free of Y.Y GB" subtitle
   - Path shown below
   - Colour shifts from green → amber → red based on thresholds
   - Grid adapts: 1 column for single disk, 2 columns for two, 3 for
     three or more

2. **CPU Load** and **Memory** — two cards side by side

**Database & runtime section** — four cards in a row:
- Database size (from `pg_database_size`, shows "N/A" when unavailable)
- Go heap usage
- Goroutine count
- Uptime

3. **CPU Load**
   - Load average (1 min) displayed prominently
   - "X.XX per CPU" subtitle
   - CPU count shown below
   - Colour based on load per CPU thresholds

3. **Memory**
   - Horizontal bar or circular progress
   - "X.X GB available of Y.Y GB" subtitle
   - Colour shifts based on thresholds



**Thresholds section** — collapsible panel showing the configured
warning/critical thresholds for each metric.

### TypeScript Types

```typescript
interface SystemHealthAlert {
  level: "warning" | "critical";
  metric: "disk" | "cpu" | "memory";
  message: string;
}

interface DiskStats {
  path: string;
  total_bytes: number;
  free_bytes: number;
  used_percent: number;
}

interface SystemHealthResponse {
  timestamp: string;
  uptime: string;

  disks: DiskStats[];

  cpu_count: number;
  load_avg_1: number;
  load_per_cpu: number;

  mem_total_bytes: number;
  mem_avail_bytes: number;
  mem_used_percent: number;

  go_heap_bytes: number;
  go_goroutines: number;

  database_size_bytes: number;

  alerts: SystemHealthAlert[];
  collection_paused: boolean;

  thresholds: {
    disk_used_warning_percent: number;
    disk_used_critical_percent: number;
    cpu_load_warning_per_cpu: number;
    cpu_load_critical_per_cpu: number;
    mem_used_warning_percent: number;
    mem_used_critical_percent: number;
  };
}
```

### API Function

```typescript
function fetchSystemHealth(): Promise<SystemHealthResponse>
```

Calls `GET /api/v1/admin/system-health`.

## Testing

### Backend

- `internal/syshealth/syshealth_test.go`:
  - `TestSnapshot_ReturnsValidStats` — verifies all fields are populated
    with sane values (disk total > 0, CPU count > 0, etc.)
  - `TestSnapshot_DiskAlerts` — injects low thresholds and verifies
    warning/critical alerts are generated
  - `TestSnapshot_CPUAlerts` — same for CPU load
  - `TestSnapshot_MemoryAlerts` — same for memory
  - `TestSnapshot_NoAlerts` — high thresholds, verifies empty alerts
  - `TestShouldPauseCollection_Critical` — returns true when critical
    alerts present
  - `TestShouldPauseCollection_WarningOnly` — returns false
  - `TestShouldPauseCollection_NoAlerts` — returns false

- `internal/webapi/handle_dashboard_test.go` or dedicated test file:
  - Method-not-allowed tests for the admin endpoint
  - Happy path returning valid JSON with expected fields

- `internal/collector/scheduler_test.go`:
  - Test that collection is skipped when `ShouldPauseCollection` returns
    true (inject a mock or use test thresholds)

### Frontend

No dedicated frontend tests are required at this stage. The page follows
the same pattern as other admin pages and is verified manually.

## Rollout Considerations

- No database migration required — all metrics are read at request time.
- The `system_health` config section is entirely optional. If omitted,
  defaults apply and the feature works out of the box.
- The admin page is only visible to admin users. Non-admin users cannot
  access the endpoint or the page.
- On unsupported platforms (e.g. Windows), load average and OS memory
  metrics return zero with no alerts generated for those metrics.