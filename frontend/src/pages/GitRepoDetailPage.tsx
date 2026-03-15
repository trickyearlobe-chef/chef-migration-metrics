import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { fetchGitRepoDetail, requestGitRepoRescan, resetGitRepo } from "../api";
import type { GitRepoDetailResponse } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { StatusBadge } from "../components/StatusBadge";


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

  useEffect(() => { load(); }, [load]);

  if (loading) return <LoadingSpinner message="Loading git repo detail…" />;
  if (error) return <ErrorAlert message={error} onRetry={load} />;
  if (!data) return <LoadingSpinner message="Loading git repo detail…" />;

  const hasGitRepos = data.git_repos && data.git_repos.length > 0;

  return (
    <div className="space-y-6">
      <nav className="text-sm text-gray-500">
        <Link to="/git-repos" className="hover:text-blue-600 hover:underline">Git Repos</Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">{data.name}</span>
      </nav>

      <div className="flex items-center gap-4">
        <h2 className="text-xl font-bold text-gray-800">{data.name}</h2>
        <button
          onClick={handleRescan}
          disabled={rescanning}
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
          title="Invalidate cached CookStyle results and rescan on the next collection cycle"
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
          <p className="font-medium">Are you sure you want to reset git data for "{data.name}"?</p>
          <p className="mt-1 text-red-600">
            This will delete all git-sourced repo data, committer data, and the local clone.
            The repo will be re-cloned from the currently configured git base URLs on the next collection cycle.
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

      {hasGitRepos && (
        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <h4 className="text-sm font-medium text-gray-600">Committers</h4>
              <p className="mt-1 text-sm text-gray-500">
                View committer history and assign repository owners
              </p>
            </div>
            <Link
              to={`/git-repos/${encodeURIComponent(data.name)}/committers`}
              className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-blue-600 shadow-sm hover:bg-gray-50"
            >
              View Committers →
            </Link>
          </div>
        </div>
      )}

      {!hasGitRepos ? (
        <EmptyState title="No git repo entries found" />
      ) : (
        <div className="space-y-4">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-gray-500">Git Repositories</h3>
          {data.git_repos.map((gd, idx) => {
            const gr = gd.git_repo;
            return (
              <div key={`gr-${idx}`} className="card">
                {/* Header */}
                <div className="mb-4 flex flex-wrap items-center gap-3">
                  <h3 className="text-base font-semibold text-gray-800">
                    {gr.name}
                  </h3>
                  <span className="badge badge-compatible">Git</span>
                  {gr.has_test_suite ? (
                    <StatusBadge variant="compatible" label="Has Test Suite" size="sm" />
                  ) : (
                    <StatusBadge variant="untested" label="No Test Suite" size="sm" />
                  )}
                  {gr.git_repo_url && (
                    <span className="text-xs text-gray-400 truncate max-w-md" title={gr.git_repo_url}>
                      {gr.git_repo_url}
                    </span>
                  )}
                </div>

                {/* Metadata */}
                <div className="mb-4 flex flex-wrap items-center gap-4 text-xs text-gray-500">
                  {gr.default_branch && (
                    <span>
                      Branch: <code className="rounded bg-gray-100 px-1 py-0.5">{gr.default_branch}</code>
                    </span>
                  )}
                  {gr.head_commit_sha && (
                    <span>
                      HEAD: <code className="rounded bg-gray-100 px-1 py-0.5" title={gr.head_commit_sha}>
                        {gr.head_commit_sha.substring(0, 12)}
                      </code>
                    </span>
                  )}
                  {gr.last_fetched_at && (
                    <span>
                      Last fetched: {new Date(gr.last_fetched_at).toLocaleString()}
                    </span>
                  )}
                </div>

                {/* Cookstyle results */}
                {gd.cookstyle && gd.cookstyle.length > 0 && (
                  <div>
                    <h4 className="mb-2 text-sm font-medium text-gray-600">CookStyle Results</h4>
                    <div className="space-y-2">
                      {gd.cookstyle.map((cs) => (
                        <div key={cs.id} className="flex flex-wrap items-center gap-3 rounded-lg border border-gray-100 p-3">
                          <span className="text-xs text-gray-500">Target: {cs.target_chef_version}</span>
                          <StatusBadge
                            variant={cs.passed ? "compatible" : "incompatible"}
                            label={cs.passed ? "Passed" : "Failed"}
                            size="sm"
                          />
                          <span className="text-xs text-gray-500">
                            Offences: {cs.offence_count} | Deprecations: {cs.deprecation_count}
                          </span>
                          <span className="text-xs text-gray-400">
                            Scanned: {new Date(cs.scanned_at).toLocaleString()}
                          </span>
                          {cs.target_chef_version && (
                            <Link
                              to={`/git-repos/${encodeURIComponent(gr.name)}/latest/remediation?target_chef_version=${encodeURIComponent(cs.target_chef_version)}`}
                              className="ml-auto text-xs font-medium text-blue-600 hover:text-blue-800 hover:underline"
                            >
                              View Remediation Detail →
                            </Link>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Test Kitchen results */}
                {gd.test_kitchen && gd.test_kitchen.length > 0 && (
                  <div className="mt-4">
                    <h4 className="mb-2 text-sm font-medium text-gray-600">Test Kitchen Results</h4>
                    <div className="space-y-2">
                      {gd.test_kitchen.map((tk) => (
                        <div key={tk.id} className="flex flex-wrap items-center gap-3 rounded-lg border border-gray-100 p-3">
                          <span className="text-xs text-gray-500">Target: {tk.target_chef_version}</span>
                          <StatusBadge
                            variant={tk.compatible ? "compatible" : "incompatible"}
                            label={tk.compatible ? "Compatible" : "Incompatible"}
                            size="sm"
                          />
                          {tk.timed_out && <StatusBadge variant="stale" label="Timed Out" size="sm" />}
                          <span className="text-xs text-gray-500">
                            Converge: {tk.converge_passed ? "✓" : "✗"} | Tests: {tk.tests_passed ? "✓" : "✗"}
                          </span>
                          {tk.platform_tested && (
                            <span className="text-xs text-gray-400">
                              {tk.platform_tested}{tk.driver_used ? ` (${tk.driver_used})` : ""}
                            </span>
                          )}
                          <span className="text-xs text-gray-400">
                            {tk.duration_seconds}s · {new Date(tk.completed_at).toLocaleString()}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Complexity results */}
                {gd.complexity && gd.complexity.length > 0 && (
                  <div className="mt-4">
                    <h4 className="mb-2 text-sm font-medium text-gray-600">Complexity Analysis</h4>
                    <div className="space-y-2">
                      {gd.complexity.map((cx) => (
                        <div key={cx.id} className="flex flex-wrap items-center gap-3 rounded-lg border border-gray-100 p-3">
                          <span className="text-xs text-gray-500">Target: {cx.target_chef_version}</span>
                          <StatusBadge
                            variant={
                              cx.complexity_label === "low" ? "low"
                                : cx.complexity_label === "medium" ? "medium"
                                  : cx.complexity_label === "high" ? "high"
                                    : cx.complexity_label === "critical" ? "critical"
                                      : "unknown"
                            }
                            label={`${(cx.complexity_label ?? "unknown").charAt(0).toUpperCase() + (cx.complexity_label ?? "unknown").slice(1)} (${cx.complexity_score ?? 0})`}
                            size="sm"
                          />
                          <span className="text-xs text-gray-500">
                            Auto-fix: {cx.auto_correctable_count} | Manual: {cx.manual_fix_count} | Errors: {cx.error_count}
                          </span>
                          <span className="text-xs text-gray-500">
                            Deprecations: {cx.deprecation_count} | Correctness: {cx.correctness_count} | Modernize: {cx.modernize_count}
                          </span>
                          <span className="text-xs text-gray-400">
                            {new Date(cx.created_at).toLocaleString()}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
