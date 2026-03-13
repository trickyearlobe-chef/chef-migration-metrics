import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { fetchAuditLog, type AuditLogFilterQuery } from "../api";
import type { OwnershipAuditEntry, Pagination as PaginationType } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";

// ---------------------------------------------------------------------------
// Ownership Audit Log page — paginated table from
// GET /api/v1/ownership/audit-log with filters for action, owner_name,
// and actor. Each row shows timestamp, action, actor, owner, entity type,
// entity key, and an expandable JSON details view.
// ---------------------------------------------------------------------------

const ACTION_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "All" },
  { value: "owner_created", label: "Owner Created" },
  { value: "owner_updated", label: "Owner Updated" },
  { value: "owner_deleted", label: "Owner Deleted" },
  { value: "assignment_created", label: "Assignment Created" },
  { value: "assignment_deleted", label: "Assignment Deleted" },
  { value: "assignment_reassigned", label: "Assignment Reassigned" },
];

/** Format a snake_case action string as Title Case (e.g. "owner_created" → "Owner Created"). */
function formatAction(action: string): string {
  return action
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

export function OwnershipAuditLogPage() {
  const [entries, setEntries] = useState<OwnershipAuditEntry[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filter state
  const [action, setAction] = useState("");
  const [ownerName, setOwnerName] = useState("");
  const [actor, setActor] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 50;

  // Track which row's details are expanded (by entry id)
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: AuditLogFilterQuery = {
      page,
      per_page: perPage,
    };
    if (action) filters.action = action;
    if (ownerName) filters.owner_name = ownerName;
    if (actor) filters.actor = actor;

    fetchAuditLog(filters)
      .then((res) => {
        setEntries(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [action, ownerName, actor, page]);

  useEffect(() => {
    load();
  }, [load]);

  // Reset to page 1 when filters change.
  useEffect(() => {
    setPage(1);
  }, [action, ownerName, actor]);

  // Count active filters for the clear button.
  const activeFilterCount = [action, ownerName, actor].filter(Boolean).length;

  const clearFilters = () => {
    setAction("");
    setOwnerName("");
    setActor("");
  };

  return (
    <div className="space-y-4">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <Link to="/ownership" className="hover:text-blue-600 hover:underline">
          Ownership
        </Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">Audit Log</span>
      </nav>

      <h2 className="text-xl font-bold text-gray-800">Ownership Audit Log</h2>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Action</label>
          <select
            value={action}
            onChange={(e) => setAction(e.target.value)}
            className="block w-48 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            {ACTION_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Owner Name</label>
          <input
            type="text"
            value={ownerName}
            onChange={(e) => setOwnerName(e.target.value)}
            placeholder="Filter by owner"
            className="block w-40 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Actor</label>
          <input
            type="text"
            value={actor}
            onChange={(e) => setActor(e.target.value)}
            placeholder="Filter by actor"
            className="block w-40 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

        {activeFilterCount > 0 && (
          <button
            onClick={clearFilters}
            className="mb-0.5 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-900"
            title="Clear all filters"
          >
            Clear ({activeFilterCount})
          </button>
        )}
      </div>

      {/* Table */}
      {loading && <LoadingSpinner message="Loading audit log…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {entries.length === 0 ? (
            <EmptyState
              title="No audit log entries found"
              description="Adjust filters or check back after ownership changes have been made."
            />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th>Timestamp</th>
                    <th>Action</th>
                    <th>Actor</th>
                    <th>Owner</th>
                    <th>Entity Type</th>
                    <th>Entity Key</th>
                    <th>Details</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((entry) => (
                    <AuditLogRow
                      key={entry.id}
                      entry={entry}
                      isExpanded={expandedId === entry.id}
                      onToggle={() =>
                        setExpandedId(expandedId === entry.id ? null : entry.id)
                      }
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {pagination && (
            <Pagination pagination={pagination} onPageChange={setPage} />
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Row component — renders the main row and an optional expanded detail row
// ---------------------------------------------------------------------------

function AuditLogRow({
  entry,
  isExpanded,
  onToggle,
}: {
  entry: OwnershipAuditEntry;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const hasDetails =
    entry.details !== undefined &&
    entry.details !== null &&
    Object.keys(entry.details).length > 0;

  return (
    <>
      <tr>
        <td className="text-xs text-gray-500 whitespace-nowrap">
          {new Date(entry.timestamp).toLocaleString()}
        </td>
        <td>
          <span className="inline-block rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700">
            {formatAction(entry.action)}
          </span>
        </td>
        <td className="text-sm">{entry.actor}</td>
        <td>
          <Link
            to={`/ownership/${encodeURIComponent(entry.owner_name)}`}
            className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
          >
            {entry.owner_name}
          </Link>
        </td>
        <td className="text-sm text-gray-600">{entry.entity_type || "—"}</td>
        <td className="text-sm text-gray-600">
          {entry.entity_key ? (
            <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">
              {entry.entity_key}
            </code>
          ) : (
            "—"
          )}
        </td>
        <td>
          {hasDetails ? (
            <button
              onClick={onToggle}
              className="rounded border border-gray-300 bg-white px-2 py-0.5 text-xs font-medium text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-900"
            >
              {isExpanded ? "Hide" : "View"}
            </button>
          ) : (
            <span className="text-xs text-gray-400">—</span>
          )}
        </td>
      </tr>
      {isExpanded && hasDetails && (
        <tr>
          <td colSpan={7} className="bg-gray-50 p-0">
            <pre className="overflow-x-auto whitespace-pre-wrap p-4 text-xs text-gray-700">
              {JSON.stringify(entry.details, null, 2)}
            </pre>
          </td>
        </tr>
      )}
    </>
  );
}
