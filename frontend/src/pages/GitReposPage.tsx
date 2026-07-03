import { useState, useEffect, useCallback, useMemo } from "react";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { Link, useSearchParams } from "react-router-dom";
import { useSort } from "../hooks/useSort";
import { useTargetChefVersion } from "../hooks/useTargetChefVersion";
import { SortableColumnHeader } from "../components/SortableColumnHeader";
import { FilterInput } from "../components/FilterInputs";
import { FilterMultiCheckbox } from "../components/FilterMultiCheckbox";
import { fetchGitRepos } from "../api";
import type { GitRepoFilterQuery } from "../api/client";
import type {
  GitRepoListItem,
  Pagination as PaginationType,
  ExportParams,
} from "../types";
import { ExportButton } from "../components/ExportButton";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StatusBadge, CookStyleStatusBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Git Repos list page — paginated table from GET /api/v1/git-repos showing
// name, git URL, test suite indicator, compatibility, head commit SHA,
// default branch, and last fetched time.
// ---------------------------------------------------------------------------

/** Truncate a string to `max` characters, appending "…" when clipped. */
function truncate(value: string, max: number): string {
  return value.length > max ? value.slice(0, max) + "…" : value;
}

/** Render a coloured clone-status pill with optional error tooltip. */
function CloneStatusBadge({
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
    ok: "Cloned",
    failed: "Missing",
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

/** Format an ISO date string into a human-friendly local representation. */
function formatDate(iso?: string): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

export function GitReposPage() {
  const [repos, setRepos] = useState<GitRepoListItem[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // URL search params — read before filter state so initial values can be
  // seeded from query strings (e.g. links from the dashboard).
  const [searchParams, setSearchParams] = useSearchParams();

  // Filters
  const [nameFilter, setNameFilter] = useState(searchParams.get("name") || "");
  const [cookstyleStatus, setCookstyleStatus] = useState<string[]>(
    searchParams.get("cookstyle_status")?.split(",").filter(Boolean) ?? [],
  );
  const [tkStatus, setTkStatus] = useState<string[]>(
    searchParams.get("tk_status")?.split(",").filter(Boolean) ?? [],
  );
  const [cloneStatus, setCloneStatus] = useState<string[]>(
    searchParams.get("clone_status")?.split(",").filter(Boolean) ?? [],
  );
  const [kitchenFilter, setKitchenFilter] = useState<string[]>(
    searchParams.get("has_test_suite")?.split(",").filter(Boolean) ?? [],
  );
  const [page, setPage] = useState(1);
  const perPage = DEFAULT_PAGE_SIZE;

  // Sort state — default to name ascending.
  const { sortField, sortOrder, handleSort } = useSort<
    "name" | "has_test_suite" | "compatibility" | "tk_status" | "last_fetched" | "git_url" | "clone_status"
  >({
    defaultField: "name",
    defaultOrder: "asc",
    descendingFields: ["has_test_suite"],
  });

  // Target Chef versions loaded from backend config.
  const { selectedVersion: selectedTargetVersion } = useTargetChefVersion({
    initialVersion: searchParams.get("target_chef_version") || undefined,
  });

  // Clear search params on mount so they don't persist on manual navigation.
  useEffect(() => {
    if (
      searchParams.has("cookstyle_status") ||
      searchParams.has("target_chef_version") ||
      searchParams.has("name") ||
      searchParams.has("tk_status") ||
      searchParams.has("clone_status") ||
      searchParams.has("has_test_suite")
    ) {
      setSearchParams({}, { replace: true });
    }
  }, []); // run once on mount

  // The active list filter/sort without pagination — shared by the list fetch
  // and the Export button so an export matches the visible list.
  const listQuery = useMemo<GitRepoFilterQuery>(() => {
    const q: GitRepoFilterQuery = {};
    if (nameFilter) q.name = nameFilter;
    if (cookstyleStatus.length > 0)
      q.cookstyle_status = cookstyleStatus.join(",");
    if (tkStatus.length > 0) q.tk_status = tkStatus.join(",");
    if (cloneStatus.length > 0) q.clone_status = cloneStatus.join(",");
    if (kitchenFilter.length > 0) q.has_test_suite = kitchenFilter.join(",");
    if (selectedTargetVersion) q.target_chef_version = selectedTargetVersion;
    if (sortField) q.sort = sortField;
    if (sortOrder) q.order = sortOrder;
    return q;
  }, [
    nameFilter,
    cookstyleStatus,
    tkStatus,
    cloneStatus,
    kitchenFilter,
    selectedTargetVersion,
    sortField,
    sortOrder,
  ]);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    fetchGitRepos({ ...listQuery, page, per_page: perPage })
      .then((res) => {
        setRepos(res.data ?? []);
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
    nameFilter,
    cookstyleStatus,
    tkStatus,
    cloneStatus,
    kitchenFilter,
    selectedTargetVersion,
    sortField,
    sortOrder,
  ]);

  // Count active filters for the clear button.
  const activeFilterCount = [
    nameFilter ? 1 : 0,
    cookstyleStatus.length > 0 ? 1 : 0,
    tkStatus.length > 0 ? 1 : 0,
    cloneStatus.length > 0 ? 1 : 0,
    kitchenFilter.length > 0 ? 1 : 0,
  ].reduce((a, b) => a + b, 0);

  const clearFilters = () => {
    setNameFilter("");
    setCookstyleStatus([]);
    setTkStatus([]);
    setCloneStatus([]);
    setKitchenFilter([]);
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-xl font-bold text-gray-800">Git Repos</h2>
        <ExportButton
          exportType="git_repos"
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
          label="TK Status"
          options={[
            { value: "passed", label: "Passed" },
            { value: "failed", label: "Failed" },
            { value: "timed_out", label: "Timed Out" },
            { value: "untested", label: "Untested" },
          ]}
          selected={tkStatus}
          onChange={setTkStatus}
        />
        <FilterMultiCheckbox
          label="Clone Status"
          options={[
            { value: "ok", label: "Cloned" },
            { value: "failed", label: "Failed" },
            { value: "pending", label: "Pending" },
          ]}
          selected={cloneStatus}
          onChange={setCloneStatus}
        />
        <FilterMultiCheckbox
          label="Kitchen"
          options={[
            { value: "yes", label: "Yes" },
            { value: "no", label: "No" },
          ]}
          selected={kitchenFilter}
          onChange={setKitchenFilter}
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
      {loading && <LoadingSpinner message="Loading git repos…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {repos.length === 0 ? (
            <EmptyState
              title="No git repos found"
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
                      label="Git URL"
                      field="git_url"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Clone"
                      field="clone_status"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Test Suite"
                      field="has_test_suite"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="CookStyle"
                      field="compatibility"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      tooltip="CookStyle — static analysis for Chef cookbook compatibility"
                    />
                    <SortableColumnHeader
                      label="TK Status"
                      field="tk_status"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      tooltip="Test Kitchen — integration test results"
                    />
                    <th>TK Results</th>
                    <th>Head Commit</th>
                    <th>Default Branch</th>
                    <SortableColumnHeader
                      label="Last Fetched"
                      field="last_fetched"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                  </tr>
                </thead>
                <tbody>
                  {repos.map((repo) => (
                    <tr key={repo.id}>
                      <td>
                        <Link
                          to={`/git-repos/${encodeURIComponent(repo.name)}`}
                          className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
                        >
                          {repo.name}
                        </Link>
                      </td>
                      <td>
                        <span
                          className="text-xs text-gray-500"
                          title={repo.git_repo_url}
                        >
                          {truncate(repo.git_repo_url, 48)}
                        </span>
                      </td>
                      <td>
                        <CloneStatusBadge
                          status={repo.clone_status}
                          error={repo.clone_error}
                        />
                      </td>
                      <td>
                        {repo.has_test_suite ? (
                          <StatusBadge
                            variant="compatible"
                            label="Yes"
                            size="sm"
                          />
                        ) : (
                          <StatusBadge
                            variant="untested"
                            label="No"
                            size="sm"
                          />
                        )}
                      </td>
                      <td>
                        <CookStyleStatusBadge
                          status={repo.cookstyle_status ?? "untested"}
                          size="sm"
                        />
                      </td>
                      <td>
                        <StatusBadge
                          variant={
                            repo.tk_status === "passed"
                              ? "compatible"
                              : repo.tk_status === "partial"
                                ? "warning"
                                : repo.tk_status === "failed"
                                  ? "incompatible"
                                  : repo.tk_status === "timed_out"
                                    ? "incompatible"
                                    : "untested"
                          }
                          label={
                            repo.tk_status === "timed_out"
                              ? "Timed Out"
                              : repo.tk_status === "partial"
                                ? "Partial"
                                : (repo.tk_status ?? "untested")
                          }
                          size="sm"
                        />
                      </td>
                      <td>
                        <span className="text-xs text-gray-600">
                          {repo.tk_total != null && repo.tk_total > 0
                            ? `${repo.tk_passed ?? 0}/${repo.tk_total}`
                            : "—"}
                        </span>
                      </td>
                      <td>
                        <span className="font-mono text-xs text-gray-600">
                          {repo.head_commit_sha
                            ? truncate(repo.head_commit_sha, 8)
                            : "—"}
                        </span>
                      </td>
                      <td>
                        <span className="text-xs text-gray-600">
                          {repo.default_branch ?? "—"}
                        </span>
                      </td>
                      <td className="text-xs text-gray-400">
                        {formatDate(repo.last_fetched_at)}
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
