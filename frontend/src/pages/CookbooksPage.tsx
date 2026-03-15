import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import { fetchCookbooks, type CookbookFilterQuery } from "../api";
import type { CookbookListItem, Pagination as PaginationType } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StatusBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Cookbooks list page — paginated table from GET /api/v1/cookbooks showing
// name, version count, active/stale indicators, download status.
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

  // Filters
  const [active, setActive] = useState("");
  const [nameFilter, setNameFilter] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 50;

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: CookbookFilterQuery = {
      page,
      per_page: perPage,
    };
    if (selectedOrg) filters.organisation = selectedOrg;
    if (active) filters.active = active;
    if (nameFilter) filters.name = nameFilter;

    fetchCookbooks(filters)
      .then((res) => {
        setCookbooks(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedOrg, active, nameFilter, page]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => { setPage(1); }, [selectedOrg, active, nameFilter]);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-bold text-gray-800">Cookbooks</h2>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Name</label>
          <input
            type="text"
            value={nameFilter}
            onChange={(e) => setNameFilter(e.target.value)}
            placeholder="Filter by name"
            className="block w-40 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Active</label>
          <select
            value={active}
            onChange={(e) => setActive(e.target.value)}
            className="block w-32 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">All</option>
            <option value="true">Active</option>
            <option value="false">Inactive</option>
          </select>
        </div>
      </div>

      {/* Table */}
      {loading && <LoadingSpinner message="Loading cookbooks…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {cookbooks.length === 0 ? (
            <EmptyState title="No cookbooks found" description="Adjust filters or wait for data collection." />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Versions</th>
                    <th>Status</th>
                    <th>Download</th>
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
                          {cb.version_count === 1 ? "1 version" : `${cb.version_count} versions`}
                        </span>
                      </td>
                      <td>
                        <div className="flex gap-1">
                          <StatusBadge
                            variant={cb.is_active ? "active" : "inactive"}
                            size="sm"
                          />
                          {cb.is_stale_cookbook && (
                            <StatusBadge variant="stale" label="Stale" size="sm" />
                          )}
                        </div>
                      </td>
                      <td className="text-xs text-gray-400">
                        {cb.download_status || "—"}
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
