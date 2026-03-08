import { useState, useEffect, useCallback } from "react";
import { useOrg } from "../context/OrgContext";
import {
  fetchLogs,
  fetchCollectionRuns,
  fetchLogDetail,
  type LogFilterQuery,
  type CollectionRunFilterQuery,
} from "../api";
import type {
  LogEntry,
  CollectionRunWithOrg,
  Pagination as PaginationType,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";

// ---------------------------------------------------------------------------
// Log viewer page — two tabs:
//   1. Logs: paginated table of log entries with severity/scope/org filters
//   2. Collection Runs: paginated table of collection run history
//
// Clicking a log row opens an inline detail panel showing full metadata
// and process output.
// ---------------------------------------------------------------------------

type Tab = "logs" | "runs";

const SEVERITIES = ["DEBUG", "INFO", "WARN", "ERROR"];

const SCOPES = [
  "collection_run",
  "git_operation",
  "test_kitchen_run",
  "cookstyle_scan",
  "notification_dispatch",
  "export_job",
  "tls",
  "readiness_evaluation",
  "startup",
  "secrets",
  "remediation",
  "webapi",
];

const RUN_STATUSES = ["running", "completed", "failed", "interrupted"];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function severityBadge(severity: string) {
  const s = severity.toUpperCase();
  const styles: Record<string, string> = {
    DEBUG: "bg-gray-100 text-gray-600 ring-gray-500/20",
    INFO: "bg-blue-100 text-blue-700 ring-blue-600/20",
    WARN: "bg-amber-100 text-amber-800 ring-amber-600/20",
    ERROR: "bg-red-100 text-red-800 ring-red-600/20",
  };
  const cls = styles[s] ?? styles.DEBUG;
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase ring-1 ring-inset ${cls}`}>
      {s}
    </span>
  );
}

function statusBadge(status: string) {
  const styles: Record<string, string> = {
    running: "bg-blue-100 text-blue-700 ring-blue-600/20",
    completed: "bg-green-100 text-green-800 ring-green-600/20",
    failed: "bg-red-100 text-red-800 ring-red-600/20",
    interrupted: "bg-amber-100 text-amber-800 ring-amber-600/20",
  };
  const cls = styles[status] ?? "bg-gray-100 text-gray-600 ring-gray-500/20";
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold capitalize ring-1 ring-inset ${cls}`}>
      {status}
    </span>
  );
}

function scopeLabel(scope: string) {
  return scope.replace(/_/g, " ");
}

function formatDuration(start: string, end?: string): string {
  if (!end) return "\u2014";
  const ms = new Date(end).getTime() - new Date(start).getTime();
  if (ms < 0) return "\u2014";
  if (ms < 1000) return `${ms}ms`;
  const secs = Math.floor(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const remainSecs = secs % 60;
  if (mins < 60) return `${mins}m ${remainSecs}s`;
  const hours = Math.floor(mins / 60);
  const remainMins = mins % 60;
  return `${hours}h ${remainMins}m`;
}

// ---------------------------------------------------------------------------
// Main page component
// ---------------------------------------------------------------------------

export function LogsPage() {
  const [tab, setTab] = useState<Tab>("logs");

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-gray-800">Logs</h2>
        <div className="flex rounded-lg bg-gray-100 p-0.5">
          <TabButton active={tab === "logs"} onClick={() => setTab("logs")} label="Log Entries" />
          <TabButton active={tab === "runs"} onClick={() => setTab("runs")} label="Collection Runs" />
        </div>
      </div>
      {tab === "logs" ? <LogsTab /> : <CollectionRunsTab />}
    </div>
  );
}

function TabButton({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${active ? "bg-white text-gray-900 shadow-sm" : "text-gray-500 hover:text-gray-700"
        }`}
    >
      {label}
    </button>
  );
}

// ---------------------------------------------------------------------------
// Logs Tab
// ---------------------------------------------------------------------------

function LogsTab() {
  const { selectedOrg } = useOrg();
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [detailEntry, setDetailEntry] = useState<LogEntry | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const [minSeverity, setMinSeverity] = useState("INFO");
  const [scope, setScope] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 50;

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    const filters: LogFilterQuery = {
      page,
      per_page: perPage,
      min_severity: minSeverity || undefined,
      scope: scope || undefined,
    };
    if (selectedOrg) filters.organisation = selectedOrg;
    fetchLogs(filters)
      .then((res) => {
        setLogs(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedOrg, minSeverity, scope, page]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => { setPage(1); }, [selectedOrg, minSeverity, scope]);

  const toggleDetail = useCallback((id: string) => {
    if (expandedId === id) {
      setExpandedId(null);
      setDetailEntry(null);
      return;
    }
    setExpandedId(id);
    setDetailEntry(null);
    setDetailLoading(true);
    fetchLogDetail(id)
      .then((entry) => setDetailEntry(entry))
      .catch(() => {
        const fallback = logs.find((l) => l.id === id) ?? null;
        setDetailEntry(fallback);
      })
      .finally(() => setDetailLoading(false));
  }, [expandedId, logs]);

  const activeFilterCount = [minSeverity !== "INFO" ? minSeverity : "", scope].filter(Boolean).length;

  return (
    <>
      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Min Severity</label>
          <select
            value={minSeverity}
            onChange={(e) => setMinSeverity(e.target.value)}
            className="block w-32 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            {SEVERITIES.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Scope</label>
          <select
            value={scope}
            onChange={(e) => setScope(e.target.value)}
            className="block w-48 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">All scopes</option>
            {SCOPES.map((s) => (
              <option key={s} value={s}>{scopeLabel(s)}</option>
            ))}
          </select>
        </div>
        {activeFilterCount > 0 && (
          <button
            onClick={() => { setMinSeverity("INFO"); setScope(""); }}
            className="mb-0.5 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-900"
            title="Clear all filters"
          >
            Clear ({activeFilterCount})
          </button>
        )}
      </div>

      {/* Log table */}
      {loading && <LoadingSpinner message="Loading logs\u2026" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {logs.length === 0 ? (
            <EmptyState title="No log entries found" description="Adjust filters or wait for application activity." />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th className="w-44">Timestamp</th>
                    <th className="w-20">Severity</th>
                    <th className="w-40">Scope</th>
                    <th>Message</th>
                    <th className="w-32">Organisation</th>
                  </tr>
                </thead>
                <tbody>
                  {logs.map((entry) => (
                    <LogRow
                      key={entry.id}
                      entry={entry}
                      expanded={expandedId === entry.id}
                      onToggle={() => toggleDetail(entry.id)}
                      detailEntry={expandedId === entry.id ? detailEntry : null}
                      detailLoading={expandedId === entry.id ? detailLoading : false}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {pagination && <Pagination pagination={pagination} onPageChange={setPage} />}
        </>
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Log row + expandable detail
// ---------------------------------------------------------------------------

function LogRow({
  entry,
  expanded,
  onToggle,
  detailEntry,
  detailLoading,
}: {
  entry: LogEntry;
  expanded: boolean;
  onToggle: () => void;
  detailEntry: LogEntry | null;
  detailLoading: boolean;
}) {
  return (
    <>
      <tr
        className={`cursor-pointer transition-colors hover:bg-gray-50 ${expanded ? "bg-blue-50/50" : ""}`}
        onClick={onToggle}
      >
        <td className="text-xs text-gray-500 tabular-nums">
          {new Date(entry.timestamp).toLocaleString()}
        </td>
        <td>{severityBadge(entry.severity)}</td>
        <td>
          <span className="inline-block rounded bg-gray-100 px-1.5 py-0.5 text-xs capitalize text-gray-700">
            {scopeLabel(entry.scope)}
          </span>
        </td>
        <td className="max-w-md truncate text-sm text-gray-800">{entry.message}</td>
        <td className="text-xs text-gray-500">{entry.organisation || "\u2014"}</td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={5} className="bg-gray-50 p-0">
            <LogDetail entry={detailEntry} loading={detailLoading} />
          </td>
        </tr>
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Log detail panel
// ---------------------------------------------------------------------------

function LogDetail({ entry, loading }: { entry: LogEntry | null; loading: boolean }) {
  if (loading) {
    return <div className="px-6 py-4 text-sm text-gray-400">Loading detail&hellip;</div>;
  }
  if (!entry) return null;

  const meta: { label: string; value: string }[] = [];
  if (entry.organisation) meta.push({ label: "Organisation", value: entry.organisation });
  if (entry.cookbook_name) meta.push({ label: "Cookbook", value: entry.cookbook_version ? `${entry.cookbook_name} ${entry.cookbook_version}` : entry.cookbook_name });
  if (entry.collection_run_id) meta.push({ label: "Collection Run", value: entry.collection_run_id });
  if (entry.commit_sha) meta.push({ label: "Commit SHA", value: entry.commit_sha });
  if (entry.chef_client_version) meta.push({ label: "Chef Client Version", value: entry.chef_client_version });
  if (entry.notification_channel) meta.push({ label: "Notification Channel", value: entry.notification_channel });
  if (entry.export_job_id) meta.push({ label: "Export Job", value: entry.export_job_id });
  if (entry.tls_domain) meta.push({ label: "TLS Domain", value: entry.tls_domain });

  return (
    <div className="space-y-3 px-6 py-4">
      {/* Full message */}
      <div>
        <span className="text-xs font-medium text-gray-500">Message</span>
        <p className="mt-0.5 text-sm text-gray-800 whitespace-pre-wrap">{entry.message}</p>
      </div>

      {/* Metadata grid */}
      {meta.length > 0 && (
        <div className="grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-3 lg:grid-cols-4">
          {meta.map((m) => (
            <div key={m.label}>
              <span className="text-xs font-medium text-gray-500">{m.label}</span>
              <p className="mt-0.5 truncate text-sm text-gray-700" title={m.value}>
                <code className="rounded bg-gray-100 px-1 py-0.5 text-xs">{m.value}</code>
              </p>
            </div>
          ))}
        </div>
      )}

      {/* Process output (stdout/stderr from external tools) */}
      {entry.process_output && (
        <div>
          <span className="text-xs font-medium text-gray-500">Process Output</span>
          <pre className="mt-1 max-h-64 overflow-auto rounded-md border border-gray-200 bg-gray-900 p-3 text-xs leading-relaxed text-green-400">
            {entry.process_output}
          </pre>
        </div>
      )}

      {/* ID and timestamp footer */}
      <div className="flex gap-4 text-[10px] text-gray-400">
        <span>ID: {entry.id}</span>
        <span>Created: {new Date(entry.created_at).toLocaleString()}</span>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Collection Runs Tab
// ---------------------------------------------------------------------------

function CollectionRunsTab() {
  const { selectedOrg } = useOrg();
  const [runs, setRuns] = useState<CollectionRunWithOrg[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 25;

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    const filters: CollectionRunFilterQuery = {
      page,
      per_page: perPage,
      status: status || undefined,
    };
    if (selectedOrg) filters.organisation = selectedOrg;
    fetchCollectionRuns(filters)
      .then((res) => {
        setRuns(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedOrg, status, page]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => { setPage(1); }, [selectedOrg, status]);

  return (
    <>
      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Status</label>
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="block w-40 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">All statuses</option>
            {RUN_STATUSES.map((s) => (
              <option key={s} value={s}>{s.charAt(0).toUpperCase() + s.slice(1)}</option>
            ))}
          </select>
        </div>
        {status && (
          <button
            onClick={() => setStatus("")}
            className="mb-0.5 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-900"
            title="Clear filter"
          >
            Clear (1)
          </button>
        )}
      </div>

      {/* Collection runs table */}
      {loading && <LoadingSpinner message="Loading collection runs\u2026" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {runs.length === 0 ? (
            <EmptyState title="No collection runs found" description="Adjust filters or wait for the next scheduled collection." />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th>Organisation</th>
                    <th>Status</th>
                    <th>Started</th>
                    <th>Duration</th>
                    <th className="text-right">Nodes Collected</th>
                    <th className="text-right">Total Nodes</th>
                    <th>Error</th>
                  </tr>
                </thead>
                <tbody>
                  {runs.map((item) => {
                    const run = item.run;
                    return (
                      <tr
                        key={run.id}
                        className={run.status === "failed" ? "bg-red-50/40" : run.status === "running" ? "bg-blue-50/40" : ""}
                      >
                        <td className="text-sm font-medium text-gray-800">{item.organisation_name}</td>
                        <td>{statusBadge(run.status)}</td>
                        <td className="text-xs text-gray-500 tabular-nums">
                          {new Date(run.started_at).toLocaleString()}
                        </td>
                        <td className="text-xs text-gray-500 tabular-nums">
                          {formatDuration(run.started_at, run.completed_at)}
                        </td>
                        <td className="text-right tabular-nums">
                          {run.nodes_collected ?? "\u2014"}
                        </td>
                        <td className="text-right tabular-nums">
                          {run.total_nodes ?? "\u2014"}
                        </td>
                        <td className="max-w-xs truncate text-xs text-red-600" title={run.error_message || ""}>
                          {run.error_message || "\u2014"}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
          {pagination && <Pagination pagination={pagination} onPageChange={setPage} />}
        </>
      )}
    </>
  );
}
