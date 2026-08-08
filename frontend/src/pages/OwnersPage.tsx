import { useState, useEffect, useCallback, useMemo } from "react";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { Link } from "react-router-dom";
import { useSort } from "../hooks/useSort";
import { SortableColumnHeader } from "../components/SortableColumnHeader";
import { FilterInput, FilterSelect } from "../components/FilterInputs";
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
    return <span className="text-xs text-gray-400">{"\u2014"}</span>;
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

type SortField =
  | "name"
  | "owner_type"
  | "nodes"
  | "git_repos"
  | "ready"
  | "blocked"
  | "created_at"
  | "updated_at";

type SortDir = "asc" | "desc";

// ---------------------------------------------------------------------------
// Owners list page
// ---------------------------------------------------------------------------

export function OwnersPage() {
  const { user, isAdmin } = useAuth();
  const canCreate = user?.role === "operator" || user?.role === "admin";

  const [owners, setOwners] = useState<Owner[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filter state
  const [search, setSearch] = useState("");
  const [ownerType, setOwnerType] = useState("");
  const [page, setPage] = useState(1);
  const perPage = DEFAULT_PAGE_SIZE;

  // Sort state — default to blocked descending to surface remediation work
  const {
    sortField,
    sortOrder: sortDir,
    handleSort,
  } = useSort<SortField>({
    defaultField: "blocked",
    defaultOrder: "desc",
    descendingFields: ["nodes", "git_repos", "ready", "blocked"],
  });

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
      order: sortDir as SortDir,
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

  // Reset to page 1 when filters or sort change.
  useEffect(() => {
    setPage(1);
  }, [search, ownerType, sortField, sortDir]);

  // Check if any owner has readiness data.
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

  // Count active filters for the clear button.
  const activeFilterCount = [search, ownerType].filter(Boolean).length;

  const clearFilters = () => {
    setSearch("");
    setOwnerType("");
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-xl font-bold text-gray-800">Owners</h2>
        <div className="flex items-center gap-2">
          {/* Importing owners and reconciling duplicate people are admin
              functions, so they are offered only to an admin — a button that
              bounces you to the dashboard reads as a fault, not a permission. */}
          {isAdmin && (
            <Link
              to="/admin/ownership/duplicates"
              className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
            >
              Possible Duplicates
            </Link>
          )}
          <Link
            to="/ownership/aliases"
            className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
          >
            Aliases
          </Link>
          {isAdmin && (
            <Link
              to="/admin/ownership/import"
              className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
            >
              Import owners
            </Link>
          )}
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
          <h3 className="mb-3 text-sm font-semibold text-gray-700">
            Create Owner
          </h3>
          {createError && (
            <div className="mb-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
              {createError}
            </div>
          )}
          <form
            onSubmit={handleCreate}
            className="flex flex-wrap items-end gap-3"
          >
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
              <label className="mb-1 block text-xs font-medium text-gray-500">
                Display Name
              </label>
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
              <label className="mb-1 block text-xs font-medium text-gray-500">
                Contact Email
              </label>
              <input
                type="text"
                value={createEmail}
                onChange={(e) => setCreateEmail(e.target.value)}
                placeholder="team@example.com"
                className="block w-48 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-500">
                Contact Channel
              </label>
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
              {creating ? "Creating\u2026" : "Create"}
            </button>
          </form>
        </div>
      )}

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <FilterInput
          label="Search"
          value={search}
          onChange={setSearch}
          placeholder="Filter by name"
        />
        <FilterSelect
          label="Owner Type"
          value={ownerType}
          onChange={setOwnerType}
          options={[
            { value: "", label: "All" },
            { value: "team", label: "Team" },
            { value: "individual", label: "Individual" },
            { value: "business_unit", label: "Business Unit" },
            { value: "cost_centre", label: "Cost Centre" },
            { value: "custom", label: "Custom" },
          ]}
          wide
        />
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

      {/* Legend for readiness bar */}
      {showReadiness && (
        <div className="flex items-center gap-4 text-xs text-gray-500">
          <span className="font-medium text-gray-600">Readiness:</span>
          <span className="flex items-center gap-1">
            <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />{" "}
            Ready
          </span>
          <span className="flex items-center gap-1">
            <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-500" />{" "}
            Blocked
          </span>
          <span className="flex items-center gap-1">
            <span className="inline-block h-2.5 w-2.5 rounded-full bg-amber-400" />{" "}
            Stale
          </span>
        </div>
      )}

      {/* Table */}
      {loading && <LoadingSpinner message="Loading owners\u2026" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {owners.length === 0 ? (
            !search && !ownerType ? (
              <div className="rounded-lg border border-gray-200 bg-white p-8 text-center">
                <svg className="mx-auto mb-3 h-10 w-10 text-gray-300" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M18 18.72a9.094 9.094 0 0 0 3.741-.479 3 3 0 0 0-4.682-2.72m.94 3.198.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0 1 12 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 0 1 6 18.719m12 0a5.971 5.971 0 0 0-.941-3.197m0 0A5.995 5.995 0 0 0 12 12.75a5.995 5.995 0 0 0-5.058 2.772m0 0a3 3 0 0 0-4.681 2.72 8.986 8.986 0 0 0 3.74.477m.94-3.197a5.971 5.971 0 0 0-.94 3.197M15 6.75a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
                </svg>
                <h3 className="mb-1 text-sm font-semibold text-gray-700">No owners configured</h3>
                <p className="mb-4 text-sm text-gray-500">
                  Owners track who is responsible for each cookbook and node.
                  Add owners manually, or ask an administrator to import them
                  from a file or a database.
                </p>
                <div className="flex flex-wrap justify-center gap-3">
                  {isAdmin && (
                    <Link to="/admin/ownership/import" className="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-blue-700">
                      Import owners
                    </Link>
                  )}
                  {canCreate && (
                    <Link to="/admin/config/organisations" className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50">
                      Organisation settings
                    </Link>
                  )}
                </div>
              </div>
            ) : (
              <EmptyState
                title="No owners found"
                description="Adjust filters or create a new owner."
              />
            )
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <SortableColumnHeader
                      label="Name"
                      field="name"
                      currentField={sortField}
                      currentOrder={sortDir}
                      onSort={handleSort}
                    />
                    <th>Display Name</th>
                    <SortableColumnHeader
                      label="Type"
                      field="owner_type"
                      currentField={sortField}
                      currentOrder={sortDir}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Nodes"
                      field="nodes"
                      currentField={sortField}
                      currentOrder={sortDir}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Git Repos"
                      field="git_repos"
                      currentField={sortField}
                      currentOrder={sortDir}
                      onSort={handleSort}
                    />
                    {showReadiness && (
                      <>
                        <SortableColumnHeader
                          label="Ready"
                          field="ready"
                          currentField={sortField}
                          currentOrder={sortDir}
                          onSort={handleSort}
                        />
                        <SortableColumnHeader
                          label="Blocked"
                          field="blocked"
                          currentField={sortField}
                          currentOrder={sortDir}
                          onSort={handleSort}
                        />
                        <th>Readiness</th>
                      </>
                    )}
                    <SortableColumnHeader
                      label="Created"
                      field="created_at"
                      currentField={sortField}
                      currentOrder={sortDir}
                      onSort={handleSort}
                    />
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
                      <td className="text-sm text-gray-600 tabular-nums">
                        {owner.assignment_counts?.node ?? 0}
                      </td>
                      <td className="text-sm text-gray-600 tabular-nums">
                        {owner.assignment_counts?.git_repo ?? 0}
                      </td>
                      {showReadiness && (
                        <>
                          <td className="text-sm tabular-nums text-green-700">
                            {owner.readiness?.ready ?? 0}
                          </td>
                          <td className="text-sm tabular-nums text-red-700 font-medium">
                            {owner.readiness?.blocked ?? 0}
                          </td>
                          <td>
                            {owner.readiness ? (
                              <ReadinessBar
                                ready={owner.readiness.ready}
                                blocked={owner.readiness.blocked}
                                stale={owner.readiness.stale}
                                total={owner.readiness.total_nodes}
                              />
                            ) : (
                              <span className="text-xs text-gray-400">
                                {"\u2014"}
                              </span>
                            )}
                          </td>
                        </>
                      )}
                      <td className="text-xs text-gray-400">
                        {new Date(owner.created_at).toLocaleDateString()}
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
