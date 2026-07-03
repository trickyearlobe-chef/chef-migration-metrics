import { useState, useEffect, useCallback } from "react";
import {
  fetchVersionDistributionTrend,
  fetchReadinessTrend,
  fetchComplexityTrend,
  fetchCookstyleRecomputeTrend,
  fetchStaleTrend,
  fetchDeploymentTrend,
} from "../../api";
import type {
  VersionDistributionTrendResponse,
  ReadinessTrendResponse,
  ComplexityTrendResponse,
  CookstyleRecomputeTrendResponse,
  StaleTrendResponse,
  DeploymentTrendResponse,
} from "../../types";
import { LoadingSpinner, ErrorAlert } from "../../components/Feedback";
import { TrendChart, breakdownToSeries } from "../../components/TrendChart";
import type { TrendSeries } from "../../components/TrendChart";
import { useGlobalFilters } from "../../context/GlobalFilterContext";

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
  const { staleTiers } = useGlobalFilters();
  const staleParam = staleTiers.length > 0 ? staleTiers.join(",") : undefined;

  const [data, setData] = useState<ReadinessTrendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchReadinessTrend(organisation, staleParam)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation, staleParam]);

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

      // Needs-review series — only when the data actually carries it (the
      // review_blocks_readiness toggle is on for some collected points).
      if (sorted.some((p) => (p.needs_review_nodes ?? 0) > 0)) {
        cntSeries.push({
          key: `needs-review-${version}`,
          label: `Chef ${version} — Needs review`,
          colour: "#f59e0b",
          data: sorted.map((p) => ({
            timestamp: p.completed_at,
            value: p.needs_review_nodes ?? 0,
          })),
        });
      }
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
// CookStyle Rollup Recompute Trend Card
// ---------------------------------------------------------------------------

// Unlike the other cards (which read frozen per-collection aggregates), this card
// reads the recompute-trend endpoint: the CookStyle rollup re-derived from offence
// fingerprints under TODAY's classification, in Ready/Needs review/Blocked/Untested
// vocabulary. Points before `recompute_available_from` cannot be recomputed, so we
// surface that frozen/recomputable boundary explicitly rather than implying the
// whole series reflects current criteria.
export function CookstyleRecomputeTrendCard() {
  const [data, setData] = useState<CookstyleRecomputeTrendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchCookstyleRecomputeTrend()
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  // buildSeries produces the 4 rollup series per target version for one source
  // ("server" cookbooks or "git" repos), so each source charts independently.
  const buildSeries = (source: string): TrendSeries[] => {
    if (!data || data.data.length === 0) return [];

    const byVersion = new Map<string, typeof data.data>();
    for (const pt of data.data) {
      if (!pt.completed_at || pt.source !== source) continue;
      const list = byVersion.get(pt.target_chef_version) ?? [];
      list.push(pt);
      byVersion.set(pt.target_chef_version, list);
    }

    const series: TrendSeries[] = [];
    for (const [version, points] of byVersion) {
      const sorted = [...points].sort(
        (a, b) =>
          new Date(a.completed_at).getTime() -
          new Date(b.completed_at).getTime(),
      );
      series.push(
        {
          key: `recompute-${source}-ready-${version}`,
          label: `Chef ${version} — Ready`,
          colour: "#22c55e",
          data: sorted.map((p) => ({ timestamp: p.completed_at, value: p.ready })),
        },
        {
          key: `recompute-${source}-needs-review-${version}`,
          label: `Chef ${version} — Needs review`,
          colour: "#f59e0b",
          data: sorted.map((p) => ({
            timestamp: p.completed_at,
            value: p.needs_review,
          })),
        },
        {
          key: `recompute-${source}-blocked-${version}`,
          label: `Chef ${version} — Blocked`,
          colour: "#ef4444",
          data: sorted.map((p) => ({ timestamp: p.completed_at, value: p.blocked })),
        },
        {
          key: `recompute-${source}-untested-${version}`,
          label: `Chef ${version} — Untested`,
          colour: "#9ca3af",
          data: sorted.map((p) => ({
            timestamp: p.completed_at,
            value: p.untested,
          })),
        },
      );
    }
    return series;
  };

  const serverSeries = buildSeries("server");
  const gitSeries = buildSeries("git");

  const boundaryNote = (() => {
    if (!data) return null;
    if (!data.recompute_available_from) {
      return "No fingerprint history yet — recomputed trend begins once scans capture offence fingerprints.";
    }
    const from = new Date(data.recompute_available_from);
    return `Recomputed under current classification from ${from.toLocaleString()}. Earlier points are frozen and cannot be recomputed.`;
  })();

  return (
    <div className="card">
      <h3 className="card-header">CookStyle Rollup — Recomputed Trend</h3>
      <p className="mb-3 text-xs text-gray-500">
        Cookbooks/repos by rollup status (Ready / Needs review / Blocked /
        Untested), re-derived from offence-fingerprint history under the current
        cop classification — so a reclassification is reflected across the whole
        series without a rescan. Server cookbooks and git repos are charted
        separately.
      </p>
      {loading && <LoadingSpinner message="Loading recomputed CookStyle trend…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <div className="space-y-4">
          <div className="space-y-1" data-testid="recompute-server-chart">
            <p className="text-xs font-medium text-gray-600">Server cookbooks</p>
            <TrendChart
              series={serverSeries}
              yLabel="Result count"
              showArea={false}
              height={200}
            />
          </div>
          <div className="space-y-1" data-testid="recompute-git-chart">
            <p className="text-xs font-medium text-gray-600">Git repos</p>
            <TrendChart
              series={gitSeries}
              yLabel="Result count"
              showArea={false}
              height={200}
            />
          </div>
          {boundaryNote && (
            <p className="text-xs italic text-gray-500" data-testid="recompute-boundary-note">
              {boundaryNote}
            </p>
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

// ---------------------------------------------------------------------------
// Deployment Progress Trend Card (historical)
// ---------------------------------------------------------------------------

export function DeploymentTrendCard({
  organisation,
}: {
  organisation?: string;
}) {
  const [data, setData] = useState<DeploymentTrendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchDeploymentTrend(organisation)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => {
    load();
  }, [load]);

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

    // If per-version data is available, render per-version series.
    const hasPerVersion = sorted.some((pt) => pt.by_version && Object.keys(pt.by_version).length > 0);

    if (hasPerVersion) {
      const versions = new Set<string>();
      for (const pt of sorted) {
        if (pt.by_version) {
          for (const v of Object.keys(pt.by_version)) versions.add(v);
        }
      }

      const colours = ["#3b82f6", "#22c55e", "#f59e0b", "#8b5cf6", "#ef4444", "#06b6d4"];
      const series: TrendSeries[] = [];
      let idx = 0;

      for (const version of [...versions].sort()) {
        const colour = colours[idx % colours.length];
        idx++;
        series.push(
          {
            key: `deployed-${version}`,
            label: `${version} Staged/Activated`,
            colour,
            data: sorted.map((pt) => ({
              timestamp: pt.completed_at,
              value: pt.by_version?.[version]?.staged_or_activated ?? 0,
            })),
          },
          {
            key: `converge-${version}`,
            label: `${version} Converge Passing`,
            colour: colour + "80", // semi-transparent variant
            data: sorted.map((pt) => ({
              timestamp: pt.completed_at,
              value: pt.by_version?.[version]?.converge_passing ?? 0,
            })),
          },
        );
      }

      return series;
    }

    // Fallback: aggregate series (backward-compat).
    return [
      {
        key: "staged-or-activated",
        label: "Staged or Activated",
        colour: "#3b82f6",
        data: sorted.map((pt) => ({
          timestamp: pt.completed_at,
          value: pt.staged_or_activated,
        })),
      },
      {
        key: "converge-passing",
        label: "Speculative Converge Passing",
        colour: "#22c55e",
        data: sorted.map((pt) => ({
          timestamp: pt.completed_at,
          value: pt.converge_passing,
        })),
      },
    ];
  })();

  return (
    <div className="card">
      <h3 className="card-header">Deployment Progress — Trend</h3>
      <p className="mb-3 text-xs text-gray-500">
        Nodes with the target version staged or activated vs. nodes where the nightly speculative converge is passing. The gap represents nodes needing investigation.
      </p>
      {loading && <LoadingSpinner message="Loading deployment trend…" />}
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
