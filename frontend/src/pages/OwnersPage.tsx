import { useState, useEffect, useCallback, useMemo } from "react";
import { Link } from "react-router-dom";
import { fetchOwners, createOwner, type OwnerFilterQuery } from "../api";
import type { Owner, Pagination as PaginationType } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { useAuth } from "../context/AuthContext";

// ---------------------------------------------------------------------------
// Owner type display labels
// ---------------------------------------------------------------------------

const OWNER_TYPE_LABELS: Record<string, string> = {
  team: "Team",
  individual: "Individual",
  business_unit: "Business Unit",
  cost_centre: "Cost Centre",
  custom: "Custom",
};

function ownerTypeLabel(t: string): string {
  return OWNER_TYPE_LABELS[t] ?? t;
}

// ---------------------------------------------------------------------------
// Readiness bar — compact inline stacked bar for ready/blocked/stale
// ---------------------------------------------------------------------------

function ReadinessBar({
  ready,
  blocked,
  stale,
  total,
}: {
  ready: number;
  blocked: number;
  stale: number;
  total: number;
}) {
  if (total === 0) {
    return <span className="text-xs text-gray-400">—</span>;
  }
  const pctReady = (ready / total) * 100;
  const pctBlocked = (blocked / total) * 100;
  const pctStale = (stale / total) * 100;

  return (
    <div className="flex items-center gap-2">
      <div className="h-2 w-20 overflow-hidden rounded-full bg-gray-200">
        <div className="flex h-full">
          <div className="bg-green-500" style={{ width: `${pctReady}%` }} />
          <div className="bg-red-500" style={{ width: `${pctBlocked}%` }} />
          <div className="bg-amber-400" style={{ width: `${pctStale}%` }} />
        </div>
      </div>
      <span className="whitespace-nowrap text-xs text-gray-500">
        {ready}/{total}
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sortable column header
// ---------------------------------------------------------------------------

type SortField = "name" | "owner_type" | "created_at" | "updated_at";
type SortDir = "asc" | "desc";

function SortHeader({
  label,
  field,
  currentField,
  currentDir,
  onSort,
}: {
  label: string;
  field: SortField;
  currentField: SortField;
  currentDir: SortDir;
  onSort: (field: SortField) => void;
}) {
  const active = field === currentField;
  return (
    <th
      onClick={() => onSort(field)}
      className="cursor-pointer select-none hover:text-blue-600"
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {active ? (
          <svg
            className="h-3.5 w-3.5 text-blue-600"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d={
                currentDir === "asc"
                  ? "M4.5 15.75l7.5-7.5 7.5 7.5"
                  : "M19.5 8.25l-7.5 7.5-7.5-7.5"
              }
            />
          </svg>
        ) : (
          <svg
            className="h-3.5 w-3.5 text-gray-300"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9"
            />
          </svg>
        )}
      </span>
    </th>
  );
}

// ---------------------------------------------------------------------------
// Owners list page
// ---------------------------------------------------------------------------

export function OwnersPage() {
  const { user } = useAuth();
  const canCreate = user?.role === "operator" || user?.role === "admin";

  const [owners, setOwners] = useState<Owner[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filter state
  const [search, setSearch] = useState("");
  const [ownerType, setOwnerType] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 50;

  // Sort state
  const [sortField, setSortField] = useState<SortField>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  // Create form state
  const [showCreate, setShowCreate] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createDisplayName, setCreateDisplayName] = useState("");
  const [createOwnerType, setCreateOwnerType] = useState("team");
  const [createEmail, setCreateEmail] = useState("");
  const [createChannel, setCreateChannel] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: OwnerFilterQuery = {
      page,
      per_page: perPage,
      sort: sortField,
      order: sortDir,
    };
    if (search) filters.search = search;
    if (ownerType) filters.owner_type = ownerType;

    fetchOwners(filters)
      .then((res) => {
        setOwners(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [search, ownerType, page, sortField, sortDir]);

  useEffect(() => {
    load();
  }, [load]);

  // Reset to page 1 when filters or sort changes.
  useEffect(() => {
    setPage(1);
  }, [search, ownerType, sortField, sortDir]);

  const handleSort = useCallback(
    (field: SortField) => {
      if (field === sortField) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
      } else {
        setSortField(field);
        setSortDir("asc");
      }
    },
    [sortField],
  );

  // Check if any owner has readiness data to decide whether to show the column.
  const showReadiness = useMemo(
    () => owners.some((o) => o.readiness && o.readiness.total_nodes > 0),
    [owners],
  );

  const resetCreateForm = () => {
    setCreateName("");
    setCreateDisplayName("");
    setCreateOwnerType("team");
    setCreateEmail("");
    setCreateChannel("");
    setCreateError(null);
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreating(true);
    setCreateError(null);

    try {
      await createOwner({
        name: createName.trim(),
        owner_type: createOwnerType,
        display_name: createDisplayName.trim() || undefined,
        contact_email: createEmail.trim() || undefined,
        contact_channel: createChannel.trim() || undefined,
      });
      resetCreateForm();
      setShowCreate(false);
      load();
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Failed to create owner.";
      setCreateError(message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-xl font-bold text-gray-800">Owners</h2>
        <div className="flex items-center gap-2">
          <Link
            to="/ownership/import"
            className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
          >
            Import CSV / JSON
          </Link>
          <Link
            to="/ownership/audit-log"
            className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
          >
            Audit Log
          </Link>
          {canCreate && (
            <button
              onClick={() => {
                if (showCreate) {
                  resetCreateForm();
                }
                setShowCreate(!showCreate);
              }}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition-colors hover:bg-blue-700"
            >
              {showCreate ? "Cancel" : "New Owner"}
            </button>
          )}
        </div>
      </div>

      {/* Inline create form */}
      {showCreate && (
        <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
          <h3 className="mb-3 text-sm font-semibold text-gray-700">Create Owner</h3>
          {createError && (
            <div className="mb-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
              {createError}
            </div>
          )}
          <form onSubmit={handleCreate} className="flex flex-wrap items-end gap-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-500">
                Name <span className="text-red-500">*</span>
              </label>
              <input
                type="text"
                required
                value={createName}
                onChange={(e) => setCreateName(e.target.value)}
                placeholder="unique-owner-name"
                className="block w-44 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-500">Display Name</label>
              <input
                type="text"
                value={createDisplayName}
                onChange={(e) => setCreateDisplayName(e.target.value)}
                placeholder="Friendly Name"
                className="block w-44 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-500">
                Type <span className="text-red-500">*</span>
              </label>
              <select
                required
                value={createOwnerType}
                onChange={(e) => setCreateOwnerType(e.target.value)}
                className="block w-36 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                <option value="team">Team</option>
                <option value="individual">Individual</option>
                <option value="business_unit">Business Unit</option>
                <option value="cost_centre">Cost Centre</option>
                <option value="custom">Custom</option>
              </select>
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-500">Contact Email</label>
              <input
                type="text"
                value={createEmail}
                onChange={(e) => setCreateEmail(e.target.value)}
                placeholder="team@example.com"
                className="block w-48 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-500">Contact Channel</label>
              <input
                type="text"
                value={createChannel}
                onChange={(e) => setCreateChannel(e.target.value)}
                placeholder="#slack-channel"
                className="block w-40 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </div>
            <button
              type="submit"
              disabled={creating}
              className="rounded-md bg-green-600 px-4 py-1.5 text-sm font-medium text-white shadow-sm transition-colors hover:bg-green-700 disabled:opacity-50"
            >
              {creating ? "Creating…" : "Create"}
            </button>
          </form>
        </div>
      )}

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Search</label>
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Filter by name"
            className="block w-40 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Owner Type</label>
          <select
            value={ownerType}
            onChange={(e) => setOwnerType(e.target.value)}
            className="block w-36 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">All</option>
            <option value="team">Team</option>
            <option value="individual">Individual</option>
            <option value="business_unit">Business Unit</option>
            <option value="cost_centre">Cost Centre</option>
            <option value="custom">Custom</option>
          </select>
        </div>
      </div>

      {/* Table */}
      {loading && <LoadingSpinner message="Loading owners…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {owners.length === 0 ? (
            <EmptyState title="No owners found" description="Adjust filters or create a new owner." />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <SortHeader label="Name" field="name" currentField={sortField} currentDir={sortDir} onSort={handleSort} />
                    <th>Display Name</th>
                    <SortHeader label="Type" field="owner_type" currentField={sortField} currentDir={sortDir} onSort={handleSort} />
                    <th>Nodes</th>
                    <th>Cookbooks</th>
                    <th>Git Repos</th>
                    {showReadiness && <th>Readiness</th>}
                    <SortHeader label="Created" field="created_at" currentField={sortField} currentDir={sortDir} onSort={handleSort} />
                  </tr>
                </thead>
                <tbody>
                  {owners.map((owner) => (
                    <tr key={owner.name}>
                      <td>
                        <Link
                          to={`/ownership/${encodeURIComponent(owner.name)}`}
                          className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
                        >
                          {owner.name}
                        </Link>
                      </td>
                      <td className="text-sm text-gray-600">
                        {owner.display_name || "\u2014"}
                      </td>
                      <td>
                        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600">
                          {ownerTypeLabel(owner.owner_type)}
                        </span>
                      </td>
                      <td className="text-sm text-gray-600">
                        {owner.assignment_counts?.node ?? 0}
                      </td>
                      <td className="text-sm text-gray-600">
                        {owner.assignment_counts?.cookbook ?? 0}
                      </td>
                      <td className="text-sm text-gray-600">
                        {owner.assignment_counts?.git_repo ?? 0}
                      </td>
                      {showReadiness && (
                        <td>
                          {owner.readiness ? (
                            <ReadinessBar
                              ready={owner.readiness.ready}
                              blocked={owner.readiness.blocked}
                              stale={owner.readiness.stale}
                              total={owner.readiness.total_nodes}
                            />
                          ) : (
                            <span className="text-xs text-gray-400">{"\u2014"}</span>
                          )}
                        </td>
                      )}
                      <td className="text-xs text-gray-400">
                        {new Date(owner.created_at).toLocaleString()}
                      </td>
                    </tr>
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
