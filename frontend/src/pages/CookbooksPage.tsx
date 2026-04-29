import { useState, useEffect, useCallback } from "react";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { Link, useSearchParams } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import { useSort } from "../hooks/useSort";
import { useTargetChefVersion } from "../hooks/useTargetChefVersion";
import { SortableColumnHeader } from "../components/SortableColumnHeader";
import { FilterInput } from "../components/FilterInputs";
import { FilterMultiCheckbox } from "../components/FilterMultiCheckbox";
import { fetchCookbooks, type CookbookFilterQuery } from "../api";
import type { CookbookListItem, Pagination as PaginationType } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StatusBadge, CookStyleBadge, TKBadge } from "../components/StatusBadge";

/** Render a coloured download-status pill with optional error tooltip. */
function DownloadStatusBadge({
  status,
  error,
}: {
  status: string;
  error?: string;
}) {
  const styles: Record<string, string> = {
    ok: "bg-green-100 text-green-800 ring-green-600/20",
    failed: "bg-red-100 text-red-800 ring-red-600/20",
    pending: "bg-gray-100 text-gray-600 ring-gray-500/20",
  };
  const labels: Record<string, string> = {
    ok: "OK",
    failed: "Failed",
    pending: "Pending",
  };
  const cls = styles[status] ?? styles.pending;
  const label = labels[status] ?? status;
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold ring-1 ring-inset ${cls}`}
      title={status === "failed" && error ? error : undefined}
    >
      {label}
      {status === "failed" && error && (
        <span className="ml-1 text-red-400">ⓘ</span>
      )}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Cookbooks list page — paginated table from GET /api/v1/cookbooks showing
// each server cookbook version as its own row with name, version, org,
// active/stale indicators, compatibility, and download status.
//
// Server cookbooks are downloaded from the Chef Infra Server and do not
// have test suites (Test Kitchen is only applicable to git repos, which
// have their own page at /git-repos).
// ---------------------------------------------------------------------------

export function CookbooksPage() {
  const { selectedOrg } = useOrg();
  const [cookbooks, setCookbooks] = useState<CookbookListItem[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // URL search params — read before filter state so initial values can be
  // seeded from query strings (e.g. links from the dashboard).
  const [searchParams, setSearchParams] = useSearchParams();

  // Filters
  const [active, setActive] = useState<string[]>(
    searchParams.get("active")?.split(",").filter(Boolean) ?? [],
  );
  const [nameFilter, setNameFilter] = useState(searchParams.get("name") || "");
  const [compatibility, setCompatibility] = useState<string[]>(
    searchParams.get("compatibility")?.split(",").filter(Boolean) ?? [],
  );
  const [downloadStatus, setDownloadStatus] = useState<string[]>(
    searchParams.get("download_status")?.split(",").filter(Boolean) ?? [],
  );
  const [page, setPage] = useState(1);
  const perPage = DEFAULT_PAGE_SIZE;

  // Sort state — default to name ascending.
  const { sortField, sortOrder, handleSort } = useSort({
    defaultField: "name",
    defaultOrder: "asc",
    descendingFields: ["version", "compatibility", "active", "download_status"],
  });

  // Target Chef versions loaded from backend config.
  const { selectedVersion: selectedTargetVersion } = useTargetChefVersion({
    initialVersion: searchParams.get("target_chef_version") || undefined,
  });

  // Clear search params on mount so they don't persist on manual navigation.
  useEffect(() => {
    if (
      searchParams.has("compatibility") ||
      searchParams.has("active") ||
      searchParams.has("name") ||
      searchParams.has("target_chef_version") ||
      searchParams.has("download_status")
    ) {
      setSearchParams({}, { replace: true });
    }
  }, []); // run once on mount

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: CookbookFilterQuery = {
      page,
      per_page: perPage,
    };
    if (selectedOrg) filters.organisation = selectedOrg;
    if (active.length > 0) filters.active = active.join(",");
    if (nameFilter) filters.name = nameFilter;
    if (compatibility.length > 0)
      filters.compatibility = compatibility.join(",");
    if (downloadStatus.length > 0)
      filters.download_status = downloadStatus.join(",");
    if (selectedTargetVersion)
      filters.target_chef_version = selectedTargetVersion;
    if (sortField) filters.sort = sortField;
    if (sortOrder) filters.order = sortOrder;

    fetchCookbooks(filters)
      .then((res) => {
        setCookbooks(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [
    selectedOrg,
    active,
    nameFilter,
    compatibility,
    downloadStatus,
    selectedTargetVersion,
    page,
    sortField,
    sortOrder,
  ]);

  useEffect(() => {
    load();
  }, [load]);
  useEffect(() => {
    setPage(1);
  }, [
    selectedOrg,
    active,
    nameFilter,
    compatibility,
    downloadStatus,
    selectedTargetVersion,
    sortField,
    sortOrder,
  ]);

  // Count active filters for the clear button.
  const activeFilterCount =
    (nameFilter ? 1 : 0) +
    (active.length > 0 ? 1 : 0) +
    (compatibility.length > 0 ? 1 : 0) +
    (downloadStatus.length > 0 ? 1 : 0);

  const clearFilters = () => {
    setNameFilter("");
    setActive([]);
    setCompatibility([]);
    setDownloadStatus([]);
  };

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-bold text-gray-800">Server Cookbooks</h2>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <FilterInput
          label="Name"
          value={nameFilter}
          onChange={setNameFilter}
          placeholder="Filter by name"
        />
        <FilterMultiCheckbox
          label="Active"
          options={[
            { value: "true", label: "Active" },
            { value: "false", label: "Inactive" },
          ]}
          selected={active}
          onChange={setActive}
        />
        <FilterMultiCheckbox
          label="Compatibility"
          options={[
            { value: "compatible", label: "Compatible" },
            { value: "incompatible", label: "Incompatible" },
            { value: "untested", label: "Untested" },
          ]}
          selected={compatibility}
          onChange={setCompatibility}
        />
        <FilterMultiCheckbox
          label="Download"
          options={[
            { value: "ok", label: "OK" },
            { value: "pending", label: "Pending" },
            { value: "failed", label: "Failed" },
          ]}
          selected={downloadStatus}
          onChange={setDownloadStatus}
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

      {/* Table */}
      {loading && <LoadingSpinner message="Loading cookbooks…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {cookbooks.length === 0 ? (
            <EmptyState
              title="No cookbooks found"
              description="Adjust filters or wait for data collection."
            />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <SortableColumnHeader
                      label="Name"
                      field="name"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Version"
                      field="version"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    {!selectedOrg && <th>Organisation</th>}
                    <SortableColumnHeader
                      label="CookStyle"
                      field="compatibility"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <th>Test Kitchen</th>
                    <SortableColumnHeader
                      label="Status"
                      field="active"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Download"
                      field="download_status"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                  </tr>
                </thead>
                <tbody>
                  {cookbooks.map((cb) => (
                    <tr
                      key={cb.id}
                      className={cb.is_stale_cookbook ? "bg-purple-50/50" : ""}
                    >
                      <td>
                        <Link
                          to={`/cookbooks/${encodeURIComponent(cb.name)}`}
                          className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
                        >
                          {cb.name}
                        </Link>
                      </td>
                      <td>
                        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600">
                          {cb.version}
                        </span>
                      </td>
                      {!selectedOrg && (
                        <td>
                          <span className="text-sm text-gray-600">
                            {cb.organisation_name ?? "—"}
                          </span>
                        </td>
                      )}
                      <td>
                        <CookStyleBadge
                          status={cb.compatibility ?? "untested"}
                          size="sm"
                        />
                      </td>
                      <td>
                        {cb.tk_status ? (
                          <TKBadge status={cb.tk_status} size="sm" />
                        ) : (
                          <span className="text-xs text-gray-400">—</span>
                        )}
                      </td>
                      <td>
                        <div className="flex gap-1">
                          <StatusBadge
                            variant={cb.is_active ? "active" : "inactive"}
                            size="sm"
                          />
                          {cb.is_stale_cookbook && (
                            <StatusBadge
                              variant="stale"
                              label="Stale"
                              size="sm"
                            />
                          )}
                        </div>
                      </td>
                      <td>
                        <DownloadStatusBadge
                          status={cb.download_status}
                          error={cb.download_error}
                        />
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
