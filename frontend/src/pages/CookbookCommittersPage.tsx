import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import {
  fetchCookbookCommitters,
  assignCookbookCommitters,
  type CommitterFilterQuery,
} from "../api";
import type {
  CookbookCommittersResponse,
  GitRepoCommitter,
  Pagination as PaginationType,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";

// ---------------------------------------------------------------------------
// Date filter options — the `value` is an ISO date string sent as `since`.
// ---------------------------------------------------------------------------

const DATE_FILTER_OPTIONS: { label: string; months: number | null }[] = [
  { label: "All time", months: null },
  { label: "Last 6 months", months: 6 },
  { label: "Last year", months: 12 },
  { label: "Last 2 years", months: 24 },
];

function sinceDate(months: number | null): string | undefined {
  if (months === null) return undefined;
  const d = new Date();
  d.setMonth(d.getMonth() - months);
  return d.toISOString();
}

// ---------------------------------------------------------------------------
// Sort arrow indicator
// ---------------------------------------------------------------------------

function SortIndicator({
  column,
  activeSort,
  activeOrder,
}: {
  column: string;
  activeSort: string;
  activeOrder: "asc" | "desc";
}) {
  if (column !== activeSort) {
    return <span className="ml-1 text-gray-300">↕</span>;
  }
  return (
    <span className="ml-1">
      {activeOrder === "asc" ? "↑" : "↓"}
    </span>
  );
}

// ---------------------------------------------------------------------------
// CookbookCommittersPage
// ---------------------------------------------------------------------------

export function CookbookCommittersPage() {
  const { name } = useParams<{ name: string }>();

  // Data
  const [response, setResponse] = useState<CookbookCommittersResponse | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters & sorting
  const [page, setPage] = useState(1);
  const perPage = 25;
  const [sort, setSort] = useState("last_commit_at");
  const [order, setOrder] = useState<"asc" | "desc">("desc");
  const [sinceMonths, setSinceMonths] = useState<number | null>(null);

  // Selection
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // Assign action
  const [assigning, setAssigning] = useState(false);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);
  const [assignError, setAssignError] = useState<string | null>(null);

  const committers: GitRepoCommitter[] = response?.data ?? [];
  const pagination: PaginationType | null = response?.pagination ?? null;

  // -----------------------------------------------------------------------
  // Data loading
  // -----------------------------------------------------------------------

  const load = useCallback(() => {
    if (!name) return;
    setLoading(true);
    setError(null);

    const filters: CommitterFilterQuery = {
      page,
      per_page: perPage,
      sort,
      order,
      since: sinceDate(sinceMonths),
    };

    fetchCookbookCommitters(name, filters)
      .then((res) => {
        setResponse(res);
        // Clear selection when data changes
        setSelected(new Set());
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name, page, perPage, sort, order, sinceMonths]);

  useEffect(() => {
    load();
  }, [load]);

  // Reset to page 1 when sort / filter changes
  useEffect(() => {
    setPage(1);
  }, [sort, order, sinceMonths]);

  // -----------------------------------------------------------------------
  // Sorting
  // -----------------------------------------------------------------------

  const handleSort = (column: string) => {
    if (sort === column) {
      setOrder((prev) => (prev === "asc" ? "desc" : "asc"));
    } else {
      setSort(column);
      setOrder("desc");
    }
  };

  // -----------------------------------------------------------------------
  // Selection
  // -----------------------------------------------------------------------

  const allSelected =
    committers.length > 0 && committers.every((c) => selected.has(c.id));

  const toggleAll = () => {
    if (allSelected) {
      setSelected(new Set());
    } else {
      setSelected(new Set(committers.map((c) => c.id)));
    }
  };

  const toggleOne = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  // -----------------------------------------------------------------------
  // Assign as owners
  // -----------------------------------------------------------------------

  const handleAssign = async () => {
    if (!name || selected.size === 0) return;
    setAssigning(true);
    setAssignError(null);
    setSuccessMsg(null);

    const selectedCommitters = committers.filter((c) => selected.has(c.id));
    const body = {
      committers: selectedCommitters.map((c) => ({
        author_email: c.author_email,
        owner_name: c.author_email.split("@")[0],
        display_name: c.author_name,
      })),
    };

    try {
      const res = await assignCookbookCommitters(name, body);
      setSuccessMsg(
        `Created ${res.owners_created} owner(s), ${res.assignments_created} assignment(s), ${res.skipped} skipped.`,
      );
      setSelected(new Set());
      load();
    } catch (e: unknown) {
      const message =
        e instanceof Error ? e.message : "Failed to assign committers.";
      setAssignError(message);
    } finally {
      setAssigning(false);
    }
  };

  // -----------------------------------------------------------------------
  // Render
  // -----------------------------------------------------------------------

  if (loading && !response) {
    return <LoadingSpinner message="Loading committers…" />;
  }

  if (error && !response) {
    return <ErrorAlert message={error} onRetry={load} />;
  }

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <Link to="/cookbooks" className="hover:text-blue-600 hover:underline">
          Cookbooks
        </Link>
        <span className="mx-1">/</span>
        <Link
          to={`/cookbooks/${encodeURIComponent(name ?? "")}`}
          className="hover:text-blue-600 hover:underline"
        >
          {name}
        </Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">Committers</span>
      </nav>

      {/* Heading */}
      <div>
        <h2 className="text-xl font-bold text-gray-800">
          {name} — Committers
        </h2>
        {response?.git_repo_url && (
          <p className="mt-1 text-sm text-gray-500">
            Repository:{" "}
            <a
              href={response.git_repo_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-600 hover:text-blue-800 hover:underline"
            >
              {response.git_repo_url}
            </a>
          </p>
        )}
      </div>

      {/* Success banner */}
      {successMsg && (
        <div className="rounded-md border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          {successMsg}
        </div>
      )}

      {/* Assign error banner */}
      {assignError && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          {assignError}
        </div>
      )}

      {/* Toolbar: date filter + assign button */}
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">
            Activity Period
          </label>
          <select
            value={sinceMonths === null ? "" : String(sinceMonths)}
            onChange={(e) =>
              setSinceMonths(
                e.target.value === "" ? null : Number(e.target.value),
              )
            }
            className="block w-44 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            {DATE_FILTER_OPTIONS.map((opt) => (
              <option
                key={opt.label}
                value={opt.months === null ? "" : String(opt.months)}
              >
                {opt.label}
              </option>
            ))}
          </select>
        </div>

        <button
          onClick={handleAssign}
          disabled={selected.size === 0 || assigning}
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {assigning ? "Assigning…" : `Assign as Owners (${selected.size})`}
        </button>
      </div>

      {/* Loading overlay for refreshes */}
      {loading && response && (
        <LoadingSpinner message="Refreshing…" />
      )}

      {/* Error on refresh */}
      {error && response && (
        <ErrorAlert message={error} onRetry={load} />
      )}

      {/* Table */}
      {!loading && committers.length === 0 ? (
        <EmptyState
          title="No committers found"
          description="Adjust the activity period filter or check that the repository has been scanned."
        />
      ) : (
        !loading && (
          <div className="card">
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th className="w-10">
                      <input
                        type="checkbox"
                        checked={allSelected}
                        onChange={toggleAll}
                        className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                      />
                    </th>
                    <th>Author Name</th>
                    <th>Email</th>
                    <th>
                      <button
                        type="button"
                        className="inline-flex items-center font-semibold hover:text-blue-600"
                        onClick={() => handleSort("commit_count")}
                      >
                        Commit Count
                        <SortIndicator
                          column="commit_count"
                          activeSort={sort}
                          activeOrder={order}
                        />
                      </button>
                    </th>
                    <th>First Commit</th>
                    <th>
                      <button
                        type="button"
                        className="inline-flex items-center font-semibold hover:text-blue-600"
                        onClick={() => handleSort("last_commit_at")}
                      >
                        Last Commit
                        <SortIndicator
                          column="last_commit_at"
                          activeSort={sort}
                          activeOrder={order}
                        />
                      </button>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {committers.map((c) => (
                    <tr key={c.id}>
                      <td>
                        <input
                          type="checkbox"
                          checked={selected.has(c.id)}
                          onChange={() => toggleOne(c.id)}
                          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                        />
                      </td>
                      <td className="text-sm text-gray-800">
                        {c.author_name}
                      </td>
                      <td className="text-sm text-gray-600">
                        {c.author_email}
                      </td>
                      <td className="text-sm text-gray-600">
                        {c.commit_count}
                      </td>
                      <td className="text-sm text-gray-500">
                        {new Date(c.first_commit_at).toLocaleDateString()}
                      </td>
                      <td className="text-sm text-gray-500">
                        {new Date(c.last_commit_at).toLocaleDateString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {pagination && (
              <Pagination pagination={pagination} onPageChange={setPage} />
            )}
          </div>
        )
      )}
    </div>
  );
}
