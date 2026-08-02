import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import {
  fetchGitRepoDetail,
  requestGitRepoRescan,
  resetGitRepo,
  fetchGitRepoFiles,
  fetchGitRepoFileContent,
  fetchGitRepoCommitters,
  assignGitRepoCommitters,
} from "../api";
import type {
  GitRepoDetailResponse,
  CookbookCommittersResponse,
  GitRepoCommitter,
  Pagination as PaginationType,
} from "../types";
import type { CommitterFilterQuery } from "../api/ownership";
import type { GitRepoFileEntry, GitRepoFileContentResponse } from "../api/git-repos";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StatusBadge, CookStyleStatusBadge } from "../components/StatusBadge";
import { GitKitchenSection } from "../components/GitKitchenSection";
import { TeamVerdictCard } from "../components/TeamVerdictCard";
import { SortableColumnHeader } from "../components/SortableColumnHeader";
import { useSort } from "../hooks/useSort";
import { SMALL_PAGE_SIZE } from "../constants";
import { GitRepoRemediationContent } from "./GitRepoRemediationPage";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function GitRepoDetailPage() {
  const { name } = useParams<{ name: string }>();

  const [data, setData] = useState<GitRepoDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [rescanning, setRescanning] = useState(false);
  const [rescanMsg, setRescanMsg] = useState<string | null>(null);
  const [resetting, setResetting] = useState(false);
  const [resetMsg, setResetMsg] = useState<string | null>(null);
  const [showResetConfirm, setShowResetConfirm] = useState(false);
  const [activeTab, setActiveTab] = useState<"overview" | "files" | "committers" | "kitchen" | "cookstyle">("overview");

  const load = useCallback(() => {
    if (!name) return;
    setLoading(true);
    setError(null);
    fetchGitRepoDetail(name)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name]);

  const handleRescan = useCallback(() => {
    if (!name) return;
    setRescanning(true);
    setRescanMsg(null);
    requestGitRepoRescan(name)
      .then((res) => {
        setRescanMsg(res.message);
        load();
      })
      .catch((e: Error) => setRescanMsg(`Rescan failed: ${e.message}`))
      .finally(() => setRescanning(false));
  }, [name, load]);

  const handleReset = useCallback(() => {
    if (!name) return;
    setResetting(true);
    setResetMsg(null);
    setShowResetConfirm(false);
    resetGitRepo(name)
      .then((res) => {
        setResetMsg(res.message);
        load();
      })
      .catch((e: Error) => setResetMsg(`Reset failed: ${e.message}`))
      .finally(() => setResetting(false));
  }, [name, load]);

  useEffect(() => {
    load();
  }, [load]);

  if (loading) return <LoadingSpinner message="Loading git repo detail…" />;
  if (error) return <ErrorAlert message={error} onRetry={load} />;
  if (!data) return <LoadingSpinner message="Loading git repo detail…" />;

  const hasGitRepos = data.git_repos && data.git_repos.length > 0;

  return (
    <div className="space-y-6">
      <nav className="text-sm text-gray-500">
        <Link to="/git-repos" className="hover:text-blue-600 hover:underline">
          Git Repos
        </Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">{data.name}</span>
      </nav>

      <div className="flex items-center gap-4">
        <h2 className="text-xl font-bold text-gray-800">{data.name}</h2>
        <button
          onClick={handleRescan}
          disabled={rescanning}
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
          title="Invalidate cached CookStyle results and trigger an immediate rescan"
        >
          {rescanning ? "Requesting…" : "Rescan CookStyle"}
        </button>
        <button
          onClick={() => setShowResetConfirm(true)}
          disabled={resetting}
          className="inline-flex items-center gap-1.5 rounded-md border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 shadow-sm hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
          title="Remove all git data for this repo — it will be re-cloned on the next collection cycle"
        >
          {resetting ? "Resetting…" : "Reset Git"}
        </button>
      </div>

      {showResetConfirm && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          <p className="font-medium">
            Are you sure you want to reset git data for "{data.name}"?
          </p>
          <p className="mt-1 text-red-600">
            This will delete all git-sourced repo data, committer data, and the
            local clone. The repo will be re-cloned from the currently
            configured git base URLs on the next collection cycle.
          </p>
          <div className="mt-3 flex gap-2">
            <button
              onClick={handleReset}
              className="rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-red-700"
            >
              Yes, Reset Git
            </button>
            <button
              onClick={() => setShowResetConfirm(false)}
              className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {rescanMsg && (
        <div className="rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
          {rescanMsg}
        </div>
      )}

      {resetMsg && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          {resetMsg}
        </div>
      )}

      {/* Tab navigation */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-4">
          <button
            onClick={() => setActiveTab("overview")}
            className={`border-b-2 px-1 py-2 text-sm font-medium ${
              activeTab === "overview"
                ? "border-blue-500 text-blue-600"
                : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700"
            }`}
          >
            Overview
          </button>
          <button
            onClick={() => setActiveTab("files")}
            className={`border-b-2 px-1 py-2 text-sm font-medium ${
              activeTab === "files"
                ? "border-blue-500 text-blue-600"
                : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700"
            }`}
          >
            Files
          </button>
          <button
            onClick={() => setActiveTab("committers")}
            className={`border-b-2 px-1 py-2 text-sm font-medium ${
              activeTab === "committers"
                ? "border-blue-500 text-blue-600"
                : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700"
            }`}
          >
            Committers
          </button>
          <button
            onClick={() => setActiveTab("kitchen")}
            className={`border-b-2 px-1 py-2 text-sm font-medium ${
              activeTab === "kitchen"
                ? "border-blue-500 text-blue-600"
                : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700"
            }`}
          >
            Kitchen
          </button>
          <button
            onClick={() => setActiveTab("cookstyle")}
            className={`border-b-2 px-1 py-2 text-sm font-medium ${
              activeTab === "cookstyle"
                ? "border-blue-500 text-blue-600"
                : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700"
            }`}
          >
            CookStyle
          </button>
        </nav>
      </div>

      {activeTab === "overview" && (
        <>
          {!hasGitRepos ? (
            <EmptyState title="No git repo entries found" />
          ) : (
            (() => {
              const gd = data.git_repos[0];
              const gr = gd.git_repo;
              return (
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                  {/* Git Details card */}
                  <div className="card">
                    <h4 className="mb-3 text-sm font-semibold text-gray-700">Repository</h4>
                    {gr.clone_status === "failed" && (
                      <div className="mb-3 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-2">
                        <span className="mt-0.5 shrink-0 text-red-500">⚠</span>
                        <div className="text-xs text-red-700">
                          <p className="font-medium">Clone failed</p>
                          {gr.clone_error && <p className="mt-0.5">{gr.clone_error}</p>}
                        </div>
                      </div>
                    )}
                    <dl className="space-y-2 text-xs">
                      {gr.git_repo_url && (
                        <div>
                          <dt className="text-gray-500">URL</dt>
                          <dd className="truncate font-mono text-gray-700" title={gr.git_repo_url}>{gr.git_repo_url}</dd>
                        </div>
                      )}
                      {gr.default_branch && (
                        <div>
                          <dt className="text-gray-500">Branch</dt>
                          <dd><code className="rounded bg-gray-100 px-1 py-0.5">{gr.default_branch}</code></dd>
                        </div>
                      )}
                      {gr.head_commit_sha && (
                        <div>
                          <dt className="text-gray-500">HEAD</dt>
                          <dd><code className="rounded bg-gray-100 px-1 py-0.5" title={gr.head_commit_sha}>{gr.head_commit_sha.substring(0, 12)}</code></dd>
                        </div>
                      )}
                      <div>
                        <dt className="text-gray-500">Status</dt>
                        <dd className="flex items-center gap-2">
                          {gr.clone_status === "failed" ? (
                            <StatusBadge variant="incompatible" label="Missing" size="sm" />
                          ) : gr.clone_status === "pending" ? (
                            <StatusBadge variant="untested" label="Pending" size="sm" />
                          ) : (
                            <StatusBadge variant="compatible" label="Cloned" size="sm" />
                          )}
                          {gr.clone_status === "ok" && (
                            gr.has_test_suite
                              ? <StatusBadge variant="compatible" label="Has Test Suite" size="sm" />
                              : <StatusBadge variant="untested" label="No Test Suite" size="sm" />
                          )}
                        </dd>
                      </div>
                      {gr.last_fetched_at && (
                        <div>
                          <dt className="text-gray-500">Last Fetched</dt>
                          <dd className="text-gray-700">{new Date(gr.last_fetched_at).toLocaleString()}</dd>
                        </div>
                      )}
                    </dl>
                  </div>

                  {/* CookStyle Summary card */}
                  <button
                    onClick={() => setActiveTab("cookstyle")}
                    className="card text-left hover:ring-1 hover:ring-blue-200 transition-shadow"
                  >
                    <h4 className="mb-3 text-sm font-semibold text-gray-700">CookStyle</h4>
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-gray-500">Status:</span>
                        <CookStyleStatusBadge
                          status={gd.git_repo.cookstyle_status ?? "untested"}
                          size="sm"
                        />
                      </div>
                      {gd.complexity && gd.complexity.length > 0 && (
                        <div className="flex items-center gap-2">
                          <span className="text-xs text-gray-500">Complexity:</span>
                          <StatusBadge
                            variant={
                              gd.complexity[0].complexity_label === "low"
                                ? "low"
                                : gd.complexity[0].complexity_label === "medium"
                                  ? "medium"
                                  : gd.complexity[0].complexity_label === "high"
                                    ? "high"
                                    : gd.complexity[0].complexity_label === "critical"
                                      ? "critical"
                                      : "unknown"
                            }
                            label={`${(gd.complexity[0].complexity_label ?? "unknown").charAt(0).toUpperCase() + (gd.complexity[0].complexity_label ?? "unknown").slice(1)} (${gd.complexity[0].complexity_score ?? 0})`}
                            size="sm"
                          />
                        </div>
                      )}
                      {gd.cookstyle && gd.cookstyle.length > 0 && (
                        <div className="text-xs text-gray-500">
                          Target: Chef {gd.cookstyle[0].target_chef_version}
                        </div>
                      )}
                    </div>
                    <p className="mt-3 text-xs font-medium text-blue-600">View Details →</p>
                  </button>

                  {/* Test Kitchen Summary card */}
                  <button
                    onClick={() => setActiveTab("kitchen")}
                    className="card text-left hover:ring-1 hover:ring-blue-200 transition-shadow"
                  >
                    <h4 className="mb-3 text-sm font-semibold text-gray-700">Test Kitchen</h4>
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-gray-500">Status:</span>
                        {gr.has_test_suite ? (
                          <StatusBadge
                            variant={
                              gd.tk_status === "passed"
                                ? "compatible"
                                : gd.tk_status === "partial"
                                  ? "warning"
                                  : gd.tk_status === "failed"
                                    ? "incompatible"
                                    : gd.tk_status === "timed_out"
                                      ? "incompatible"
                                      : "untested"
                            }
                            label={
                              gd.tk_status === "timed_out"
                                ? "Timed Out"
                                : gd.tk_status === "partial"
                                  ? "Partial"
                                  : gd.tk_status
                                    ? gd.tk_status.charAt(0).toUpperCase() + gd.tk_status.slice(1)
                                    : "Not Run"
                            }
                            size="sm"
                          />
                        ) : (
                          <StatusBadge variant="untested" label="No Test Suite" size="sm" />
                        )}
                      </div>
                      {gd.tk_total != null && gd.tk_total > 0 && (
                        <div className="flex items-center gap-2">
                          <span className="text-xs text-gray-500">Results:</span>
                          <span className="text-sm font-medium text-gray-800">
                            {gd.tk_passed ?? 0} / {gd.tk_total} suites passed
                          </span>
                        </div>
                      )}
                    </div>
                    <p className="mt-3 text-xs font-medium text-blue-600">View Details →</p>
                  </button>

                  {/* A person's verdict, beside the two automated ones. It
                      outranks both, and this is where it gets recorded. */}
                  <TeamVerdictCard gitRepoName={gr.name} cookbookName={gr.name} />
                </div>
              );
            })()
          )}
        </>
      )}

      {activeTab === "files" && name && (
        <GitRepoFileBrowser repoName={name} />
      )}

      {activeTab === "committers" && name && (
        <GitRepoCommittersTab repoName={name} />
      )}

      {activeTab === "kitchen" && name && (
        <GitKitchenSection repoName={name} />
      )}

      {activeTab === "cookstyle" && name && (
        <GitRepoRemediationContent repoName={name} version="latest" />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Committers Tab Component
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

function GitRepoCommittersTab({ repoName }: { repoName: string }) {
  const [response, setResponse] = useState<CookbookCommittersResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [page, setPage] = useState(1);
  const perPage = SMALL_PAGE_SIZE;
  type CommitterSortField =
    | "author_name"
    | "author_email"
    | "commit_count"
    | "first_commit_at"
    | "last_commit_at";
  const {
    sortField: sort,
    sortOrder: order,
    handleSort,
  } = useSort<CommitterSortField>({
    defaultField: "last_commit_at",
    defaultOrder: "desc",
    descendingFields: ["commit_count", "first_commit_at", "last_commit_at"],
  });
  const [sinceMonths, setSinceMonths] = useState<number | null>(null);

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const key = (c: GitRepoCommitter) => c.author_email;

  const [assigning, setAssigning] = useState(false);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);
  const [assignError, setAssignError] = useState<string | null>(null);

  const committers: GitRepoCommitter[] = response?.data ?? [];
  const pagination: PaginationType | null = response?.pagination ?? null;

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: CommitterFilterQuery = {
      page,
      per_page: perPage,
      sort,
      order,
      since: sinceDate(sinceMonths),
    };

    fetchGitRepoCommitters(repoName, filters)
      .then((res) => {
        setResponse(res);
        const ownerIds = new Set(
          (res.data ?? []).filter((c) => c.is_owner).map((c) => key(c)),
        );
        setSelected(ownerIds);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [repoName, page, perPage, sort, order, sinceMonths]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    setPage(1);
  }, [sort, order, sinceMonths]);

  const allSelected =
    committers.length > 0 && committers.every((c) => selected.has(key(c)));

  const toggleAll = () => {
    if (allSelected) {
      setSelected(new Set());
    } else {
      setSelected(new Set(committers.map((c) => key(c))));
    }
  };

  const toggleOne = (c: GitRepoCommitter) => {
    const k = key(c);
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(k)) {
        next.delete(k);
      } else {
        next.add(k);
      }
      return next;
    });
  };

  const handleAssign = async () => {
    if (selected.size === 0) return;
    setAssigning(true);
    setAssignError(null);
    setSuccessMsg(null);

    const selectedCommitters = committers.filter((c) => selected.has(key(c)));
    const body = {
      committers: selectedCommitters.map((c) => ({
        author_email: c.author_email,
        owner_name: c.author_email.split("@")[0],
        display_name: c.author_name,
      })),
    };

    try {
      const res = await assignGitRepoCommitters(repoName, body);
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

  if (loading && !response) {
    return <LoadingSpinner message="Loading committers…" />;
  }

  if (error && !response) {
    return <ErrorAlert message={error} onRetry={load} />;
  }

  return (
    <div className="space-y-4">
      {successMsg && (
        <div className="rounded-md border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          {successMsg}
        </div>
      )}

      {assignError && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          {assignError}
        </div>
      )}

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

      {loading && response && <LoadingSpinner message="Refreshing…" />}
      {error && response && <ErrorAlert message={error} onRetry={load} />}

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
                    <SortableColumnHeader
                      label="Author Name"
                      field="author_name"
                      currentField={sort}
                      currentOrder={order}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Email"
                      field="author_email"
                      currentField={sort}
                      currentOrder={order}
                      onSort={handleSort}
                    />
                    <th>Owner</th>
                    <SortableColumnHeader
                      label="Commit Count"
                      field="commit_count"
                      currentField={sort}
                      currentOrder={order}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="First Commit"
                      field="first_commit_at"
                      currentField={sort}
                      currentOrder={order}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Last Commit"
                      field="last_commit_at"
                      currentField={sort}
                      currentOrder={order}
                      onSort={handleSort}
                    />
                  </tr>
                </thead>
                <tbody>
                  {committers.map((c) => (
                    <tr key={key(c)} className={c.is_owner ? "bg-blue-50" : ""}>
                      <td>
                        <input
                          type="checkbox"
                          checked={selected.has(key(c))}
                          onChange={() => toggleOne(c)}
                          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                        />
                      </td>
                      <td className="text-sm text-gray-800">{c.author_name}</td>
                      <td className="text-sm text-gray-600">
                        {c.author_email}
                      </td>
                      <td className="text-sm">
                        {c.is_owner && (
                          <span className="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800">
                            Owner
                          </span>
                        )}
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

// ---------------------------------------------------------------------------
// File Browser Component — IDE-style tree with expand/collapse
// ---------------------------------------------------------------------------

interface TreeNode {
  name: string;
  path: string;
  type: "file" | "dir";
  size?: number;
  children?: TreeNode[];
  loaded?: boolean;
  expanded?: boolean;
}

function GitRepoFileBrowser({ repoName }: { repoName: string }) {
  const [tree, setTree] = useState<TreeNode[]>([]);
  const [loadingRoot, setLoadingRoot] = useState(true);
  const [treeError, setTreeError] = useState<string | null>(null);

  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<GitRepoFileContentResponse | null>(null);
  const [loadingFile, setLoadingFile] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<"rendered" | "raw">("rendered");

  // Load root directory on mount.
  useEffect(() => {
    setLoadingRoot(true);
    setTreeError(null);
    fetchGitRepoFiles(repoName, ".")
      .then((entries) => {
        setTree(
          entriesToNodes(entries, ".").sort(sortNodes),
        );
      })
      .catch((e: Error) => setTreeError(e.message))
      .finally(() => setLoadingRoot(false));
  }, [repoName]);

  function entriesToNodes(entries: GitRepoFileEntry[], parentPath: string): TreeNode[] {
    return entries.map((e) => ({
      name: e.name,
      path: parentPath === "." ? e.name : `${parentPath}/${e.name}`,
      type: e.type,
      size: e.size,
      children: e.type === "dir" ? undefined : undefined,
      loaded: false,
      expanded: false,
    }));
  }

  function sortNodes(a: TreeNode, b: TreeNode): number {
    // Directories first, then alphabetical.
    if (a.type !== b.type) return a.type === "dir" ? -1 : 1;
    return a.name.localeCompare(b.name);
  }

  // Toggle a directory node: expand (load children if needed) or collapse.
  const toggleDir = useCallback(
    (targetPath: string) => {
      setTree((prev) => updateTree(prev, targetPath));
    },
    [repoName],
  );

  function updateTree(nodes: TreeNode[], targetPath: string): TreeNode[] {
    return nodes.map((node) => {
      if (node.path === targetPath && node.type === "dir") {
        if (node.expanded) {
          // Collapse.
          return { ...node, expanded: false };
        }
        if (node.loaded && node.children) {
          // Already loaded, just expand.
          return { ...node, expanded: true };
        }
        // Need to load children — mark as expanding and fetch.
        loadChildren(node.path);
        return { ...node, expanded: true, children: undefined, loaded: false };
      }
      if (node.children) {
        return { ...node, children: updateTree(node.children, targetPath) };
      }
      return node;
    });
  }

  function loadChildren(dirPath: string) {
    fetchGitRepoFiles(repoName, dirPath).then((entries) => {
      setTree((prev) =>
        insertChildren(prev, dirPath, entriesToNodes(entries, dirPath).sort(sortNodes)),
      );
    });
  }

  function insertChildren(nodes: TreeNode[], targetPath: string, children: TreeNode[]): TreeNode[] {
    return nodes.map((node) => {
      if (node.path === targetPath) {
        return { ...node, children, loaded: true, expanded: true };
      }
      if (node.children) {
        return { ...node, children: insertChildren(node.children, targetPath, children) };
      }
      return node;
    });
  }

  const selectFile = useCallback(
    (path: string) => {
      setSelectedFile(path);
      setViewMode("rendered");
      setLoadingFile(true);
      setFileError(null);
      setFileContent(null);
      fetchGitRepoFileContent(repoName, path)
        .then(setFileContent)
        .catch((e: Error) => setFileError(e.message))
        .finally(() => setLoadingFile(false));
    },
    [repoName],
  );

  function isMarkdownFile(path: string): boolean {
    const lower = path.toLowerCase();
    return lower.endsWith(".md") || lower.endsWith(".markdown");
  }

  function formatSize(bytes?: number): string {
    if (bytes === undefined || bytes === 0) return "";
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  function renderTree(nodes: TreeNode[], depth: number = 0) {
    return nodes.map((node) => (
      <div key={node.path}>
        <button
          onClick={() =>
            node.type === "dir" ? toggleDir(node.path) : selectFile(node.path)
          }
          className={`flex w-full items-center gap-1.5 rounded px-1.5 py-[3px] text-left text-[13px] hover:bg-gray-100 ${
            selectedFile === node.path
              ? "bg-blue-50 text-blue-700"
              : "text-gray-700"
          }`}
          style={{ paddingLeft: `${depth * 16 + 6}px` }}
        >
          {node.type === "dir" ? (
            <span className="inline-flex w-4 shrink-0 items-center justify-center text-[10px] text-gray-400">
              {node.expanded ? "▼" : "▶"}
            </span>
          ) : (
            <span className="inline-flex w-4 shrink-0" />
          )}
          <span className="shrink-0 text-sm">
            {node.type === "dir"
              ? node.expanded
                ? "📂"
                : "📁"
              : "📄"}
          </span>
          <span className="truncate">{node.name}</span>
          {node.type === "file" && node.size != null && node.size > 0 && (
            <span className="ml-auto shrink-0 text-[11px] text-gray-400">
              {formatSize(node.size)}
            </span>
          )}
        </button>
        {node.type === "dir" && node.expanded && node.children && (
          <div>{renderTree(node.children, depth + 1)}</div>
        )}
        {node.type === "dir" && node.expanded && !node.loaded && (
          <div
            className="text-[11px] text-gray-400 italic"
            style={{ paddingLeft: `${(depth + 1) * 16 + 6}px` }}
          >
            Loading…
          </div>
        )}
      </div>
    ));
  }

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
      {/* Tree panel */}
      <div className="lg:col-span-1">
        <div className="card max-h-[700px] overflow-y-auto p-2">
          <div className="mb-1 flex items-center gap-2 px-1.5 pb-1 border-b border-gray-100">
            <span className="text-sm">📁</span>
            <span className="text-[13px] font-semibold text-gray-700">
              {repoName}
            </span>
          </div>

          {loadingRoot && <LoadingSpinner message="Loading files…" />}
          {treeError && (
            <div className="mt-2 text-sm text-red-600">{treeError}</div>
          )}

          {!loadingRoot && !treeError && tree.length === 0 && (
            <p className="mt-2 text-xs text-gray-400">Empty repository</p>
          )}

          {!loadingRoot && !treeError && (
            <div className="mt-1">{renderTree(tree)}</div>
          )}
        </div>
      </div>

      {/* File content panel */}
      <div className="lg:col-span-2">
        <div className="card">
          {!selectedFile && (
            <p className="text-sm text-gray-400">
              Select a file to view its contents.
            </p>
          )}
          {selectedFile && loadingFile && (
            <LoadingSpinner message="Loading file…" />
          )}
          {selectedFile && fileError && (
            <div className="text-sm text-red-600">{fileError}</div>
          )}
          {selectedFile && fileContent && (
            <div>
              <div className="mb-3 flex items-center justify-between border-b border-gray-100 pb-2">
                <span className="font-mono text-xs text-gray-500">
                  {fileContent.path}
                </span>
                <div className="flex items-center gap-2">
                  {fileContent.encoding === "text" &&
                    isMarkdownFile(fileContent.path) && (
                      <div className="inline-flex rounded-md border border-gray-200 text-[11px] font-medium">
                        <button
                          onClick={() => setViewMode("rendered")}
                          className={`rounded-l-md px-2 py-0.5 ${
                            viewMode === "rendered"
                              ? "bg-blue-50 text-blue-700"
                              : "text-gray-500 hover:bg-gray-50"
                          }`}
                        >
                          Rendered
                        </button>
                        <button
                          onClick={() => setViewMode("raw")}
                          className={`rounded-r-md border-l border-gray-200 px-2 py-0.5 ${
                            viewMode === "raw"
                              ? "bg-blue-50 text-blue-700"
                              : "text-gray-500 hover:bg-gray-50"
                          }`}
                        >
                          Raw
                        </button>
                      </div>
                    )}
                  <span className="text-xs text-gray-400">
                    {formatSize(fileContent.size)}
                    {fileContent.encoding === "base64" && " (binary)"}
                  </span>
                </div>
              </div>
              {fileContent.encoding === "base64" ? (
                <div className="flex items-center justify-center rounded-lg border border-dashed border-gray-200 p-8">
                  <p className="text-sm text-gray-500">
                    Binary file — content cannot be displayed as text.
                  </p>
                </div>
              ) : isMarkdownFile(fileContent.path) &&
                viewMode === "rendered" ? (
                <div className="prose prose-sm max-w-none max-h-[600px] overflow-auto rounded-lg bg-white p-4">
                  <ReactMarkdown
                    remarkPlugins={[remarkGfm]}
                    components={{
                      a: ({ href, children, ...props }) => (
                        <a
                          href={href}
                          target="_blank"
                          rel="noopener noreferrer"
                          {...props}
                        >
                          {children}
                        </a>
                      ),
                    }}
                  >
                    {fileContent.content}
                  </ReactMarkdown>
                </div>
              ) : (
                <pre className="max-h-[600px] overflow-auto rounded-lg bg-gray-50 p-4 text-xs leading-relaxed text-gray-800">
                  <code>{fileContent.content}</code>
                </pre>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
