# System Health — Package Layout

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
