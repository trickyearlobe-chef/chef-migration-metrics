// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import { fetchDeploymentStatus } from "../../api";
import type { DeploymentStatusResponse } from "../../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../../components/Feedback";

// ---------------------------------------------------------------------------
// Deployment Status Card — per-version battery bars
// ---------------------------------------------------------------------------

export function DeploymentStatusCard({
  organisation,
}: {
  organisation?: string;
}) {
  const [data, setData] = useState<DeploymentStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchDeploymentStatus(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="card">
      <h3 className="card-header">Deployment Status — Per Version</h3>
      <p className="mb-3 text-xs text-gray-500">
        Current deployment state for each Chef version across the fleet. Shows staged, activated, and converge status per version.
      </p>
      {loading && <LoadingSpinner message="Loading deployment status…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && data && data.data.length === 0 && (
        <EmptyState title="No deployment data available." />
      )}
      {!loading && !error && data && data.data.length > 0 && (
        <div className="space-y-4">
          {data.data.map((entry) => (
            <VersionDeploymentBar key={entry.version} entry={entry} totalNodes={data.total_nodes} />
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Per-version horizontal stacked bar
// ---------------------------------------------------------------------------

interface VersionBarProps {
  entry: DeploymentStatusResponse["data"][number];
  totalNodes: number;
}

function VersionDeploymentBar({ entry, totalNodes }: VersionBarProps) {
  const barWidth = totalNodes > 0 ? (entry.total / totalNodes) * 100 : 0;

  const segments = [
    { label: "Staged", count: entry.staged, colour: "#60a5fa" },       // blue-400
    { label: "Activated", count: entry.activated, colour: "#34d399" },  // emerald-400
    { label: "Converge Passing", count: entry.converge_passing, colour: "#22c55e" }, // green-500
    { label: "Converge Failing", count: entry.converge_failing, colour: "#ef4444" }, // red-500
  ];

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-sm">
        <span className="font-medium text-gray-700">{entry.version}</span>
        <span className="text-xs text-gray-500">
          {entry.total} node{entry.total !== 1 ? "s" : ""}
        </span>
      </div>
      <div
        className="flex h-5 overflow-hidden rounded bg-gray-100"
        style={{ width: `${Math.max(barWidth, 5)}%` }}
        role="img"
        aria-label={`${entry.version}: ${entry.staged} staged, ${entry.activated} activated, ${entry.converge_passing} passing, ${entry.converge_failing} failing`}
      >
        {segments.map((seg) => {
          if (seg.count === 0) return null;
          const segWidth = (seg.count / entry.total) * 100;
          return (
            <div
              key={seg.label}
              className="h-full border-r border-white/50"
              style={{ width: `${segWidth}%`, backgroundColor: seg.colour }}
              title={`${seg.label}: ${seg.count}`}
            />
          );
        })}
      </div>
      <div className="flex flex-wrap gap-3 text-xs text-gray-500">
        {segments.map((seg) => (
          <span key={seg.label} className="flex items-center gap-1">
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{ backgroundColor: seg.colour }}
            />
            {seg.label}: {seg.count}
          </span>
        ))}
      </div>
    </div>
  );
}
