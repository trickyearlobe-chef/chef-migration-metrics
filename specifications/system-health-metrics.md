# System Health — System Metrics Collected

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
