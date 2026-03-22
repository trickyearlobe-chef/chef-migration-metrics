import { useState, useEffect, useCallback } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { fetchGitRepos, fetchFilterTargetChefVersions } from "../api";
import type { GitRepoListItem, Pagination as PaginationType } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StatusBadge, CompatibilityBadge } from "../components/StatusBadge";
import { highestSemver } from "../semver";

// ---------------------------------------------------------------------------
// Git Repos list page — paginated table from GET /api/v1/git-repos showing
// name, git URL, test suite indicator, compatibility, head commit SHA,
// default branch, and last fetched time.
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

  // URL search params — read before filter state so initial values can be
  // seeded from query strings (e.g. links from the dashboard).
  const [searchParams, setSearchParams] = useSearchParams();

  // Filters
  const [nameFilter, setNameFilter] = useState(searchParams.get("name") || "");
  const [compatibility, setCompatibility] = useState(searchParams.get("compatibility") || "");
  const [tkStatus, setTkStatus] = useState(searchParams.get("tk_status") || "");
  const [page, setPage] = useState(1);
  const perPage = 50;

  // Target Chef versions loaded from backend config.
  const [targetVersions, setTargetVersions] = useState<string[]>([]);
  const [selectedTargetVersion, setSelectedTargetVersion] = useState<string>(searchParams.get("target_chef_version") || "");

  // Clear search params on mount so they don't persist on manual navigation.
  useEffect(() => {
    if (searchParams.has("compatibility") || searchParams.has("target_chef_version") || searchParams.has("name") || searchParams.has("tk_status")) {
      setSearchParams({}, { replace: true });
    }
  }, []); // run once on mount

  // Load target Chef versions once on mount.
  useEffect(() => {
    fetchFilterTargetChefVersions()
      .then((res) => {
        const versions = res.data ?? [];
        setTargetVersions(versions);
        if (versions.length > 0 && !selectedTargetVersion) {
          setSelectedTargetVersion(highestSemver(versions) ?? versions[0]);
        }
      })
      .catch(() => setTargetVersions([]));
  }, []); // intentionally run only on mount

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: {
      name?: string;
      compatibility?: string;
      tk_status?: string;
      target_chef_version?: string;
      page?: number;
      per_page?: number;
    } = {
      page,
      per_page: perPage,
    };
    if (nameFilter) filters.name = nameFilter;
    if (compatibility) filters.compatibility = compatibility;
    if (tkStatus) filters.tk_status = tkStatus;
    if (selectedTargetVersion) filters.target_chef_version = selectedTargetVersion;

    fetchGitRepos(filters)
      .then((res) => {
        setRepos(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [nameFilter, compatibility, tkStatus, selectedTargetVersion, page]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => { setPage(1); }, [nameFilter, compatibility, tkStatus, selectedTargetVersion]);

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
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Compatibility</label>
          <select
            value={compatibility}
            onChange={(e) => setCompatibility(e.target.value)}
            className="block w-40 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">All</option>
            <option value="compatible">Compatible</option>
            <option value="incompatible">Incompatible</option>
            <option value="untested">Untested</option>
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">TK Status</label>
          <select
            value={tkStatus}
            onChange={(e) => setTkStatus(e.target.value)}
            className="block w-40 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">All</option>
            <option value="passed">Passed</option>
            <option value="failed">Failed</option>
            <option value="timed_out">Timed Out</option>
            <option value="untested">Untested</option>
          </select>
        </div>
        {targetVersions.length > 1 && (
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-500">Target Version</label>
            <select
              value={selectedTargetVersion}
              onChange={(e) => setSelectedTargetVersion(e.target.value)}
              className="block w-36 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            >
              {targetVersions.map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
            </select>
          </div>
        )}
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
                    <th>Compatibility</th>
                    <th>TK Status</th>
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
                        <CompatibilityBadge
                          status={repo.compatibility ?? "untested"}
                          size="sm"
                        />
                      </td>
                      <td>
                        <StatusBadge
                          variant={repo.tk_status === "passed" ? "compatible" : repo.tk_status === "failed" ? "incompatible" : repo.tk_status === "timed_out" ? "incompatible" : "untested"}
                          label={repo.tk_status === "timed_out" ? "Timed Out" : (repo.tk_status ?? "untested")}
                          size="sm"
                        />
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
