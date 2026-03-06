import { useState, useEffect, useCallback } from "react";
import { useOrg } from "../context/OrgContext";
import {
  fetchVersionDistribution,
  fetchReadiness,
  fetchCookbookCompatibility,
} from "../api";
import type {
  VersionDistributionResponse,
  ReadinessResponse,
  CookbookCompatibilityResponse,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";

// ---------------------------------------------------------------------------
// Dashboard page — three summary panels:
//   1. Chef Client Version Distribution (bar chart)
//   2. Node Upgrade Readiness (ready / blocked / stale counts)
//   3. Cookbook Compatibility (compatible / incompatible / untested)
// ---------------------------------------------------------------------------

export function DashboardPage() {
  const { selectedOrg } = useOrg();
  const org = selectedOrg || undefined;

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-bold text-gray-800">Dashboard</h2>
      <div className="grid gap-6 lg:grid-cols-2 xl:grid-cols-3">
        <VersionDistributionCard organisation={org} />
        <ReadinessCard organisation={org} />
        <CookbookCompatibilityCard organisation={org} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Version Distribution Card
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
    <div className="card xl:col-span-2">
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
                  <div key={v.version} className="bar-chart-row">
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
                  </div>
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
// Readiness Card
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
                      <div
                        className="bg-green-500 transition-all duration-500"
                        style={{ width: `${(r.ready_nodes / r.total_nodes) * 100}%` }}
                        title={`Ready: ${r.ready_nodes}`}
                      />
                      <div
                        className="bg-red-400 transition-all duration-500"
                        style={{ width: `${(r.blocked_nodes / r.total_nodes) * 100}%` }}
                        title={`Blocked: ${r.blocked_nodes}`}
                      />
                    </div>
                  )}
                  <div className="flex gap-4 text-xs">
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Ready: {r.ready_nodes.toLocaleString()} ({r.ready_percent.toFixed(1)}%)
                    </span>
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Blocked: {r.blocked_nodes.toLocaleString()}
                    </span>
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
// Cookbook Compatibility Card
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
      <h3 className="card-header">Cookbook Compatibility</h3>
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
                      <div
                        className="bg-green-500 transition-all duration-500"
                        style={{ width: `${(c.compatible_cookbooks / c.total_cookbooks) * 100}%` }}
                        title={`Compatible: ${c.compatible_cookbooks}`}
                      />
                      <div
                        className="bg-red-400 transition-all duration-500"
                        style={{ width: `${(c.incompatible_cookbooks / c.total_cookbooks) * 100}%` }}
                        title={`Incompatible: ${c.incompatible_cookbooks}`}
                      />
                      <div
                        className="bg-gray-300 transition-all duration-500"
                        style={{ width: `${(c.untested_cookbooks / c.total_cookbooks) * 100}%` }}
                        title={`Untested: ${c.untested_cookbooks}`}
                      />
                    </div>
                  )}
                  <div className="flex flex-wrap gap-3 text-xs">
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-green-500" />
                      Compatible: {c.compatible_cookbooks}
                    </span>
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-400" />
                      Incompatible: {c.incompatible_cookbooks}
                    </span>
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-full bg-gray-300" />
                      Untested: {c.untested_cookbooks}
                    </span>
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
