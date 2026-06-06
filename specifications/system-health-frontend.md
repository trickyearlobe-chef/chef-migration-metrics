# System Health — Frontend — Admin System Stats Page

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
- Database size (from `pg_database_size`, shows "N/A" when unavailable;
  subtitle shows table count)
- Go heap usage
- Goroutine count
- Uptime

**Database tables panel** — collapsible `<details>` panel showing a
sorted table of all user tables with columns: Table, Total, Data,
Indexes, Rows (est.), and a relative size bar. Tables are ordered by
total size descending. The bar width is proportional to the largest
table. Only shown when `table_sizes` is non-empty.

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

interface TableSize {
  table_name: string;
  total_bytes: number;
  table_bytes: number;
  index_bytes: number;
  row_estimate: number;
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
  table_sizes: TableSize[];

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
