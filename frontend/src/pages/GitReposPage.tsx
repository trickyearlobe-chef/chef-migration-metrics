import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { fetchGitRepos } from "../api";
import type { GitRepoListItem, Pagination as PaginationType } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StatusBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Git Repos list page — paginated table from GET /api/v1/git-repos showing
// name, git URL, test suite indicator, head commit SHA, default branch, and
// last fetched time.
// ---------------------------------------------------------------------------

/** Truncate a string to `max` characters, appending "…" when clipped. */
function truncate(value: string, max: number): string {
  return value.length > max ? value.slice(0, max) + "…" : value;
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

  // Filters
  const [nameFilter, setNameFilter] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 50;

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: { name?: string; page?: number; per_page?: number } = {
      page,
      per_page: perPage,
    };
    if (nameFilter) filters.name = nameFilter;

    fetchGitRepos(filters)
      .then((res) => {
        setRepos(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [nameFilter, page]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => { setPage(1); }, [nameFilter]);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-bold text-gray-800">Git Repos</h2>

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
      </div>

      {/* Table */}
      {loading && <LoadingSpinner message="Loading git repos…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {repos.length === 0 ? (
            <EmptyState title="No git repos found" description="Adjust filters or wait for data collection." />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Git URL</th>
                    <th>Test Suite</th>
                    <th>Head Commit</th>
                    <th>Default Branch</th>
                    <th>Last Fetched</th>
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
                        {repo.has_test_suite ? (
                          <StatusBadge variant="compatible" label="Yes" size="sm" />
                        ) : (
                          <StatusBadge variant="untested" label="No" size="sm" />
                        )}
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
