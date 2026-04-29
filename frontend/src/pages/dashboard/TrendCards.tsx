import { useState, useEffect, useCallback } from "react";
import {
  fetchVersionDistributionTrend,
  fetchReadinessTrend,
  fetchComplexityTrend,
  fetchStaleTrend,
} from "../../api";
import type {
  VersionDistributionTrendResponse,
  ReadinessTrendResponse,
  ComplexityTrendResponse,
  StaleTrendResponse,
} from "../../types";
import { LoadingSpinner, ErrorAlert } from "../../components/Feedback";
import { TrendChart, breakdownToSeries } from "../../components/TrendChart";
import type { TrendSeries } from "../../components/TrendChart";

// ---------------------------------------------------------------------------
// Version Distribution Trend Card (historical)
// ---------------------------------------------------------------------------

export function VersionDistributionTrendCard({
  organisation,
}: {
  organisation?: string;
}) {
  const [data, setData] = useState<VersionDistributionTrendResponse | null>(
    null,
  );
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

  useEffect(() => {
    load();
  }, [load]);

  // Transform backend trend points into TrendSeries for the chart.
  // Each collection run is a timestamp; each Chef version is a series line.
  const trendSeries: TrendSeries[] = (() => {
    if (!data || data.data.length === 0) return [];

    // Sort points by completed_at ascending.
    const sorted = [...data.data]
      .filter((pt) => pt.completed_at !== "")
      .sort(
        (a, b) =>
          new Date(a.completed_at).getTime() -
          new Date(b.completed_at).getTime(),
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
      <p className="mb-3 text-xs text-gray-500">
        Number of nodes running each Chef client version over time. Shows migration progress as nodes move to newer versions.
      </p>
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

export function ReadinessTrendCard({
  organisation,
}: {
  organisation?: string;
}) {
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

  useEffect(() => {
    load();
  }, [load]);

  // Build series from readiness trend points using real timestamps.
  // Each target Chef version gets lines for ready % and absolute counts.
  const { percentSeries, countSeries } = (() => {
    if (!data || data.data.length === 0) {
      return {
        percentSeries: [] as TrendSeries[],
        countSeries: [] as TrendSeries[],
      };
    }

    // Group by target_chef_version, filter to points with real timestamps.
    const byVersion = new Map<string, typeof data.data>();
    for (const pt of data.data) {
      if (!pt.completed_at || pt.completed_at === "") continue;
      const list = byVersion.get(pt.target_chef_version) ?? [];
      list.push(pt);
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

      // Sort by completed_at ascending.
      const sorted = [...points].sort(
        (a, b) =>
          new Date(a.completed_at).getTime() -
          new Date(b.completed_at).getTime(),
      );

      pctSeries.push({
        key: `ready-pct-${version}`,
        label: `Chef ${version} (% ready)`,
        colour,
        data: sorted.map((p) => ({
          timestamp: p.completed_at,
          value: p.ready_percent,
        })),
      });

      cntSeries.push(
        {
          key: `ready-${version}`,
          label: `Chef ${version} — Ready`,
          colour,
          data: sorted.map((p) => ({
            timestamp: p.completed_at,
            value: p.ready_nodes,
          })),
        },
        {
          key: `blocked-${version}`,
          label: `Chef ${version} — Blocked`,
          colour: "#ef4444",
          data: sorted.map((p) => ({
            timestamp: p.completed_at,
            value: p.blocked_nodes,
          })),
        },
      );
    }

    return { percentSeries: pctSeries, countSeries: cntSeries };
  })();

  return (
    <div className="card">
      <h3 className="card-header">Node Readiness — Trend</h3>
      <p className="mb-3 text-xs text-gray-500">
        Percentage of nodes where all cookbooks are CookStyle-compatible and disk space is sufficient. A node is "ready" when it can accept the target Chef version.
      </p>
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

export function ComplexityTrendCard({
  organisation,
}: {
  organisation?: string;
}) {
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

  useEffect(() => {
    load();
  }, [load]);

  // Build series from complexity trend points using real timestamps.
  const { scoreSeries, breakdownSeries } = (() => {
    if (!data || data.data.length === 0) {
      return {
        scoreSeries: [] as TrendSeries[],
        breakdownSeries: [] as TrendSeries[],
      };
    }

    // Group by target_chef_version, filter to points with real timestamps.
    const byVersion = new Map<string, typeof data.data>();
    for (const pt of data.data) {
      if (!pt.completed_at || pt.completed_at === "") continue;
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

    for (const [version, points] of byVersion) {
      const colour = colours[colourIdx % colours.length];
      colourIdx++;

      // Sort by completed_at ascending.
      const sorted = [...points].sort(
        (a, b) =>
          new Date(a.completed_at).getTime() -
          new Date(b.completed_at).getTime(),
      );

      sScore.push({
        key: `avg-score-${version}`,
        label: `Chef ${version} avg score`,
        colour,
        data: sorted.map((p) => ({
          timestamp: p.completed_at,
          value: Math.round(p.average_score * 10) / 10,
        })),
      });

      // Label breakdown — stacked counts from the latest point.
      if (sorted.length > 0) {
        const latest = sorted[sorted.length - 1];
        const ts = latest.completed_at;
        sBreakdown.push(
          {
            key: `low-${version}`,
            label: "Low",
            colour: "#22c55e",
            data: [{ timestamp: ts, value: latest.low_count }],
          },
          {
            key: `med-${version}`,
            label: "Medium",
            colour: "#f59e0b",
            data: [{ timestamp: ts, value: latest.medium_count }],
          },
          {
            key: `high-${version}`,
            label: "High",
            colour: "#ef4444",
            data: [{ timestamp: ts, value: latest.high_count }],
          },
          {
            key: `crit-${version}`,
            label: "Critical",
            colour: "#7c3aed",
            data: [{ timestamp: ts, value: latest.critical_count }],
          },
        );
      }
    }

    return { scoreSeries: sScore, breakdownSeries: sBreakdown };
  })();

  return (
    <div className="card">
      <h3 className="card-header">CookStyle Complexity — Trend</h3>
      <p className="mb-3 text-xs text-gray-500">
        Average CookStyle offence severity across all cookbooks. Lower is better — tracks progress as cookbooks are remediated.
      </p>
      {loading && <LoadingSpinner message="Loading complexity trend…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <div className="space-y-6">
          <TrendChart
            series={scoreSeries}
            yLabel="Avg offence severity"
            showArea={true}
            height={220}
          />
          {breakdownSeries.length > 0 && (
            <div>
              <h4 className="mb-2 text-sm font-semibold text-gray-700">
                Complexity Breakdown
              </h4>
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

export function StaleTrendCard({ organisation }: { organisation?: string }) {
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

  useEffect(() => {
    load();
  }, [load]);

  // Transform backend stale trend points into chart series.
  // Each collection run is a timestamp; we show stale and fresh as two lines.
  const trendSeries: TrendSeries[] = (() => {
    if (!data || data.data.length === 0) return [];

    const sorted = [...data.data]
      .filter((pt) => pt.completed_at !== "")
      .sort(
        (a, b) =>
          new Date(a.completed_at).getTime() -
          new Date(b.completed_at).getTime(),
      );

    if (sorted.length === 0) return [];

    return [
      {
        key: "critical",
        label: "Gone (Critical)",
        colour: "#dc2626", // red-600
        data: sorted.map((pt) => ({
          timestamp: pt.completed_at,
          value: pt.critical_nodes ?? pt.stale_nodes,
        })),
      },
      {
        key: "warning",
        label: "Missing (Warning)",
        colour: "#d97706", // amber-600
        data: sorted.map((pt) => ({
          timestamp: pt.completed_at,
          value: pt.warning_nodes ?? 0,
        })),
      },
      {
        key: "fresh",
        label: "Fresh",
        colour: "#16a34a", // green-600
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
      <p className="mb-3 text-xs text-gray-500">
        Nodes that have stopped checking in. "Missing" nodes may be temporarily offline; "Gone" nodes have not reported for an extended period.
      </p>
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
