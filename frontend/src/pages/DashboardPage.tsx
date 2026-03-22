import { useState, useEffect, useCallback } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import {
  fetchVersionDistribution,
  fetchPlatformDistribution,
  fetchReadiness,
  fetchCookbookCompatibility,
  fetchGitRepoCompatibility,
  fetchTestKitchenCompatibility,
  fetchVersionDistributionTrend,
  fetchReadinessTrend,
  fetchComplexityTrend,
  fetchStaleTrend,
} from "../api";
import type {
  VersionDistributionResponse,
  PlatformDistributionResponse,
  ReadinessResponse,
  CookbookCompatibilityResponse,
  GitRepoCompatibilityResponse,
  TestKitchenCompatibilityResponse,
  VersionDistributionTrendResponse,
  ReadinessTrendResponse,
  ComplexityTrendResponse,
  StaleTrendResponse,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import {
  TrendChart,
  breakdownToSeries,
} from "../components/TrendChart";
import type { TrendSeries } from "../components/TrendChart";

// ---------------------------------------------------------------------------
// Dashboard page — two tabs:
//   "Current Status" — point-in-time summary cards
//   "Trends"         — historical trend charts
//
// The active tab is persisted in the URL via ?tab=status|trends so that
// bookmarks and shared links preserve the view.
// ---------------------------------------------------------------------------

type DashboardTab = "status" | "trends";

const TABS: { key: DashboardTab; label: string }[] = [
  { key: "status", label: "Current Status" },
  { key: "trends", label: "Trends" },
];

function isValidTab(value: string | null): value is DashboardTab {
  return value === "status" || value === "trends";
}

export function DashboardPage() {
  const { selectedOrg } = useOrg();
  const org = selectedOrg || undefined;

  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get("tab");
  const activeTab: DashboardTab = isValidTab(rawTab) ? rawTab : "status";

  const switchTab = (tab: DashboardTab) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (tab === "status") {
        next.delete("tab");
      } else {
        next.set("tab", tab);
      }
      return next;
    });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-gray-800">Dashboard</h2>
      </div>

      {/* ---- Tab bar ---- */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Dashboard tabs">
          {TABS.map((t) => (
            <button
              key={t.key}
              onClick={() => switchTab(t.key)}
              className={
                "whitespace-nowrap border-b-2 px-1 pb-3 text-sm font-medium transition-colors " +
                (activeTab === t.key
                  ? "border-blue-500 text-blue-600"
                  : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700")
              }
            >
              {t.label}
            </button>
          ))}
        </nav>
      </div>

      {/* ---- Current Status tab ---- */}
      {activeTab === "status" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <VersionDistributionCard organisation={org} />
          <PlatformDistributionCard organisation={org} />
          <ReadinessCard organisation={org} />
          <CookbookCompatibilityCard organisation={org} />
          <GitRepoCompatibilityCard organisation={org} />
          <TestKitchenCompatibilityCard organisation={org} />
        </div>
      )}

      {/* ---- Trends tab ---- */}
      {activeTab === "trends" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <VersionDistributionTrendCard organisation={org} />
          <ReadinessTrendCard organisation={org} />
          <ComplexityTrendCard organisation={org} />
          <StaleTrendCard organisation={org} />
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Version Distribution Card (point-in-time)
// ---------------------------------------------------------------------------

function VersionDistributionCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<VersionDistributionResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchVersionDistribution(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Chef Client Version Distribution</h3>
      {loading && <LoadingSpinner message="Loading version data…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          <p className="mb-4 text-sm text-gray-500">
            {data.total_nodes.toLocaleString()} total nodes
          </p>
          {data.distribution.length === 0 ? (
            <EmptyState title="No node data" description="No nodes have been collected yet." />
          ) : (
            <div className="space-y-1">
              {data.distribution.map((v) => {
                const pct = data.total_nodes > 0 ? (v.count / data.total_nodes) * 100 : 0;
                return (
                  <Link
                    key={v.version}
                    to={`/nodes?chef_version=${encodeURIComponent(v.version)}`}
                    className="bar-chart-row hover:bg-gray-50 rounded transition-colors"
                  >
                    <span className="bar-chart-label" title={v.version}>{v.version}</span>
                    <div className="bar-chart-track">
                      <div
                        className="bar-chart-fill bg-blue-500"
                        style={{ width: `${Math.max(pct, 2)}%` }}
                      >
                        {pct >= 8 && <span>{pct.toFixed(1)}%</span>}
                      </div>
                    </div>
                    <span className="bar-chart-value">{v.count.toLocaleString()}</span>
                  </Link>
                );
              })}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Platform Distribution Card (point-in-time)
// ---------------------------------------------------------------------------

function PlatformDistributionCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<PlatformDistributionResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchPlatformDistribution(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">OS Platform Distribution</h3>
      {loading && <LoadingSpinner message="Loading platform data…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          <p className="mb-4 text-sm text-gray-500">
            {data.total_nodes.toLocaleString()} total nodes
          </p>
          {data.distribution.length === 0 ? (
            <EmptyState title="No platform data" description="No nodes have been collected yet." />
          ) : (
            <div className="space-y-1">
              {data.distribution.map((v) => {
                const pct = data.total_nodes > 0 ? (v.count / data.total_nodes) * 100 : 0;
                return (
                  <Link
                    key={v.platform}
                    to={`/nodes?platform=${encodeURIComponent(v.platform)}`}
                    className="bar-chart-row hover:bg-gray-50 rounded transition-colors"
                  >
                    <span className="w-44 shrink-0 truncate text-right text-sm text-gray-600" title={v.platform}>{v.platform}</span>
                    <div className="bar-chart-track">
                      <div
                        className="bar-chart-fill bg-purple-500"
                        style={{ width: `${Math.max(pct, 2)}%` }}
                      >
                        {pct >= 8 && <span>{pct.toFixed(1)}%</span>}
                      </div>
                    </div>
                    <span className="bar-chart-value">{v.count.toLocaleString()}</span>
                  </Link>
                );
              })}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Readiness Card (point-in-time)
// ---------------------------------------------------------------------------

function ReadinessCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<ReadinessResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchReadiness(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Node Upgrade Readiness</h3>
      {loading && <LoadingSpinner message="Loading readiness…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          {data.data.length === 0 ? (
            <EmptyState title="No readiness data" description="Configure target Chef versions to see readiness." />
          ) : (
            <div className="space-y-4">
              {data.data.map((r) => (
                <div key={r.target_chef_version} className="rounded-lg border border-gray-100 p-4">
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700">Target: {r.target_chef_version}</span>
                    <span className="text-xs text-gray-400">{r.total_nodes} nodes</span>
                  </div>
                  {/* Stacked progress bar */}
                  {r.total_nodes > 0 && (
                    <div className="mb-3 flex h-4 overflow-hidden rounded-full bg-gray-100">
                      <Link
                        to={`/nodes?readiness=ready&target_version=${encodeURIComponent(r.target_chef_version)}`}
                        className="bg-green-500 transition-all duration-500 hover:bg-green-600"
                        style={{ width: `${(r.ready_nodes / r.total_nodes) * 100}%` }}
                        title={`Ready: ${r.ready_nodes}`}
                      />
                      <Link
                        to={`/nodes?readiness=blocked&target_version=${encodeURIComponent(r.target_chef_version)}`}
                        className="bg-red-400 transition-all duration-500 hover:bg-red-500"
                        style={{ width: `${(r.blocked_nodes / r.total_nodes) * 100}%` }}
                        title={`Blocked: ${r.blocked_nodes}`}
                      />
                    </div>
                  )}
                  <div className="flex gap-4 text-xs">
                    <Link
                      to={`/nodes?readiness=ready&target_version=${encodeURIComponent(r.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Ready: {r.ready_nodes.toLocaleString()} ({r.ready_percent.toFixed(1)}%)
                    </Link>
                    <Link
                      to={`/nodes?readiness=blocked&target_version=${encodeURIComponent(r.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Blocked: {r.blocked_nodes.toLocaleString()}
                    </Link>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Cookbook Compatibility Card (point-in-time)
// ---------------------------------------------------------------------------

function CookbookCompatibilityCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<CookbookCompatibilityResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchCookbookCompatibility(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Server Cookbook CookStyle Compatibility</h3>
      {loading && <LoadingSpinner message="Loading compatibility…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          {data.data.length === 0 ? (
            <EmptyState title="No compatibility data" description="Configure target Chef versions to see compatibility." />
          ) : (
            <div className="space-y-4">
              {data.data.map((c) => (
                <div key={c.target_chef_version} className="rounded-lg border border-gray-100 p-4">
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700">Target: {c.target_chef_version}</span>
                    <span className="text-xs text-gray-400">{c.total_cookbooks} cookbooks</span>
                  </div>
                  {/* Stacked progress bar */}
                  {c.total_cookbooks > 0 && (
                    <div className="mb-3 flex h-4 overflow-hidden rounded-full bg-gray-100">
                      <Link
                        to={`/cookbooks?compatibility=compatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-green-500 transition-all duration-500 hover:bg-green-600"
                        style={{ width: `${(c.compatible_cookbooks / c.total_cookbooks) * 100}%` }}
                        title={`Compatible: ${c.compatible_cookbooks}`}
                      />
                      <Link
                        to={`/cookbooks?compatibility=incompatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-red-400 transition-all duration-500 hover:bg-red-500"
                        style={{ width: `${(c.incompatible_cookbooks / c.total_cookbooks) * 100}%` }}
                        title={`Incompatible: ${c.incompatible_cookbooks}`}
                      />
                      <Link
                        to={`/cookbooks?compatibility=untested&active=false&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-gray-300 transition-all duration-500 hover:bg-gray-400"
                        style={{ width: `${((c.untested_inactive_cookbooks || 0) / c.total_cookbooks) * 100}%` }}
                        title={`Inactive (unused): ${c.untested_inactive_cookbooks || 0}`}
                      />
                      <Link
                        to={`/cookbooks?compatibility=untested&active=true&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-amber-300 transition-all duration-500 hover:bg-amber-400"
                        style={{ width: `${((c.untested_unscanned_cookbooks || 0) / c.total_cookbooks) * 100}%` }}
                        title={`Not yet scanned: ${c.untested_unscanned_cookbooks || 0}`}
                      />
                    </div>
                  )}
                  <div className="flex flex-wrap gap-3 text-xs">
                    <Link
                      to={`/cookbooks?compatibility=compatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Compatible: {c.compatible_cookbooks}
                    </Link>
                    <Link
                      to={`/cookbooks?compatibility=incompatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Incompatible: {c.incompatible_cookbooks}
                    </Link>
                    {(c.untested_inactive_cookbooks || 0) > 0 && (
                      <Link
                        to={`/cookbooks?compatibility=untested&active=false&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="flex items-center gap-1 hover:underline"
                      >
                        <span className="inline-block h-2.5 w-2.5 rounded-full bg-gray-300" />
                        Inactive: {c.untested_inactive_cookbooks}
                      </Link>
                    )}
                    {(c.untested_unscanned_cookbooks || 0) > 0 && (
                      <Link
                        to={`/cookbooks?compatibility=untested&active=true&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="flex items-center gap-1 hover:underline"
                      >
                        <span className="inline-block h-2.5 w-2.5 rounded-full bg-amber-300" />
                        Unscanned: {c.untested_unscanned_cookbooks}
                      </Link>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Git Repo CookStyle Compatibility Card
// ---------------------------------------------------------------------------

function GitRepoCompatibilityCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<GitRepoCompatibilityResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchGitRepoCompatibility(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Git Repo CookStyle Compatibility</h3>
      {loading && <LoadingSpinner message="Loading compatibility…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          {data.data.length === 0 ? (
            <EmptyState title="No compatibility data" description="Configure target Chef versions to see compatibility." />
          ) : (
            <div className="space-y-4">
              {data.data.map((c) => (
                <div key={c.target_chef_version} className="rounded-lg border border-gray-100 p-4">
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700">Target: {c.target_chef_version}</span>
                    <span className="text-xs text-gray-400">{c.total_repos} git repos</span>
                  </div>
                  {/* Stacked progress bar */}
                  {c.total_repos > 0 && (
                    <div className="mb-3 flex h-4 overflow-hidden rounded-full bg-gray-100">
                      <Link
                        to={`/git-repos?compatibility=compatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-green-500 transition-all duration-500 hover:bg-green-600"
                        style={{ width: `${(c.compatible_repos / c.total_repos) * 100}%` }}
                        title={`Compatible: ${c.compatible_repos}`}
                      />
                      <Link
                        to={`/git-repos?compatibility=incompatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-red-400 transition-all duration-500 hover:bg-red-500"
                        style={{ width: `${(c.incompatible_repos / c.total_repos) * 100}%` }}
                        title={`Incompatible: ${c.incompatible_repos}`}
                      />
                      <Link
                        to={`/git-repos?compatibility=untested&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-gray-300 transition-all duration-500 hover:bg-gray-400"
                        style={{ width: `${(c.untested_repos / c.total_repos) * 100}%` }}
                        title={`Untested: ${c.untested_repos}`}
                      />
                    </div>
                  )}
                  <div className="flex flex-wrap gap-3 text-xs">
                    <Link
                      to={`/git-repos?compatibility=compatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Compatible: {c.compatible_repos}
                    </Link>
                    <Link
                      to={`/git-repos?compatibility=incompatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Incompatible: {c.incompatible_repos}
                    </Link>
                    <Link
                      to={`/git-repos?compatibility=untested&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-gray-300" />
                      Untested: {c.untested_repos}
                    </Link>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Test Kitchen Compatibility Card
// ---------------------------------------------------------------------------

function TestKitchenCompatibilityCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<TestKitchenCompatibilityResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchTestKitchenCompatibility(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Test Kitchen Compatibility</h3>
      {loading && <LoadingSpinner message="Loading Test Kitchen results…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          {data.data.length === 0 ? (
            <EmptyState title="No Test Kitchen data" description="Configure target Chef versions and run Test Kitchen to see results." />
          ) : (
            <div className="space-y-4">
              {data.data.map((tk) => (
                <div key={tk.target_chef_version} className="rounded-lg border border-gray-100 p-4">
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700">Target: {tk.target_chef_version}</span>
                    <span className="text-xs text-gray-400">{tk.total_repos} git repos</span>
                  </div>
                  {/* Stacked progress bar */}
                  {tk.total_repos > 0 && (
                    <div className="mb-3 flex h-4 overflow-hidden rounded-full bg-gray-100">
                      <Link
                        to={`/git-repos?compatibility=compatible&target_chef_version=${encodeURIComponent(tk.target_chef_version)}`}
                        className="bg-green-500 transition-all duration-500 hover:bg-green-600"
                        style={{ width: `${(tk.passed_repos / tk.total_repos) * 100}%` }}
                        title={`Passed: ${tk.passed_repos}`}
                      />
                      <Link
                        to={`/git-repos?compatibility=incompatible&target_chef_version=${encodeURIComponent(tk.target_chef_version)}`}
                        className="bg-red-400 transition-all duration-500 hover:bg-red-500"
                        style={{ width: `${(tk.failed_repos / tk.total_repos) * 100}%` }}
                        title={`Failed: ${tk.failed_repos}`}
                      />
                      <div
                        className="bg-amber-400 transition-all duration-500"
                        style={{ width: `${(tk.timed_out_repos / tk.total_repos) * 100}%` }}
                        title={`Timed out: ${tk.timed_out_repos}`}
                      />
                      <Link
                        to={`/git-repos?compatibility=untested&target_chef_version=${encodeURIComponent(tk.target_chef_version)}`}
                        className="bg-gray-300 transition-all duration-500 hover:bg-gray-400"
                        style={{ width: `${(tk.untested_repos / tk.total_repos) * 100}%` }}
                        title={`Untested: ${tk.untested_repos}`}
                      />
                    </div>
                  )}
                  <div className="flex flex-wrap gap-3 text-xs">
                    <Link
                      to={`/git-repos?compatibility=compatible&target_chef_version=${encodeURIComponent(tk.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Passed: {tk.passed_repos}
                    </Link>
                    <Link
                      to={`/git-repos?compatibility=incompatible&target_chef_version=${encodeURIComponent(tk.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Failed: {tk.failed_repos}
                    </Link>
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-amber-400" />
                      Timed out: {tk.timed_out_repos}
                    </span>
                    <Link
                      to={`/git-repos?compatibility=untested&target_chef_version=${encodeURIComponent(tk.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-gray-300" />
                      Untested: {tk.untested_repos}
                    </Link>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Version Distribution Trend Card (historical)
// ---------------------------------------------------------------------------

function VersionDistributionTrendCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<VersionDistributionTrendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchVersionDistributionTrend(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  // Transform backend trend points into TrendSeries for the chart.
  // Each collection run is a timestamp; each Chef version is a series line.
  const trendSeries: TrendSeries[] = (() => {
    if (!data || data.data.length === 0) return [];

    // Sort points by completed_at ascending.
    const sorted = [...data.data]
      .filter((pt) => pt.completed_at !== "")
      .sort(
        (a, b) =>
          new Date(a.completed_at).getTime() - new Date(b.completed_at).getTime(),
      );

    if (sorted.length === 0) return [];

    return breakdownToSeries(
      sorted.map((pt) => ({
        timestamp: pt.completed_at,
        breakdown: pt.distribution,
      })),
    );
  })();

  return (
    <div className="card">
      <h3 className="card-header">Version Distribution — Trend</h3>
      {loading && <LoadingSpinner message="Loading version trend…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <TrendChart
          series={trendSeries}
          yLabel="Node count"
          showArea={true}
          height={260}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Readiness Trend Card (historical)
// ---------------------------------------------------------------------------

function ReadinessTrendCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<ReadinessTrendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchReadinessTrend(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  // The readiness trend endpoint currently returns one point per
  // (organisation, target_chef_version) pair — a snapshot of the current
  // state. As more collection runs accumulate, multiple points will appear
  // over time. For now, we render one series per target_chef_version showing
  // the ready_percent.
  //
  // We also add a "ready nodes" / "blocked nodes" absolute count chart when
  // there are multiple data points.
  const { percentSeries, countSeries } = (() => {
    if (!data || data.data.length === 0) {
      return { percentSeries: [] as TrendSeries[], countSeries: [] as TrendSeries[] };
    }

    // Group by target_chef_version.
    const byVersion = new Map<
      string,
      Array<{ org: string; ready: number; blocked: number; total: number; pct: number }>
    >();
    for (const pt of data.data) {
      const list = byVersion.get(pt.target_chef_version) ?? [];
      list.push({
        org: pt.organisation_name,
        ready: pt.ready_nodes,
        blocked: pt.blocked_nodes,
        total: pt.total_nodes,
        pct: pt.ready_percent,
      });
      byVersion.set(pt.target_chef_version, list);
    }

    const colours = [
      "#22c55e", // green-500
      "#3b82f6", // blue-500
      "#f59e0b", // amber-500
      "#ef4444", // red-500
      "#8b5cf6", // violet-500
    ];

    const pctSeries: TrendSeries[] = [];
    const cntSeries: TrendSeries[] = [];
    let colourIdx = 0;

    for (const [version, points] of byVersion) {
      const colour = colours[colourIdx % colours.length];
      colourIdx++;

      // For readiness percent — use organisation name as a pseudo-timestamp
      // so we get one point per org. If there's only one org, we still show
      // it as a single-point chart. When the backend gains historical
      // readiness snapshots, the timestamp will come from the collection run.
      //
      // Since the current endpoint doesn't return timestamps, we synthesise
      // them using the current time offset by index so points are spread out.
      const now = Date.now();
      pctSeries.push({
        key: `ready-pct-${version}`,
        label: `Chef ${version} (% ready)`,
        colour,
        data: points.map((p, i) => ({
          timestamp: new Date(now - (points.length - 1 - i) * 86_400_000).toISOString(),
          value: p.pct,
        })),
      });

      cntSeries.push(
        {
          key: `ready-${version}`,
          label: `Chef ${version} — Ready`,
          colour,
          data: points.map((p, i) => ({
            timestamp: new Date(now - (points.length - 1 - i) * 86_400_000).toISOString(),
            value: p.ready,
          })),
        },
        {
          key: `blocked-${version}`,
          label: `Chef ${version} — Blocked`,
          colour: "#ef4444",
          data: points.map((p, i) => ({
            timestamp: new Date(now - (points.length - 1 - i) * 86_400_000).toISOString(),
            value: p.blocked,
          })),
        },
      );
    }

    return { percentSeries: pctSeries, countSeries: cntSeries };
  })();

  return (
    <div className="card">
      <h3 className="card-header">Node Readiness — Trend</h3>
      {loading && <LoadingSpinner message="Loading readiness trend…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <div className="space-y-6">
          <TrendChart
            series={percentSeries}
            yLabel="Ready %"
            isPercent={true}
            showArea={true}
            height={220}
          />
          {countSeries.length > 0 && (
            <TrendChart
              series={countSeries}
              yLabel="Node count"
              showArea={false}
              height={180}
            />
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Complexity Trend Card (historical)
// ---------------------------------------------------------------------------

function ComplexityTrendCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<ComplexityTrendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchComplexityTrend(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  // Build series from complexity trend points. Each target Chef version
  // gets a line showing average complexity score. We also build a stacked
  // breakdown series showing low/medium/high/critical counts.
  const { scoreSeries, breakdownSeries } = (() => {
    if (!data || data.data.length === 0) {
      return { scoreSeries: [] as TrendSeries[], breakdownSeries: [] as TrendSeries[] };
    }

    // Group by target_chef_version.
    const byVersion = new Map<string, typeof data.data>();
    for (const pt of data.data) {
      const list = byVersion.get(pt.target_chef_version) ?? [];
      list.push(pt);
      byVersion.set(pt.target_chef_version, list);
    }

    const colours = [
      "#f59e0b", // amber-500
      "#3b82f6", // blue-500
      "#8b5cf6", // violet-500
      "#ef4444", // red-500
    ];

    const sScore: TrendSeries[] = [];
    const sBreakdown: TrendSeries[] = [];
    let colourIdx = 0;
    const now = Date.now();

    for (const [version, points] of byVersion) {
      const colour = colours[colourIdx % colours.length];
      colourIdx++;

      // Average score series — one point per org for this version.
      sScore.push({
        key: `avg-score-${version}`,
        label: `Chef ${version} avg score`,
        colour,
        data: points.map((p, i) => ({
          timestamp: new Date(now - (points.length - 1 - i) * 86_400_000).toISOString(),
          value: Math.round(p.average_score * 10) / 10,
        })),
      });

      // Label breakdown — stacked counts per complexity label.
      if (points.length > 0) {
        const latest = points[points.length - 1];
        const ts = new Date(now).toISOString();
        sBreakdown.push(
          { key: `low-${version}`, label: "Low", colour: "#22c55e", data: [{ timestamp: ts, value: latest.low_count }] },
          { key: `med-${version}`, label: "Medium", colour: "#f59e0b", data: [{ timestamp: ts, value: latest.medium_count }] },
          { key: `high-${version}`, label: "High", colour: "#ef4444", data: [{ timestamp: ts, value: latest.high_count }] },
          { key: `crit-${version}`, label: "Critical", colour: "#7c3aed", data: [{ timestamp: ts, value: latest.critical_count }] },
        );
      }
    }

    return { scoreSeries: sScore, breakdownSeries: sBreakdown };
  })();

  return (
    <div className="card">
      <h3 className="card-header">Complexity Score — Trend</h3>
      {loading && <LoadingSpinner message="Loading complexity trend…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <div className="space-y-6">
          <TrendChart
            series={scoreSeries}
            yLabel="Avg complexity"
            showArea={true}
            height={220}
          />
          {breakdownSeries.length > 0 && (
            <div>
              <h4 className="mb-2 text-sm font-semibold text-gray-700">Complexity Breakdown</h4>
              <div className="flex flex-wrap gap-4 text-xs text-gray-600">
                {breakdownSeries.map((s) => (
                  <span key={s.key} className="flex items-center gap-1">
                    <span
                      className="inline-block h-2.5 w-2.5 rounded-full"
                      style={{ backgroundColor: s.colour }}
                    />
                    {s.label}: {s.data[0]?.value ?? 0}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Stale Node Trend Card (historical)
// ---------------------------------------------------------------------------

function StaleTrendCard({ organisation }: { organisation?: string }) {
  const [data, setData] = useState<StaleTrendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchStaleTrend(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => { load(); }, [load]);

  // Transform backend stale trend points into chart series.
  // Each collection run is a timestamp; we show stale and fresh as two lines.
  const trendSeries: TrendSeries[] = (() => {
    if (!data || data.data.length === 0) return [];

    const sorted = [...data.data]
      .filter((pt) => pt.completed_at !== "")
      .sort(
        (a, b) =>
          new Date(a.completed_at).getTime() - new Date(b.completed_at).getTime(),
      );

    if (sorted.length === 0) return [];

    return [
      {
        key: "stale",
        label: "Stale nodes",
        colour: "#ef4444", // red-500
        data: sorted.map((pt) => ({
          timestamp: pt.completed_at,
          value: pt.stale_nodes,
        })),
      },
      {
        key: "fresh",
        label: "Fresh nodes",
        colour: "#22c55e", // green-500
        data: sorted.map((pt) => ({
          timestamp: pt.completed_at,
          value: pt.fresh_nodes,
        })),
      },
    ];
  })();

  return (
    <div className="card">
      <h3 className="card-header">Stale Nodes — Trend</h3>
      {loading && <LoadingSpinner message="Loading stale node trend…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <TrendChart
          series={trendSeries}
          yLabel="Node count"
          showArea={true}
          height={220}
        />
      )}
    </div>
  );
}
