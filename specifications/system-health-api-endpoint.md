# System Health — API Endpoint

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

  "table_sizes": [
    {
      "table_name": "node_snapshots",
      "total_bytes": 52428800,
      "table_bytes": 41943040,
      "index_bytes": 10485760,
      "row_estimate": 15000
    },
    {
      "table_name": "server_cookbooks",
      "total_bytes": 20971520,
      "table_bytes": 16777216,
      "index_bytes": 4194304,
      "row_estimate": 800
    },
    {
      "table_name": "schema_migrations",
      "total_bytes": 16384,
      "table_bytes": 8192,
      "index_bytes": 8192,
      "row_estimate": 12
    }
  ],

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
`thresholds` from config, and returns the combined response. The `disks`,
`alerts`, and `table_sizes` arrays are guaranteed to be `[]` (never
`null`) in JSON.

### Database size and per-table breakdown

The handler queries two pieces of database storage information:

1. **`database_size_bytes`** — total database size via
   `pg_database_size(current_database())`, returned as a single integer.
2. **`table_sizes`** — per-table disk usage for all user tables in the
   `public` schema, ordered by total size descending. Each entry
   includes:

   | Field          | Source                              | Description                        |
   |----------------|-------------------------------------|------------------------------------|
   | `table_name`   | `pg_class.relname`                  | Table name                         |
   | `total_bytes`  | `pg_total_relation_size(oid)`       | Table + indexes + TOAST            |
   | `table_bytes`  | `pg_table_size(oid)`                | Table + TOAST (no indexes)         |
   | `index_bytes`  | `pg_indexes_size(oid)`              | All indexes on the table           |
   | `row_estimate` | `pg_stat_user_tables.n_live_tup`    | Estimated live row count           |

Both queries use the `DataStore` interface:

```go
DatabaseSize(ctx context.Context) (int64, error)
DatabaseTableSizes(ctx context.Context) ([]TableSize, error)
```

```go
type TableSize struct {
    TableName   string `json:"table_name"`
    TotalBytes  int64  `json:"total_bytes"`
    TableBytes  int64  `json:"table_bytes"`
    IndexBytes  int64  `json:"index_bytes"`
    RowEstimate int64  `json:"row_estimate"`
}
```

Both queries are best-effort: if the database is unreachable or a query
fails, `database_size_bytes` falls back to `0` and `table_sizes` falls
back to `[]`. The endpoint still returns 200. If no `DataStore` is
configured (nil), both fields return their zero values.

This works for both local and remote PostgreSQL instances since it
queries through the existing database connection — no filesystem access
is required.
