import { useState, useEffect, useCallback } from "react";
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
// Owners list page — paginated table from GET /api/v1/owners with filter
// inputs for search text and owner_type. Operators and admins can create
// new owners via an inline form that expands below the header.
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
  }, [search, ownerType, page]);

  useEffect(() => { load(); }, [load]);

  // Reset to page 1 when filters change.
  useEffect(() => { setPage(1); }, [search, ownerType]);

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
                    <th>Name</th>
                    <th>Display Name</th>
                    <th>Type</th>
                    <th>Nodes</th>
                    <th>Cookbooks</th>
                    <th>Git Repos</th>
                    <th>Created</th>
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
                        {owner.display_name || "—"}
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
