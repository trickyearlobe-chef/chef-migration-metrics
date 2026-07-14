import { useState, useEffect, useCallback, useMemo } from "react";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { Link, useSearchParams } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import { useSort } from "../hooks/useSort";
import { useTargetChefVersion } from "../hooks/useTargetChefVersion";
import { SortableColumnHeader } from "../components/SortableColumnHeader";
import { FilterInput } from "../components/FilterInputs";
import { FilterMultiCheckbox } from "../components/FilterMultiCheckbox";
import { fetchCookbooks, type CookbookFilterQuery } from "../api";
import type {
  CookbookListItem,
  Pagination as PaginationType,
  ExportParams,
  SavedFilterParams,
} from "../types";
import { SavedFilterBar } from "../components/SavedFilterBar";
import {
  cookbookStateToParams,
  paramsToCookbookState,
} from "./listSavedFilters";
import { ExportButton } from "../components/ExportButton";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StatusBadge, CookStyleStatusBadge, TKBadge } from "../components/StatusBadge";

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
  const [cookstyleStatus, setCookstyleStatus] = useState<string[]>(
    searchParams.get("cookstyle_status")?.split(",").filter(Boolean) ?? [],
  );
  const [downloadStatus, setDownloadStatus] = useState<string[]>(
    searchParams.get("download_status")?.split(",").filter(Boolean) ?? [],
  );
  const [tkStatus, setTkStatus] = useState<string[]>(
    searchParams.get("tk_status")?.split(",").filter(Boolean) ?? [],
  );
  const [page, setPage] = useState(1);
  const perPage = DEFAULT_PAGE_SIZE;

  // Sort state — default to name ascending.
  const { sortField, sortOrder, handleSort } = useSort({
    defaultField: "name",
    defaultOrder: "asc",
    descendingFields: ["version", "compatibility", "active", "download_status", "tk_status"],
  });

  // Target Chef versions loaded from backend config.
  const { selectedVersion: selectedTargetVersion } = useTargetChefVersion({
    initialVersion: searchParams.get("target_chef_version") || undefined,
  });

  // Clear search params on mount so they don't persist on manual navigation.
  useEffect(() => {
    if (
      searchParams.has("cookstyle_status") ||
      searchParams.has("active") ||
      searchParams.has("name") ||
      searchParams.has("target_chef_version") ||
      searchParams.has("download_status") ||
      searchParams.has("tk_status")
    ) {
      setSearchParams({}, { replace: true });
    }
  }, []); // run once on mount

  // The active list filter/sort without pagination — shared by the list fetch
  // and the Export button so an export matches the visible list.
  const listQuery = useMemo<CookbookFilterQuery>(() => {
    const q: CookbookFilterQuery = {};
    if (selectedOrg) q.organisation = selectedOrg;
    if (active.length > 0) q.active = active.join(",");
    if (nameFilter) q.name = nameFilter;
    if (cookstyleStatus.length > 0)
      q.cookstyle_status = cookstyleStatus.join(",");
    if (downloadStatus.length > 0)
      q.download_status = downloadStatus.join(",");
    if (tkStatus.length > 0) q.tk_status = tkStatus.join(",");
    if (selectedTargetVersion) q.target_chef_version = selectedTargetVersion;
    if (sortField) q.sort = sortField;
    if (sortOrder) q.order = sortOrder;
    return q;
  }, [
    selectedOrg,
    active,
    nameFilter,
    cookstyleStatus,
    downloadStatus,
    tkStatus,
    selectedTargetVersion,
    sortField,
    sortOrder,
  ]);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    fetchCookbooks({ ...listQuery, page, per_page: perPage })
      .then((res) => {
        setCookbooks(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [listQuery, page, perPage]);

  useEffect(() => {
    load();
  }, [load]);
  useEffect(() => {
    setPage(1);
  }, [
    selectedOrg,
    active,
    nameFilter,
    cookstyleStatus,
    downloadStatus,
    tkStatus,
    selectedTargetVersion,
    sortField,
    sortOrder,
  ]);

  // Count active filters for the clear button.
  const activeFilterCount =
    (nameFilter ? 1 : 0) +
    (active.length > 0 ? 1 : 0) +
    (cookstyleStatus.length > 0 ? 1 : 0) +
    (downloadStatus.length > 0 ? 1 : 0) +
    (tkStatus.length > 0 ? 1 : 0);

  const clearFilters = () => {
    setNameFilter("");
    setActive([]);
    setCookstyleStatus([]);
    setDownloadStatus([]);
    setTkStatus([]);
  };

  // The current selection in the vocabulary a saved filter stores. Sort, page and
  // the global lens are deliberately not in here.
  const currentFilterParams = useMemo<SavedFilterParams>(
    () =>
      cookbookStateToParams({
        nameFilter,
        active,
        cookstyleStatus,
        downloadStatus,
        tkStatus,
      }),
    [nameFilter, active, cookstyleStatus, downloadStatus, tkStatus],
  );

  /**
   * Apply a saved selection: set the filter state and nothing else. Sort and the
   * global lens are how the operator is reading the list, not part of the named
   * cohort — only `page` resets, as it does for any filter change.
   */
  const applySavedFilter = useCallback((params: SavedFilterParams) => {
    const next = paramsToCookbookState(params);
    setNameFilter(next.nameFilter);
    setActive(next.active);
    setCookstyleStatus(next.cookstyleStatus);
    setDownloadStatus(next.downloadStatus);
    setTkStatus(next.tkStatus);
    setPage(1);
  }, []);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-xl font-bold text-gray-800">Server Cookbooks</h2>
        <ExportButton
          exportType="cookbooks"
          params={listQuery as ExportParams}
          label="Export"
        />
      </div>

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
          label="CookStyle"
          options={[
            { value: "ready", label: "Ready" },
            { value: "needs_review", label: "Needs review" },
            { value: "blocked", label: "Blocked" },
            { value: "untested", label: "Untested" },
          ]}
          selected={cookstyleStatus}
          onChange={setCookstyleStatus}
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
        <FilterMultiCheckbox
          label="Test Kitchen"
          options={[
            { value: "passed", label: "Passed" },
            { value: "failed", label: "Failed" },
            { value: "partial", label: "Partial" },
            { value: "untested", label: "Untested" },
            { value: "no_repo", label: "No Git Repo" },
          ]}
          selected={tkStatus}
          onChange={setTkStatus}
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
        <SavedFilterBar
          view="cookbooks"
          currentParams={currentFilterParams}
          onApply={applySavedFilter}
        />
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
                      tooltip="CookStyle — static analysis for Chef cookbook compatibility"
                    />
                    <SortableColumnHeader
                      label="Test Kitchen"
                      field="tk_status"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      tooltip="Test Kitchen — integration test results from matching Git repository"
                    />
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
                        <CookStyleStatusBadge
                          status={cb.cookstyle_status ?? "untested"}
                          size="sm"
                        />
                      </td>
                      <td>
                        {cb.tk_status ? (
                          <TKBadge status={cb.tk_status} size="sm" />
                        ) : (
                          <span className="text-xs text-gray-400" title="No matching Git repository found for this cookbook">—</span>
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
