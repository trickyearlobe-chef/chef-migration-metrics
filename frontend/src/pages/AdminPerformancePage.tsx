import { useState, useEffect, useCallback, useRef } from "react";
import {
  fetchPerformanceStats,
  fetchPerformanceDB,
  resetPerformanceStats,
  resetPerformanceDB,
  vacuumFull,
} from "../api";
import type {
  PerformanceResponse,
  PerformanceDBResponse,
  EndpointStat,
  TopQueryStat,
  PgTableStat,
  PgIndexStat,
  PgActiveQuery,
} from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return val.toFixed(i > 1 ? 1 : 0) + " " + units[i];
}

function formatMs(ms: number): string {
  return ms.toFixed(1);
}

function shortDate(iso: string | null): string {
  if (!iso) return "Never";
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) + "…" : s;
}

function p95Colour(p95: number): string {
  if (p95 > 500) return "bg-red-50";
  if (p95 >= 100) return "bg-yellow-50";
  return "bg-green-50";
}

function queryRowColour(maxMs: number): string {
  if (maxMs > 1000) return "bg-red-50";
  if (maxMs >= 100) return "bg-yellow-50";
  return "bg-green-50";
}

function durationRowColour(ms: number): string {
  if (ms > 5000) return "bg-red-50";
  if (ms > 1000) return "bg-yellow-50";
  return "";
}

function cacheHitRatio(hit: number, read: number): string {
  if (hit === 0 && read === 0) return "N/A";
  return ((hit / (hit + read)) * 100).toFixed(1) + "%";
}

// ---------------------------------------------------------------------------
// Shared table styling
// ---------------------------------------------------------------------------

const cardCls = "bg-white rounded-lg shadow-sm border border-gray-200 p-5";
const thCls = "px-4 py-2 text-xs font-medium text-gray-500 uppercase tracking-wider bg-gray-50";
const tdCls = "px-4 py-2 text-sm";
const tableCls = "w-full text-sm text-left";
const hdrCls = "text-lg font-semibold text-gray-800";
const btnCls = "rounded-md bg-gray-100 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-200 transition-colors";
const btnDangerCls = "rounded-md bg-red-50 px-3 py-1.5 text-xs font-medium text-red-700 hover:bg-red-100 transition-colors";

function NoData() {
  return <p className="py-4 text-center text-sm text-gray-400">No data</p>;
}

// ---------------------------------------------------------------------------
// Section components
// ---------------------------------------------------------------------------

function EndpointsSection({ data }: { data: PerformanceResponse }) {
  const handleReset = () => {
    if (window.confirm("Reset API performance stats? This clears all collected endpoint metrics.")) {
      resetPerformanceStats().catch(() => {});
    }
  };

  return (
    <section className={cardCls}>
      <div className="mb-3 flex items-center justify-between">
        <h2 className={hdrCls}>
          API Endpoints{" "}
          <span className="text-xs font-normal text-gray-400">
            (window: {data.window_seconds}s)
          </span>
        </h2>
        <button className={btnCls} onClick={handleReset}>Reset API Stats</button>
      </div>
      {data.endpoints.length === 0 ? <NoData /> : (
        <div className="overflow-x-auto">
          <table className={tableCls}>
            <thead>
              <tr>
                <th className={thCls}>Method</th>
                <th className={thCls}>Path</th>
                <th className={thCls + " text-right"}>Count</th>
                <th className={thCls + " text-right"}>p50 ms</th>
                <th className={thCls + " text-right"}>p95 ms</th>
                <th className={thCls + " text-right"}>p99 ms</th>
                <th className={thCls + " text-right"}>Max ms</th>
                <th className={thCls + " text-right"}>Errors</th>
              </tr>
            </thead>
            <tbody>
              {data.endpoints.map((e: EndpointStat) => (
                <tr key={e.method + e.path} className={p95Colour(e.p95_ms) + " border-b border-gray-100"}>
                  <td className={tdCls + " font-mono text-xs font-medium"}>{e.method}</td>
                  <td className={tdCls + " font-mono text-xs"}>{e.path}</td>
                  <td className={tdCls + " text-right"}>{e.count}</td>
                  <td className={tdCls + " text-right"}>{formatMs(e.p50_ms)}</td>
                  <td className={tdCls + " text-right font-medium"}>{formatMs(e.p95_ms)}</td>
                  <td className={tdCls + " text-right"}>{formatMs(e.p99_ms)}</td>
                  <td className={tdCls + " text-right"}>{formatMs(e.max_ms)}</td>
                  <td className={tdCls + " text-right" + (e.error_count > 0 ? " text-red-600 font-medium" : "")}>{e.error_count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function TopQueriesSection({ data }: { data: PerformanceDBResponse }) {
  if (!data.pg_stat_statements_available) {
    return (
      <section className={cardCls}>
        <h2 className={hdrCls + " mb-3"}>Top Queries</h2>
        <div className="rounded-md bg-gray-100 px-4 py-3 text-sm text-gray-500">
          pg_stat_statements extension is not available
        </div>
      </section>
    );
  }

  return (
    <section className={cardCls}>
      <h2 className={hdrCls + " mb-3"}>Top Queries</h2>
      {data.top_queries.length === 0 ? <NoData /> : (
        <div className="overflow-x-auto">
          <table className={tableCls}>
            <thead>
              <tr>
                <th className={thCls}>Query</th>
                <th className={thCls + " text-right"}>Calls</th>
                <th className={thCls + " text-right"}>Total ms</th>
                <th className={thCls + " text-right"}>Mean ms</th>
                <th className={thCls + " text-right"}>Max ms</th>
                <th className={thCls + " text-right"}>Rows</th>
                <th className={thCls + " text-right"}>Cache Hit</th>
              </tr>
            </thead>
            <tbody>
              {data.top_queries.map((q: TopQueryStat, i: number) => (
                <tr key={i} className={queryRowColour(q.max_time_ms) + " border-b border-gray-100"}>
                  <td className={tdCls + " font-mono text-xs max-w-xs"} title={q.query}>
                    {truncate(q.query, 120)}
                  </td>
                  <td className={tdCls + " text-right"}>{q.calls}</td>
                  <td className={tdCls + " text-right"}>{formatMs(q.total_time_ms)}</td>
                  <td className={tdCls + " text-right"}>{formatMs(q.mean_time_ms)}</td>
                  <td className={tdCls + " text-right font-medium"}>{formatMs(q.max_time_ms)}</td>
                  <td className={tdCls + " text-right"}>{q.rows}</td>
                  <td className={tdCls + " text-right"}>{cacheHitRatio(q.shared_blks_hit, q.shared_blks_read)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="mt-2 text-xs italic text-gray-400">
            Queries sorted by total execution time. High max_time indicates worst-case latency spikes.
          </p>
        </div>
      )}
    </section>
  );
}

function TableHealthSection({ tables }: { tables: PgTableStat[] }) {
  const needsIndex = (t: PgTableStat) => t.seq_scan > 100 && t.idx_scan === 0;
  const manyDead = (t: PgTableStat) => t.n_dead_tup > 1000;

  return (
    <section className={cardCls}>
      <h2 className={hdrCls + " mb-3"}>Table Health</h2>
      {tables.length === 0 ? <NoData /> : (
        <div className="overflow-x-auto">
          <table className={tableCls}>
            <thead>
              <tr>
                <th className={thCls}>Table</th>
                <th className={thCls + " text-right"}>Seq Scan</th>
                <th className={thCls + " text-right"}>Idx Scan</th>
                <th className={thCls + " text-right"}>Live Tuples</th>
                <th className={thCls + " text-right"}>Dead Tuples</th>
                <th className={thCls}>Last Vacuum</th>
                <th className={thCls}>Last Analyze</th>
              </tr>
            </thead>
            <tbody>
              {tables.map((t: PgTableStat) => (
                <tr key={t.table_name} className={(needsIndex(t) ? "bg-red-50" : "") + " border-b border-gray-100"}>
                  <td className={tdCls + " font-mono text-xs font-medium"}>{t.table_name}</td>
                  <td className={tdCls + " text-right"}>{t.seq_scan}</td>
                  <td className={tdCls + " text-right"}>
                    {t.idx_scan}
                    {needsIndex(t) && (
                      <div className="text-xs italic text-gray-400">Sequential scans only — may need an index</div>
                    )}
                  </td>
                  <td className={tdCls + " text-right"}>{t.n_live_tup}</td>
                  <td className={tdCls + " text-right"}>
                    {t.n_dead_tup}
                    {manyDead(t) && (
                      <div className="text-xs italic text-gray-400">Many dead tuples — VACUUM may be needed</div>
                    )}
                  </td>
                  <td className={tdCls}>{shortDate(t.last_vacuum)}</td>
                  <td className={tdCls}>{shortDate(t.last_analyze)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function IndexUsageSection({ indexes }: { indexes: PgIndexStat[] }) {
  const rowColour = (ix: PgIndexStat): string => {
    if (ix.idx_scan === 0 && ix.size_bytes > 1_000_000) return "bg-red-50";
    if (ix.idx_scan === 0) return "bg-yellow-50";
    return "";
  };

  return (
    <section className={cardCls}>
      <h2 className={hdrCls + " mb-3"}>Index Usage</h2>
      {indexes.length === 0 ? <NoData /> : (
        <div className="overflow-x-auto">
          <table className={tableCls}>
            <thead>
              <tr>
                <th className={thCls}>Table</th>
                <th className={thCls}>Index</th>
                <th className={thCls + " text-right"}>Scans</th>
                <th className={thCls + " text-right"}>Tuples Read</th>
                <th className={thCls + " text-right"}>Tuples Fetched</th>
                <th className={thCls + " text-right"}>Size</th>
              </tr>
            </thead>
            <tbody>
              {indexes.map((ix: PgIndexStat) => (
                <tr key={ix.table_name + ix.index_name} className={rowColour(ix) + " border-b border-gray-100"}>
                  <td className={tdCls + " font-mono text-xs"}>{ix.table_name}</td>
                  <td className={tdCls + " font-mono text-xs font-medium"}>
                    {ix.index_name}
                    {ix.idx_scan === 0 && (
                      <div className="text-xs italic text-gray-400">Never used — candidate for removal</div>
                    )}
                  </td>
                  <td className={tdCls + " text-right"}>{ix.idx_scan}</td>
                  <td className={tdCls + " text-right"}>{ix.idx_tup_read}</td>
                  <td className={tdCls + " text-right"}>{ix.idx_tup_fetch}</td>
                  <td className={tdCls + " text-right"}>{formatBytes(ix.size_bytes)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function ActiveQueriesSection({ queries }: { queries: PgActiveQuery[] }) {
  return (
    <section className={cardCls}>
      <h2 className={hdrCls + " mb-3"}>Active Queries</h2>
      {queries.length === 0 ? <NoData /> : (
        <div className="overflow-x-auto">
          <table className={tableCls}>
            <thead>
              <tr>
                <th className={thCls}>PID</th>
                <th className={thCls}>State</th>
                <th className={thCls}>Query</th>
                <th className={thCls + " text-right"}>Duration ms</th>
                <th className={thCls}>Wait Event Type</th>
                <th className={thCls}>Wait Event</th>
              </tr>
            </thead>
            <tbody>
              {queries.map((q: PgActiveQuery) => (
                <tr key={q.pid} className={durationRowColour(q.duration_ms) + " border-b border-gray-100"}>
                  <td className={tdCls + " font-mono text-xs"}>{q.pid}</td>
                  <td className={tdCls}>{q.state}</td>
                  <td className={tdCls + " font-mono text-xs max-w-xs"} title={q.query}>
                    {truncate(q.query, 120)}
                  </td>
                  <td className={tdCls + " text-right font-medium"}>{formatMs(q.duration_ms)}</td>
                  <td className={tdCls}>{q.wait_event_type ?? "—"}</td>
                  <td className={tdCls}>{q.wait_event ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function AdminPerformancePage() {
  const [apiData, setApiData] = useState<PerformanceResponse | null>(null);
  const [dbData, setDbData] = useState<PerformanceDBResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const load = useCallback(() => {
    Promise.all([fetchPerformanceStats(), fetchPerformanceDB()])
      .then(([api, db]) => {
        setApiData(api);
        setDbData(db);
        setError(null);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
    intervalRef.current = setInterval(load, 10_000);
    return () => { if (intervalRef.current) clearInterval(intervalRef.current); };
  }, [load]);

  if (loading) return <LoadingSpinner message="Loading performance data…" />;
  if (error && !apiData && !dbData) return <ErrorAlert message="Failed to load performance data" detail={error} onRetry={load} />;

  const handleResetApi = () => {
    if (window.confirm("Reset API performance stats? This clears all collected endpoint metrics.")) {
      resetPerformanceStats().then(load).catch(() => {});
    }
  };

  const handleResetDb = () => {
    if (window.confirm("Reset DB performance stats? This clears cumulative pg_stat data including query statistics and table/index counters. This action cannot be undone.")) {
      resetPerformanceDB().then(load).catch(() => {});
    }
  };

  const [vacuuming, setVacuuming] = useState(false);
  const handleVacuum = () => {
    if (window.confirm("Run VACUUM FULL? This reclaims disk space by rewriting all tables. The database will be locked during this operation — it may take several minutes for large databases.")) {
      setVacuuming(true);
      vacuumFull()
        .then(load)
        .catch(() => {})
        .finally(() => setVacuuming(false));
    }
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-gray-900">Performance Diagnostics</h1>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          Refresh error: {error}
        </div>
      )}

      {apiData && <EndpointsSection data={apiData} />}

      {dbData && (
        <>
          <TopQueriesSection data={dbData} />
          <TableHealthSection tables={dbData.table_stats} />
          <IndexUsageSection indexes={dbData.index_stats} />
          <ActiveQueriesSection queries={dbData.active_queries} />
        </>
      )}

      <section className={cardCls}>
        <h2 className={hdrCls + " mb-3"}>Actions</h2>
        <div className="flex gap-3">
          <button className={btnCls} onClick={handleResetApi}>Reset API Stats</button>
          <button className={btnDangerCls} onClick={handleResetDb}>Reset DB Stats</button>
          <button className={btnDangerCls} onClick={handleVacuum} disabled={vacuuming}>
            {vacuuming ? "Vacuuming…" : "VACUUM FULL"}
          </button>
        </div>
        <p className="mt-2 text-xs text-gray-400">
          VACUUM FULL reclaims disk space by rewriting tables. Only needed when disk space is critically low — PostgreSQL normally reuses free space automatically. Tables are locked during the operation.
        </p>
      </section>
    </div>
  );
}
