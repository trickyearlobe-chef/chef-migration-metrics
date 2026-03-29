import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { fetchSystemHealth } from "../api";
import type { SystemHealthResponse } from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import type { TableSize } from "../types";
import { useSort } from "../hooks/useSort";
import { SortableColumnHeader } from "../components/SortableColumnHeader";

// ---------------------------------------------------------------------------
// DB table sort types & helpers
// ---------------------------------------------------------------------------

type TableSortField =
  | "table_name"
  | "total_bytes"
  | "table_bytes"
  | "index_bytes"
  | "row_estimate";

function compareTableSizes(
  a: TableSize,
  b: TableSize,
  field: TableSortField,
): number {
  switch (field) {
    case "table_name":
      return a.table_name.localeCompare(b.table_name);
    case "total_bytes":
      return a.total_bytes - b.total_bytes;
    case "table_bytes":
      return a.table_bytes - b.table_bytes;
    case "index_bytes":
      return a.index_bytes - b.index_bytes;
    case "row_estimate":
      return a.row_estimate - b.row_estimate;
    default:
      return 0;
  }
}

// ---------------------------------------------------------------------------
// Admin System Stats Page
//
// Shows host-level resource metrics (disk, CPU, memory) and Go runtime
// stats for the machine running CMM. Auto-refreshes every 10 seconds.
// Displays alert banners when thresholds are breached, and a prominent
// notice when collection is paused due to critical alerts.
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return val.toFixed(i > 1 ? 1 : 0) + " " + units[i];
}

function percentColour(
  value: number,
  warning: number,
  critical: number,
): string {
  if (value >= critical) return "text-red-600";
  if (value >= warning) return "text-amber-600";
  return "text-green-600";
}

function barColour(value: number, warning: number, critical: number): string {
  if (value >= critical) return "bg-red-500";
  if (value >= warning) return "bg-amber-500";
  return "bg-green-500";
}

function loadColour(
  loadPerCPU: number,
  warning: number,
  critical: number,
): string {
  if (loadPerCPU >= critical) return "text-red-600";
  if (loadPerCPU >= warning) return "text-amber-600";
  return "text-green-600";
}

function UsageBar({
  percent,
  warning,
  critical,
  label,
}: {
  percent: number;
  warning: number;
  critical: number;
  label: string;
}) {
  const clamped = Math.min(Math.max(percent, 0), 100);
  return (
    <div>
      <div className="mb-1 flex items-center justify-between text-xs text-gray-500">
        <span>{label}</span>
        <span
          className={"font-medium " + percentColour(percent, warning, critical)}
        >
          {percent.toFixed(1)}%
        </span>
      </div>
      <div className="h-3 w-full overflow-hidden rounded-full bg-gray-100">
        <div
          className={
            "h-full rounded-full transition-all duration-700 " +
            barColour(percent, warning, critical)
          }
          style={{ width: clamped + "%" }}
        />
      </div>
    </div>
  );
}

export function AdminSystemStatsPage() {
  const [data, setData] = useState<SystemHealthResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const {
    sortField: tableSortField,
    sortOrder: tableSortOrder,
    handleSort: handleTableSort,
  } = useSort<TableSortField>({
    defaultField: "total_bytes",
    defaultOrder: "desc",
    descendingFields: [
      "total_bytes",
      "table_bytes",
      "index_bytes",
      "row_estimate",
    ],
  });

  const load = useCallback(() => {
    fetchSystemHealth()
      .then((res) => {
        setData(res);
        setError(null);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
    intervalRef.current = setInterval(load, 10_000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [load]);

  const th = data?.thresholds;

  const sortedTableSizes = useMemo(() => {
    if (!data) return [];
    const sorted = [...data.table_sizes].sort((a, b) => {
      const cmp = compareTableSizes(a, b, tableSortField);
      return tableSortOrder === "asc" ? cmp : -cmp;
    });
    return sorted;
  }, [data, tableSortField, tableSortOrder]);

  // Find the max total_bytes across all tables for bar chart scaling
  const maxTableBytes = useMemo(() => {
    if (sortedTableSizes.length === 0) return 1;
    return Math.max(...sortedTableSizes.map((t) => t.total_bytes), 1);
  }, [sortedTableSizes]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-gray-800">System Stats</h2>
        {data && (
          <span className="text-xs text-gray-400">
            Uptime: {data.uptime} &middot; Auto-refreshes every 10s
          </span>
        )}
      </div>

      {loading && !data && <LoadingSpinner message="Loading system health…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}

      {data && th && (
        <>
          {/* ---- Collection paused banner ---- */}
          {data.collection_paused && (
            <div className="flex items-start gap-3 rounded-lg border border-red-300 bg-red-50 p-4">
              <svg
                className="mt-0.5 h-5 w-5 shrink-0 text-red-600"
                fill="none"
                viewBox="0 0 24 24"
                strokeWidth={1.5}
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M15.75 5.25v13.5m-7.5-13.5v13.5"
                />
              </svg>
              <div>
                <p className="text-sm font-semibold text-red-800">
                  Data collection paused
                </p>
                <p className="mt-1 text-xs text-red-700">
                  Collection has been automatically paused due to critical
                  resource alerts. It will resume once resources return below
                  critical thresholds.
                </p>
              </div>
            </div>
          )}

          {/* ---- Alert banners ---- */}
          {data.alerts.length > 0 && (
            <div className="space-y-2">
              {data.alerts.map((a, i) => (
                <div
                  key={i}
                  className={
                    "flex items-center gap-2 rounded-lg border p-3 text-sm " +
                    (a.level === "critical"
                      ? "border-red-300 bg-red-50 text-red-800"
                      : "border-amber-300 bg-amber-50 text-amber-800")
                  }
                >
                  <svg
                    className="h-4 w-4 shrink-0"
                    fill="none"
                    viewBox="0 0 24 24"
                    strokeWidth={1.5}
                    stroke="currentColor"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M12 9v3.75m9-.75a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 3.75h.008v.008H12v-.008Z"
                    />
                  </svg>
                  <span className="font-medium capitalize">{a.level}:</span>
                  {a.message}
                </div>
              ))}
            </div>
          )}

          {/* ---- Disk cards (one per unique filesystem) ---- */}
          {data.disks.length > 0 && (
            <div
              className={
                "grid gap-6 " +
                (data.disks.length === 1
                  ? "lg:grid-cols-1 max-w-md"
                  : data.disks.length === 2
                    ? "lg:grid-cols-2"
                    : "lg:grid-cols-3")
              }
            >
              {data.disks.map((disk, i) => (
                <div key={disk.path} className="card">
                  <h3 className="card-header">
                    Disk Usage{data.disks.length > 1 ? ` (${i + 1})` : ""}
                  </h3>
                  <UsageBar
                    percent={disk.used_percent}
                    warning={th.disk_used_warning_percent}
                    critical={th.disk_used_critical_percent}
                    label="Used"
                  />
                  <p className="mt-3 text-sm text-gray-600">
                    {formatBytes(disk.free_bytes)} free of{" "}
                    {formatBytes(disk.total_bytes)}
                  </p>
                  <p className="mt-1 text-xs text-gray-400">
                    Path: {disk.path}
                  </p>
                </div>
              ))}
            </div>
          )}

          {/* ---- Metric cards ---- */}
          <div className="grid gap-6 lg:grid-cols-2">
            {/* CPU Load */}
            <div className="card">
              <h3 className="card-header">CPU Load</h3>
              <div className="flex items-baseline gap-2">
                <span
                  className={
                    "text-3xl font-bold " +
                    loadColour(
                      data.load_per_cpu,
                      th.cpu_load_warning_per_cpu,
                      th.cpu_load_critical_per_cpu,
                    )
                  }
                >
                  {data.load_avg_1.toFixed(2)}
                </span>
                <span className="text-sm text-gray-500">1-min avg</span>
              </div>
              <p className="mt-3 text-sm text-gray-600">
                {data.load_per_cpu.toFixed(2)} per CPU
              </p>
              <p className="mt-1 text-xs text-gray-400">
                {data.cpu_count} CPUs
              </p>
            </div>

            {/* Memory */}
            <div className="card">
              <h3 className="card-header">Memory Usage</h3>
              <UsageBar
                percent={data.mem_used_percent}
                warning={th.mem_used_warning_percent}
                critical={th.mem_used_critical_percent}
                label="Used"
              />
              <p className="mt-3 text-sm text-gray-600">
                {formatBytes(data.mem_avail_bytes)} available of{" "}
                {formatBytes(data.mem_total_bytes)}
              </p>
            </div>
          </div>

          {/* ---- Database & runtime ---- */}
          <div className="grid gap-6 lg:grid-cols-4">
            <div className="card">
              <h3 className="card-header text-sm">Database Size</h3>
              <span className="text-2xl font-bold text-gray-700">
                {data.database_size_bytes > 0
                  ? formatBytes(data.database_size_bytes)
                  : "N/A"}
              </span>
              {data.database_size_bytes > 0 && (
                <p className="mt-1 text-xs text-gray-400">
                  PostgreSQL &middot; {data.table_sizes.length} table
                  {data.table_sizes.length !== 1 ? "s" : ""}
                </p>
              )}
            </div>
            <div className="card">
              <h3 className="card-header text-sm">Go Heap</h3>
              <span className="text-2xl font-bold text-gray-700">
                {formatBytes(data.go_heap_bytes)}
              </span>
            </div>
            <div className="card">
              <h3 className="card-header text-sm">Goroutines</h3>
              <span className="text-2xl font-bold text-gray-700">
                {data.go_goroutines.toLocaleString()}
              </span>
            </div>
            <div className="card">
              <h3 className="card-header text-sm">Uptime</h3>
              <span className="text-2xl font-bold text-gray-700">
                {data.uptime}
              </span>
            </div>
          </div>

          {/* ---- Database table sizes ---- */}
          {data.table_sizes.length > 0 && (
            <details className="card">
              <summary className="cursor-pointer text-sm font-medium text-gray-600 hover:text-gray-800">
                Database Tables ({data.table_sizes.length})
              </summary>
              <div className="mt-4 overflow-x-auto">
                <table className="w-full text-left text-sm">
                  <thead>
                    <tr className="border-b border-gray-200 text-xs text-gray-500">
                      <SortableColumnHeader
                        label="Table"
                        field="table_name"
                        currentField={tableSortField}
                        currentOrder={tableSortOrder}
                        onSort={handleTableSort}
                        className="pb-2 pr-4 font-medium"
                      />
                      <SortableColumnHeader
                        label="Total"
                        field="total_bytes"
                        currentField={tableSortField}
                        currentOrder={tableSortOrder}
                        onSort={handleTableSort}
                        className="pb-2 pr-4 font-medium text-right"
                      />
                      <SortableColumnHeader
                        label="Data"
                        field="table_bytes"
                        currentField={tableSortField}
                        currentOrder={tableSortOrder}
                        onSort={handleTableSort}
                        className="pb-2 pr-4 font-medium text-right"
                      />
                      <SortableColumnHeader
                        label="Indexes"
                        field="index_bytes"
                        currentField={tableSortField}
                        currentOrder={tableSortOrder}
                        onSort={handleTableSort}
                        className="pb-2 pr-4 font-medium text-right"
                      />
                      <SortableColumnHeader
                        label="Rows (est.)"
                        field="row_estimate"
                        currentField={tableSortField}
                        currentOrder={tableSortOrder}
                        onSort={handleTableSort}
                        className="pb-2 pr-4 font-medium text-right"
                      />
                      <th
                        className="pb-2 font-medium"
                        style={{ minWidth: "120px" }}
                      ></th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedTableSizes.map((t) => {
                      const pct = Math.max(
                        (t.total_bytes / maxTableBytes) * 100,
                        0.5,
                      );
                      return (
                        <tr
                          key={t.table_name}
                          className="border-b border-gray-100 last:border-0"
                        >
                          <td className="py-2 pr-4 font-mono text-xs text-gray-700">
                            {t.table_name}
                          </td>
                          <td className="py-2 pr-4 text-right text-xs text-gray-700">
                            {formatBytes(t.total_bytes)}
                          </td>
                          <td className="py-2 pr-4 text-right text-xs text-gray-500">
                            {formatBytes(t.table_bytes)}
                          </td>
                          <td className="py-2 pr-4 text-right text-xs text-gray-500">
                            {formatBytes(t.index_bytes)}
                          </td>
                          <td className="py-2 pr-4 text-right text-xs text-gray-500">
                            {t.row_estimate.toLocaleString()}
                          </td>
                          <td className="py-2">
                            <div className="h-2 w-full overflow-hidden rounded-full bg-gray-100">
                              <div
                                className="h-full rounded-full bg-blue-400 transition-all duration-700"
                                style={{ width: pct + "%" }}
                              />
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </details>
          )}

          {/* ---- Thresholds ---- */}
          <details className="card">
            <summary className="cursor-pointer text-sm font-medium text-gray-600 hover:text-gray-800">
              Configured Thresholds
            </summary>
            <div className="mt-4 grid gap-4 text-xs text-gray-500 sm:grid-cols-3">
              <div>
                <p className="font-medium text-gray-700">Disk</p>
                <p>Warning: {th.disk_used_warning_percent}%</p>
                <p>Critical: {th.disk_used_critical_percent}%</p>
              </div>
              <div>
                <p className="font-medium text-gray-700">CPU (per CPU)</p>
                <p>Warning: {th.cpu_load_warning_per_cpu}</p>
                <p>Critical: {th.cpu_load_critical_per_cpu}</p>
              </div>
              <div>
                <p className="font-medium text-gray-700">Memory</p>
                <p>Warning: {th.mem_used_warning_percent}%</p>
                <p>Critical: {th.mem_used_critical_percent}%</p>
              </div>
            </div>
          </details>
        </>
      )}
    </div>
  );
}
