import { useState, useEffect, useCallback, useRef } from "react";
import { useParams, Link } from "react-router-dom";
import {
  fetchGitRepoDetail,
  requestGitRepoRescan,
  requestGitRepoTestKitchenRescan,
  resetGitRepo,
  triggerGitKitchenRun,
  fetchKitchenAnalysisCookbook,
  fetchFilterTargetChefVersions,
} from "../api";
import type {
  GitRepoDetailResponse,
  TestKitchenResult,
  KitchenPlatformInfo,
  KitchenSuiteInfo,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { StatusBadge } from "../components/StatusBadge";
import { CookstyleResultRow } from "../components/CookstyleResultRow";

function TKResultCard({ tk }: { tk: TestKitchenResult }) {
  const [showLogs, setShowLogs] = useState(false);
  const hasLogs = !!(
    tk.converge_output ||
    tk.verify_output ||
    tk.destroy_output
  );

  return (
    <div className="rounded-lg border border-gray-100 p-3">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-xs text-gray-500">
          Target: {tk.target_chef_version}
        </span>
        <StatusBadge
          variant={tk.compatible ? "compatible" : "incompatible"}
          label={tk.compatible ? "Compatible" : "Incompatible"}
          size="sm"
        />
        {tk.timed_out && (
          <StatusBadge variant="stale" label="Timed Out" size="sm" />
        )}
        <span className="text-xs text-gray-500">
          Converge: {tk.converge_passed ? "✓" : "✗"} | Tests:{" "}
          {tk.tests_passed ? "✓" : "✗"}
        </span>
        {tk.platform_tested && (
          <span className="text-xs text-gray-400">
            {tk.platform_tested}
            {tk.driver_used ? ` (${tk.driver_used})` : ""}
          </span>
        )}
        <span className="text-xs text-gray-400">
          {tk.duration_seconds}s · {new Date(tk.completed_at).toLocaleString()}
        </span>
        {hasLogs && (
          <button
            onClick={() => setShowLogs((v) => !v)}
            className="ml-auto text-xs text-blue-600 hover:text-blue-800"
          >
            {showLogs ? "Hide logs" : "Show logs"}
          </button>
        )}
      </div>
      {showLogs && hasLogs && (
        <div className="mt-3 space-y-2">
          {tk.converge_output && (
            <div>
              <div className="mb-1 text-xs font-medium text-gray-500">
                Converge
              </div>
              <pre className="max-h-96 overflow-auto rounded bg-gray-900 p-3 text-xs text-gray-100 whitespace-pre-wrap">
                {tk.converge_output}
              </pre>
            </div>
          )}
          {tk.verify_output && (
            <div>
              <div className="mb-1 text-xs font-medium text-gray-500">
                Verify
              </div>
              <pre className="max-h-96 overflow-auto rounded bg-gray-900 p-3 text-xs text-gray-100 whitespace-pre-wrap">
                {tk.verify_output}
              </pre>
            </div>
          )}
          {tk.destroy_output && (
            <div>
              <div className="mb-1 text-xs font-medium text-gray-500">
                Destroy
              </div>
              <pre className="max-h-96 overflow-auto rounded bg-gray-900 p-3 text-xs text-gray-100 whitespace-pre-wrap">
                {tk.destroy_output}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function GitRepoDetailPage() {
  const { name } = useParams<{ name: string }>();

  const [data, setData] = useState<GitRepoDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [rescanning, setRescanning] = useState(false);
  const [rescanMsg, setRescanMsg] = useState<string | null>(null);
  const [rescanningTK, setRescanningTK] = useState(false);
  const [rescanTKMsg, setRescanTKMsg] = useState<string | null>(null);
  const [resetting, setResetting] = useState(false);
  const [resetMsg, setResetMsg] = useState<string | null>(null);
  const [showResetConfirm, setShowResetConfirm] = useState(false);

  // Kitchen test trigger state
  const [showKitchenForm, setShowKitchenForm] = useState(false);
  const [targetVersions, setTargetVersions] = useState<string[]>([]);
  const [platforms, setPlatforms] = useState<KitchenPlatformInfo[]>([]);
  const [suites, setSuites] = useState<KitchenSuiteInfo[]>([]);
  const [kitchenVersion, setKitchenVersion] = useState("");
  const [kitchenPlatform, setKitchenPlatform] = useState("");
  const [kitchenSuite, setKitchenSuite] = useState("");
  const [kitchenTriggering, setKitchenTriggering] = useState(false);
  const [kitchenMsg, setKitchenMsg] = useState<string | null>(null);
  const kitchenPollingRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const load = useCallback(() => {
    if (!name) return;
    setLoading(true);
    setError(null);
    fetchGitRepoDetail(name)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name]);

  useEffect(() => {
    fetchFilterTargetChefVersions()
      .then((res) => {
        setTargetVersions(res.data || []);
        if (res.data && res.data.length > 0) setKitchenVersion(res.data[0]);
      })
      .catch(() => {
        /* ignore */
      });
  }, []);

  useEffect(() => {
    if (!name) return;
    fetchKitchenAnalysisCookbook(name)
      .then((analysis) => {
        if (analysis) {
          const plats = analysis.platforms || [];
          const sts = analysis.suites || [];
          setPlatforms(plats);
          setSuites(sts);
          if (plats.length > 0) setKitchenPlatform(plats[0].name);
          if (sts.length > 0) setKitchenSuite(sts[0].name);
        }
      })
      .catch(() => {
        /* ignore */
      });
  }, [name]);

  useEffect(() => {
    return () => {
      if (kitchenPollingRef.current) clearInterval(kitchenPollingRef.current);
    };
  }, []);

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

  const handleRescanTK = useCallback(() => {
    if (!name) return;
    setRescanningTK(true);
    setRescanTKMsg(null);
    requestGitRepoTestKitchenRescan(name)
      .then((res) => {
        setRescanTKMsg(res.message);
        load();
      })
      .catch((e: Error) => setRescanTKMsg(`Rerun failed: ${e.message}`))
      .finally(() => setRescanningTK(false));
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

  const handleKitchenTrigger = useCallback(async () => {
    if (!name) return;
    setKitchenTriggering(true);
    setKitchenMsg(null);
    try {
      const resp = await triggerGitKitchenRun({
        git_repo_name: name,
        target_chef_version: kitchenVersion,
        platform_name: kitchenPlatform,
        suite_name: kitchenSuite,
      });
      setKitchenMsg(`Started: ${resp.message}`);
      // Poll for completion by reloading the page data
      if (kitchenPollingRef.current) clearInterval(kitchenPollingRef.current);
      const startTime = Date.now();
      kitchenPollingRef.current = setInterval(() => {
        if (Date.now() - startTime > 30 * 60 * 1000) {
          if (kitchenPollingRef.current)
            clearInterval(kitchenPollingRef.current);
          return;
        }
        load();
      }, 10000);
    } catch (e: unknown) {
      setKitchenMsg(`Error: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setKitchenTriggering(false);
    }
  }, [name, kitchenVersion, kitchenPlatform, kitchenSuite, load]);

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
          onClick={handleRescanTK}
          disabled={rescanningTK}
          className="inline-flex items-center gap-1.5 rounded-md border border-orange-300 bg-white px-3 py-1.5 text-sm font-medium text-orange-700 shadow-sm hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50"
          title="Invalidate cached Test Kitchen results and trigger an immediate rerun"
        >
          {rescanningTK ? "Requesting…" : "Rerun Test Kitchen"}
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

      {rescanTKMsg && (
        <div className="rounded-md border border-orange-200 bg-orange-50 px-4 py-3 text-sm text-orange-800">
          {rescanTKMsg}
        </div>
      )}

      {resetMsg && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          {resetMsg}
        </div>
      )}

      {hasGitRepos && (
        <div className="card">
          <h3 className="card-header">Run Kitchen Test</h3>
          <div className="mb-2">
            <button
              className="text-sm text-blue-600 hover:underline"
              onClick={() => setShowKitchenForm(!showKitchenForm)}
            >
              {showKitchenForm ? "▾ Hide Test Form" : "▸ New Kitchen Run…"}
            </button>
          </div>
          {showKitchenForm && (
            <div className="rounded border border-gray-200 bg-gray-50 p-4 space-y-3">
              <div className="flex flex-wrap gap-4 items-end">
                <label className="text-sm">
                  <span className="block font-medium text-gray-700 mb-1">
                    Target Chef Version
                  </span>
                  <select
                    className="rounded border border-gray-300 px-2 py-1 text-sm"
                    value={kitchenVersion}
                    onChange={(e) => setKitchenVersion(e.target.value)}
                  >
                    {targetVersions.map((v) => (
                      <option key={v} value={v}>
                        {v}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="text-sm">
                  <span className="block font-medium text-gray-700 mb-1">
                    Platform
                  </span>
                  {platforms.length > 0 ? (
                    <select
                      className="rounded border border-gray-300 px-2 py-1 text-sm"
                      value={kitchenPlatform}
                      onChange={(e) => setKitchenPlatform(e.target.value)}
                    >
                      {platforms.map((p) => (
                        <option key={p.name} value={p.name}>
                          {p.name}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      type="text"
                      className="rounded border border-gray-300 px-2 py-1 text-sm"
                      value={kitchenPlatform}
                      onChange={(e) => setKitchenPlatform(e.target.value)}
                      placeholder="e.g. ubuntu-22.04"
                    />
                  )}
                </label>
                <label className="text-sm">
                  <span className="block font-medium text-gray-700 mb-1">
                    Suite
                  </span>
                  {suites.length > 0 ? (
                    <select
                      className="rounded border border-gray-300 px-2 py-1 text-sm"
                      value={kitchenSuite}
                      onChange={(e) => setKitchenSuite(e.target.value)}
                    >
                      {suites.map((s) => (
                        <option key={s.name} value={s.name}>
                          {s.name}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      type="text"
                      className="rounded border border-gray-300 px-2 py-1 text-sm"
                      value={kitchenSuite}
                      onChange={(e) => setKitchenSuite(e.target.value)}
                      placeholder="e.g. default"
                    />
                  )}
                </label>
                <button
                  className="rounded bg-blue-600 px-3 py-1 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
                  onClick={handleKitchenTrigger}
                  disabled={
                    kitchenTriggering ||
                    !kitchenVersion ||
                    !kitchenPlatform ||
                    !kitchenSuite
                  }
                >
                  {kitchenTriggering ? "Starting…" : "Run Test"}
                </button>
              </div>
              {kitchenMsg && (
                <p
                  className={`text-sm ${kitchenMsg.startsWith("Error") ? "text-red-600" : "text-blue-600"}`}
                >
                  {kitchenMsg}
                </p>
              )}
            </div>
          )}
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
          <h3 className="text-sm font-semibold uppercase tracking-wide text-gray-500">
            Git Repositories
          </h3>
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
                  {gr.clone_status === "failed" ? (
                    <span className="inline-flex items-center rounded-full bg-red-100 px-2 py-0.5 text-[10px] font-semibold text-red-800 ring-1 ring-inset ring-red-600/20">
                      Missing
                    </span>
                  ) : gr.clone_status === "pending" ? (
                    <span className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-semibold text-gray-600 ring-1 ring-inset ring-gray-500/20">
                      Pending
                    </span>
                  ) : null}
                  {gr.clone_status === "ok" && gr.has_test_suite ? (
                    <StatusBadge
                      variant="compatible"
                      label="Has Test Suite"
                      size="sm"
                    />
                  ) : gr.clone_status === "ok" ? (
                    <StatusBadge
                      variant="untested"
                      label="No Test Suite"
                      size="sm"
                    />
                  ) : null}
                  {gr.git_repo_url && (
                    <span
                      className="text-xs text-gray-400 truncate max-w-md"
                      title={gr.git_repo_url}
                    >
                      {gr.git_repo_url}
                    </span>
                  )}
                </div>

                {/* Clone failure alert */}
                {gr.clone_status === "failed" && (
                  <div className="mb-4 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3">
                    <span className="mt-0.5 shrink-0 text-red-500">⚠</span>
                    <div className="text-sm text-red-700">
                      <p className="font-medium">
                        Repository could not be cloned
                      </p>
                      {gr.clone_error && (
                        <p className="mt-1 text-xs text-red-600">
                          {gr.clone_error}
                        </p>
                      )}
                      <p className="mt-1 text-xs text-red-500">
                        Clone will be reattempted on the next collection run.
                      </p>
                    </div>
                  </div>
                )}

                {/* Metadata */}
                <div className="mb-4 flex flex-wrap items-center gap-4 text-xs text-gray-500">
                  {gr.default_branch && (
                    <span>
                      Branch:{" "}
                      <code className="rounded bg-gray-100 px-1 py-0.5">
                        {gr.default_branch}
                      </code>
                    </span>
                  )}
                  {gr.head_commit_sha && (
                    <span>
                      HEAD:{" "}
                      <code
                        className="rounded bg-gray-100 px-1 py-0.5"
                        title={gr.head_commit_sha}
                      >
                        {gr.head_commit_sha.substring(0, 12)}
                      </code>
                    </span>
                  )}
                  {gr.last_fetched_at && (
                    <span>
                      Last fetched:{" "}
                      {new Date(gr.last_fetched_at).toLocaleString()}
                    </span>
                  )}
                </div>

                {/* Cookstyle results */}
                <div>
                  <h4 className="mb-2 text-sm font-medium text-gray-600">
                    CookStyle Results
                  </h4>
                  {gd.cookstyle && gd.cookstyle.length > 0 ? (
                    <div className="space-y-2">
                      {gd.cookstyle.map((cs) => (
                        <CookstyleResultRow
                          key={cs.id}
                          result={cs}
                          linkBase={`/git-repos/${encodeURIComponent(gr.name)}/latest/remediation`}
                        />
                      ))}
                    </div>
                  ) : (
                    <div className="flex items-center gap-2 rounded-lg border border-dashed border-gray-200 p-3">
                      <StatusBadge
                        variant="untested"
                        label="Not Yet Scanned"
                        size="sm"
                      />
                      <span className="text-xs text-gray-400">
                        CookStyle results will appear here after the next
                        collection run.
                      </span>
                    </div>
                  )}
                </div>

                {/* Test Kitchen results */}
                <div className="mt-4">
                  <h4 className="mb-2 text-sm font-medium text-gray-600">
                    Test Kitchen Results
                  </h4>
                  {gd.test_kitchen && gd.test_kitchen.length > 0 ? (
                    <div className="space-y-2">
                      {gd.test_kitchen.map((tk) => (
                        <TKResultCard key={tk.target_chef_version} tk={tk} />
                      ))}
                    </div>
                  ) : (
                    <div className="flex items-center gap-2 rounded-lg border border-dashed border-gray-200 p-3">
                      {gr.has_test_suite ? (
                        <>
                          <StatusBadge
                            variant="untested"
                            label="Not Yet Run"
                            size="sm"
                          />
                          <span className="text-xs text-gray-400">
                            A test suite was detected but Test Kitchen has not
                            run yet. Results will appear after the next
                            collection run (requires Test Kitchen to be enabled
                            in the server configuration).
                          </span>
                        </>
                      ) : (
                        <>
                          <StatusBadge
                            variant="untested"
                            label="No Test Suite"
                            size="sm"
                          />
                          <span className="text-xs text-gray-400">
                            This repository does not contain a{" "}
                            <code className="rounded bg-gray-100 px-1 py-0.5">
                              .kitchen.yml
                            </code>{" "}
                            file. Add one to enable integration testing with
                            Test Kitchen.
                          </span>
                        </>
                      )}
                    </div>
                  )}
                </div>

                {/* Complexity results */}
                {gd.complexity && gd.complexity.length > 0 && (
                  <div className="mt-4">
                    <h4 className="mb-2 text-sm font-medium text-gray-600">
                      Complexity Analysis
                    </h4>
                    <div className="space-y-2">
                      {gd.complexity.map((cx) => (
                        <div
                          key={cx.id}
                          className="flex flex-wrap items-center gap-3 rounded-lg border border-gray-100 p-3"
                        >
                          <span className="text-xs text-gray-500">
                            Target: {cx.target_chef_version}
                          </span>
                          <StatusBadge
                            variant={
                              cx.complexity_label === "low"
                                ? "low"
                                : cx.complexity_label === "medium"
                                  ? "medium"
                                  : cx.complexity_label === "high"
                                    ? "high"
                                    : cx.complexity_label === "critical"
                                      ? "critical"
                                      : "unknown"
                            }
                            label={`${(cx.complexity_label ?? "unknown").charAt(0).toUpperCase() + (cx.complexity_label ?? "unknown").slice(1)} (${cx.complexity_score ?? 0})`}
                            size="sm"
                          />
                          <span className="text-xs text-gray-500">
                            Auto-fix: {cx.auto_correctable_count} | Manual:{" "}
                            {cx.manual_fix_count} | Errors: {cx.error_count}
                          </span>
                          <span className="text-xs text-gray-500">
                            Deprecations: {cx.deprecation_count} | Correctness:{" "}
                            {cx.correctness_count} | Modernize:{" "}
                            {cx.modernize_count}
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
