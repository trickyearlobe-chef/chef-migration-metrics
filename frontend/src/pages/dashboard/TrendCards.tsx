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
import {
  TrendChart,
  breakdownToSeries,
} from "../../components/TrendChart";
import type { TrendSeries } from "../../components/TrendChart";

// ---------------------------------------------------------------------------
// Version Distribution Trend Card (historical)
// ---------------------------------------------------------------------------

export function VersionDistributionTrendCard({ organisation }: { organisation?: string }) {
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

export function ReadinessTrendCard({ organisation }: { organisation?: string }) {
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

export function ComplexityTrendCard({ organisation }: { organisation?: string }) {
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
