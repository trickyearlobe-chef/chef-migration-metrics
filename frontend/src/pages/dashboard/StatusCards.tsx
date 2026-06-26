import { useState, useEffect, useCallback, useMemo } from "react";
import { Link } from "react-router-dom";
import {
  fetchVersionDistribution,
  fetchPlatformDistribution,
  fetchReadiness,
  fetchCookbookCompatibility,
  fetchGitRepoCompatibility,
  fetchTestKitchenCompatibility,
} from "../../api";
import type {
  VersionDistributionResponse,
  PlatformDistributionResponse,
  PlatformGroup,
  ReadinessResponse,
  CookbookCompatibilityResponse,
  GitRepoCompatibilityResponse,
  TestKitchenCompatibilityResponse,
} from "../../types";
import {
  LoadingSpinner,
  ErrorAlert,
  EmptyState,
} from "../../components/Feedback";
import {
  BatteryBarChart,
  groupByMajorVersion,
} from "../../components/battery-bar";
import type { BarGroup } from "../../components/battery-bar";

// ---------------------------------------------------------------------------
// Version Distribution Card (point-in-time)
// ---------------------------------------------------------------------------

export function VersionDistributionCard({
  organisation,
}: {
  organisation?: string;
}) {
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

  useEffect(() => {
    load();
  }, [load]);

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
            <EmptyState
              title="No node data"
              description="No nodes have been collected yet."
            />
          ) : (
            <BatteryBarChart
              groups={groupByMajorVersion(data.distribution, data.total_nodes, "Chef")}
              totalCount={data.total_nodes}
              childLinkBuilder={(version) =>
                `/nodes?chef_version=${encodeURIComponent(version)}`
              }
            />
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Platform Distribution Card (point-in-time)
// ---------------------------------------------------------------------------

export function PlatformDistributionCard({
  organisation,
}: {
  organisation?: string;
}) {
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

  useEffect(() => {
    load();
  }, [load]);

  const barGroups: BarGroup[] = useMemo(() => {
    if (!data?.groups) return [];
    return platformGroupsToBarGroups(data.groups);
  }, [data]);

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
            <EmptyState
              title="No platform data"
              description="No nodes have been collected yet."
            />
          ) : (
            <BatteryBarChart
              groups={barGroups}
              totalCount={data.total_nodes}
              childLinkBuilder={(platform) =>
                `/nodes?platform=${encodeURIComponent(platform)}`
              }
            />
          )}
        </>
      )}
    </div>
  );
}

/** Convert API PlatformGroup[] to BarGroup[] for BatteryBarChart. */
function platformGroupsToBarGroups(groups: PlatformGroup[]): BarGroup[] {
  return groups.map((g) => ({
    key: g.group_key,
    label: g.group_display_name,
    totalCount: g.total_count,
    totalPercentage: g.total_percent,
    entries: g.versions.map((v) => ({
      label: v.display_name ?? v.platform,
      filterValue: v.platform,
      count: v.count,
      percent: v.percent,
    })),
  }));
}

// ---------------------------------------------------------------------------
// Readiness Card (point-in-time)
// ---------------------------------------------------------------------------

export function ReadinessCard({ organisation }: { organisation?: string }) {
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

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Node Upgrade Readiness</h3>
      {loading && <LoadingSpinner message="Loading readiness…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          {data.data.length === 0 ? (
            <EmptyState
              title="No readiness data"
              description="Configure target Chef versions to see readiness."
            />
          ) : (
            <div className="space-y-4">
              {data.data.map((r) => (
                <div
                  key={r.target_chef_version}
                  className="rounded-lg border border-gray-100 p-4"
                >
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700">
                      Target: {r.target_chef_version}
                    </span>
                    <span className="text-xs text-gray-400">
                      {r.total_nodes} nodes
                    </span>
                  </div>
                  {/* Stacked progress bar */}
                  {r.total_nodes > 0 && (
                    <div className="mb-3 flex h-4 overflow-hidden rounded-full bg-gray-100">
                      <Link
                        to={`/nodes?readiness=ready`}
                        className="bg-green-500 transition-all duration-500 hover:bg-green-600"
                        style={{
                          width: `${(r.ready_nodes / r.total_nodes) * 100}%`,
                        }}
                        title={`Ready: ${r.ready_nodes}`}
                      />
                      {(r.needs_review_nodes ?? 0) > 0 && (
                        <Link
                          to={`/nodes?readiness=needs_review`}
                          className="bg-amber-400 transition-all duration-500 hover:bg-amber-500"
                          style={{
                            width: `${((r.needs_review_nodes ?? 0) / r.total_nodes) * 100}%`,
                          }}
                          title={`Needs review: ${r.needs_review_nodes ?? 0}`}
                        />
                      )}
                      <Link
                        to={`/nodes?readiness=blocked`}
                        className="bg-red-400 transition-all duration-500 hover:bg-red-500"
                        style={{
                          width: `${(r.blocked_nodes / r.total_nodes) * 100}%`,
                        }}
                        title={`Blocked: ${r.blocked_nodes}`}
                      />
                    </div>
                  )}
                  <div className="flex flex-wrap gap-4 text-xs">
                    <Link
                      to={`/nodes?readiness=ready`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Ready: {r.ready_nodes.toLocaleString()} (
                      {r.ready_percent.toFixed(1)}%)
                    </Link>
                    {(r.needs_review_nodes ?? 0) > 0 && (
                      <Link
                        to={`/nodes?readiness=needs_review`}
                        className="flex items-center gap-1 hover:underline"
                      >
                        <span className="inline-block h-2.5 w-2.5 rounded-full bg-amber-400" />
                        Needs review: {(r.needs_review_nodes ?? 0).toLocaleString()} (
                        {(r.total_nodes > 0
                          ? ((r.needs_review_nodes ?? 0) / r.total_nodes) * 100
                          : 0
                        ).toFixed(1)}
                        %)
                      </Link>
                    )}
                    <Link
                      to={`/nodes?readiness=blocked`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Blocked: {r.blocked_nodes.toLocaleString()} (
                      {(r.total_nodes > 0
                        ? (r.blocked_nodes / r.total_nodes) * 100
                        : 0
                      ).toFixed(1)}
                      %)
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

export function CookbookCompatibilityCard({
  organisation,
}: {
  organisation?: string;
}) {
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

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Server Cookbook CookStyle Compatibility</h3>
      {loading && <LoadingSpinner message="Loading compatibility…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          {data.data.length === 0 ? (
            <EmptyState
              title="No compatibility data"
              description="Configure target Chef versions to see compatibility."
            />
          ) : (
            <div className="space-y-4">
              {data.data.map((c) => (
                <div
                  key={c.target_chef_version}
                  className="rounded-lg border border-gray-100 p-4"
                >
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700">
                      Target: {c.target_chef_version}
                    </span>
                    <span className="text-xs text-gray-400">
                      {c.total_cookbooks} cookbooks
                    </span>
                  </div>
                  {/* Stacked progress bar */}
                  {c.total_cookbooks > 0 && (
                    <div className="mb-3 flex h-4 overflow-hidden rounded-full bg-gray-100">
                      <Link
                        to={`/cookbooks?compatibility=compatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-green-500 transition-all duration-500 hover:bg-green-600"
                        style={{
                          width: `${(c.compatible_cookbooks / c.total_cookbooks) * 100}%`,
                        }}
                        title={`Compatible: ${c.compatible_cookbooks}`}
                      />
                      <Link
                        to={`/cookbooks?compatibility=incompatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-red-400 transition-all duration-500 hover:bg-red-500"
                        style={{
                          width: `${(c.incompatible_cookbooks / c.total_cookbooks) * 100}%`,
                        }}
                        title={`Incompatible: ${c.incompatible_cookbooks}`}
                      />
                      <Link
                        to={`/cookbooks?compatibility=untested&active=false&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-gray-300 transition-all duration-500 hover:bg-gray-400"
                        style={{
                          width: `${((c.untested_inactive_cookbooks || 0) / c.total_cookbooks) * 100}%`,
                        }}
                        title={`Inactive (unused): ${c.untested_inactive_cookbooks || 0}`}
                      />
                      <Link
                        to={`/cookbooks?compatibility=untested&active=true&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-amber-300 transition-all duration-500 hover:bg-amber-400"
                        style={{
                          width: `${((c.untested_unscanned_cookbooks || 0) / c.total_cookbooks) * 100}%`,
                        }}
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
                      Compatible: {c.compatible_cookbooks.toLocaleString()}
                    </Link>
                    <Link
                      to={`/cookbooks?compatibility=incompatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Incompatible: {c.incompatible_cookbooks.toLocaleString()}
                    </Link>
                    <Link
                      to={`/cookbooks?compatibility=untested&active=false&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-gray-300" />
                      Inactive:{" "}
                      {(c.untested_inactive_cookbooks || 0).toLocaleString()}
                    </Link>
                    <Link
                      to={`/cookbooks?compatibility=untested&active=true&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-amber-300" />
                      Unscanned:{" "}
                      {(c.untested_unscanned_cookbooks || 0).toLocaleString()}
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
// Git Repo CookStyle Compatibility Card
// ---------------------------------------------------------------------------

export function GitRepoCompatibilityCard({
  organisation,
}: {
  organisation?: string;
}) {
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

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Git Repo CookStyle Compatibility</h3>
      {loading && <LoadingSpinner message="Loading compatibility…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          {data.data.length === 0 ? (
            <EmptyState
              title="No compatibility data"
              description="Configure target Chef versions to see compatibility."
            />
          ) : (
            <div className="space-y-4">
              {data.data.map((c) => (
                <div
                  key={c.target_chef_version}
                  className="rounded-lg border border-gray-100 p-4"
                >
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700">
                      Target: {c.target_chef_version}
                    </span>
                    <span className="text-xs text-gray-400">
                      {c.total_repos} git repos
                    </span>
                  </div>
                  {/* Stacked progress bar */}
                  {c.total_repos > 0 && (
                    <div className="mb-3 flex h-4 overflow-hidden rounded-full bg-gray-100">
                      <Link
                        to={`/git-repos?compatibility=compatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-green-500 transition-all duration-500 hover:bg-green-600"
                        style={{
                          width: `${(c.compatible_repos / c.total_repos) * 100}%`,
                        }}
                        title={`Compatible: ${c.compatible_repos}`}
                      />
                      <Link
                        to={`/git-repos?compatibility=incompatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-red-400 transition-all duration-500 hover:bg-red-500"
                        style={{
                          width: `${(c.incompatible_repos / c.total_repos) * 100}%`,
                        }}
                        title={`Incompatible: ${c.incompatible_repos}`}
                      />
                      {c.untested_clone_failed_repos > 0 && (
                        <Link
                          to={`/git-repos?compatibility=untested&clone_status=failed&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                          className="bg-red-200 transition-all duration-500 hover:bg-red-300"
                          style={{
                            width: `${(c.untested_clone_failed_repos / c.total_repos) * 100}%`,
                            backgroundImage:
                              "repeating-linear-gradient(135deg, transparent, transparent 3px, rgba(255,255,255,0.4) 3px, rgba(255,255,255,0.4) 6px)",
                          }}
                          title={`Clone failed — cannot scan: ${c.untested_clone_failed_repos}`}
                        />
                      )}
                      {c.untested_pending_scan_repos > 0 && (
                        <Link
                          to={`/git-repos?compatibility=untested&clone_status=ok&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                          className="bg-amber-200 transition-all duration-500 hover:bg-amber-300"
                          style={{
                            width: `${(c.untested_pending_scan_repos / c.total_repos) * 100}%`,
                          }}
                          title={`Cloned but not yet scanned: ${c.untested_pending_scan_repos}`}
                        />
                      )}
                    </div>
                  )}
                  <div className="flex flex-wrap gap-3 text-xs">
                    <Link
                      to={`/git-repos?compatibility=compatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Compatible: {c.compatible_repos.toLocaleString()}
                    </Link>
                    <Link
                      to={`/git-repos?compatibility=incompatible&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Incompatible: {c.incompatible_repos.toLocaleString()}
                    </Link>
                    {c.untested_clone_failed_repos > 0 && (
                      <Link
                        to={`/git-repos?compatibility=untested&clone_status=failed&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="flex items-center gap-1 hover:underline"
                      >
                        <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-200 ring-1 ring-red-300" />
                        Clone failed:{" "}
                        {c.untested_clone_failed_repos.toLocaleString()}
                      </Link>
                    )}
                    {c.untested_pending_scan_repos > 0 && (
                      <Link
                        to={`/git-repos?compatibility=untested&clone_status=ok&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="flex items-center gap-1 hover:underline"
                      >
                        <span className="inline-block h-2.5 w-2.5 rounded-full bg-amber-200 ring-1 ring-amber-300" />
                        Pending scan:{" "}
                        {c.untested_pending_scan_repos.toLocaleString()}
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
// Test Kitchen Compatibility Card
// ---------------------------------------------------------------------------

export function TestKitchenCompatibilityCard({
  organisation,
}: {
  organisation?: string;
}) {
  const [data, setData] = useState<TestKitchenCompatibilityResponse | null>(
    null,
  );
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

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Git Repo Test Kitchen Compatibility</h3>
      {loading && <LoadingSpinner message="Loading compatibility…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && (
        <>
          {data.data.length === 0 ? (
            <EmptyState
              title="No Test Kitchen data"
              description="Configure target Chef versions and run Test Kitchen to see results."
            />
          ) : (
            <div className="space-y-4">
              {data.data.map((c) => (
                <div
                  key={c.target_chef_version}
                  className="rounded-lg border border-gray-100 p-4"
                >
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700">
                      Target: {c.target_chef_version}
                    </span>
                    <span className="text-xs text-gray-400">
                      {c.total_repos} git repos
                    </span>
                  </div>
                  {/* Stacked progress bar */}
                  {c.total_repos > 0 && (
                    <div className="mb-3 flex h-4 overflow-hidden rounded-full bg-gray-100">
                      <Link
                        to={`/git-repos?tk_status=passed&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-green-500 transition-all duration-500 hover:bg-green-600"
                        style={{
                          width: `${(c.passed_repos / c.total_repos) * 100}%`,
                        }}
                        title={`Passed: ${c.passed_repos}`}
                      />
                      {c.partial_repos > 0 && (
                        <Link
                          to={`/git-repos?tk_status=partial&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                          className="bg-orange-400 transition-all duration-500 hover:bg-orange-500"
                          style={{
                            width: `${(c.partial_repos / c.total_repos) * 100}%`,
                          }}
                          title={`Partial: ${c.partial_repos}`}
                        />
                      )}
                      <Link
                        to={`/git-repos?tk_status=failed&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="bg-red-400 transition-all duration-500 hover:bg-red-500"
                        style={{
                          width: `${(c.failed_repos / c.total_repos) * 100}%`,
                        }}
                        title={`Failed: ${c.failed_repos}`}
                      />
                      {c.timed_out_repos > 0 && (
                        <Link
                          to={`/git-repos?tk_status=timed_out&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                          className="bg-amber-400 transition-all duration-500 hover:bg-amber-500"
                          style={{
                            width: `${(c.timed_out_repos / c.total_repos) * 100}%`,
                            backgroundImage:
                              "repeating-linear-gradient(135deg, transparent, transparent 3px, rgba(255,255,255,0.3) 3px, rgba(255,255,255,0.3) 6px)",
                          }}
                          title={`Timed out: ${c.timed_out_repos}`}
                        />
                      )}
                      {c.untested_repos > 0 && (
                        <Link
                          to={`/git-repos?tk_status=untested&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                          className="bg-gray-300 transition-all duration-500 hover:bg-gray-400"
                          style={{
                            width: `${(c.untested_repos / c.total_repos) * 100}%`,
                          }}
                          title={`Not tested: ${c.untested_repos}`}
                        />
                      )}
                    </div>
                  )}
                  <div className="flex flex-wrap gap-3 text-xs">
                    <Link
                      to={`/git-repos?tk_status=passed&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Passed: {c.passed_repos.toLocaleString()}
                    </Link>
                    {c.partial_repos > 0 && (
                      <Link
                        to={`/git-repos?tk_status=partial&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="flex items-center gap-1 hover:underline"
                      >
                        <span className="inline-block h-2.5 w-2.5 rounded-full bg-orange-400" />
                        Partial: {c.partial_repos.toLocaleString()}
                      </Link>
                    )}
                    <Link
                      to={`/git-repos?tk_status=failed&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                      className="flex items-center gap-1 hover:underline"
                    >
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Failed: {c.failed_repos.toLocaleString()}
                    </Link>
                    {c.timed_out_repos > 0 && (
                      <Link
                        to={`/git-repos?tk_status=timed_out&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="flex items-center gap-1 hover:underline"
                      >
                        <span className="inline-block h-2.5 w-2.5 rounded-full bg-amber-400" />
                        Timed out: {c.timed_out_repos.toLocaleString()}
                      </Link>
                    )}
                    {c.untested_repos > 0 && (
                      <Link
                        to={`/git-repos?tk_status=untested&target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                        className="flex items-center gap-1 hover:underline"
                      >
                        <span className="inline-block h-2.5 w-2.5 rounded-full bg-gray-300" />
                        Untested: {c.untested_repos.toLocaleString()}
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
