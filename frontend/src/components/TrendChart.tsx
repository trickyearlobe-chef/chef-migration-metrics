// ---------------------------------------------------------------------------
// TrendChart — a reusable SVG line/area chart component for rendering
// time-series data with multiple series. No external charting library
// required — pure React + SVG.
//
// Features:
//   • Multiple coloured series with optional area fills
//   • Auto-scaling Y axis with human-friendly tick marks
//   • X-axis date labels with intelligent tick spacing
//   • Hover tooltip showing values for all series at a given point
//   • Responsive width via a container query
//   • Graceful empty state when no data is available
// ---------------------------------------------------------------------------

import { useState, useRef, useMemo, useCallback } from "react";

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

export interface TrendDataPoint {
  /** ISO-8601 timestamp or any string parseable by `new Date()` */
  timestamp: string;
  /** Numeric value at this point in time */
  value: number;
}

export interface TrendSeries {
  /** Unique key for the series */
  key: string;
  /** Human-readable label shown in the legend and tooltip */
  label: string;
  /** CSS colour string for the line/area */
  colour: string;
  /** Data points — must be sorted by timestamp ascending */
  data: TrendDataPoint[];
}

export interface TrendChartProps {
  /** One or more series to render */
  series: TrendSeries[];
  /** Chart title shown above the SVG (optional) */
  title?: string;
  /** Y-axis label (optional) */
  yLabel?: string;
  /** Fixed Y maximum — if omitted the chart auto-scales */
  yMax?: number;
  /** Whether to fill the area under each line (default: true) */
  showArea?: boolean;
  /** Height of the SVG in pixels (default: 220) */
  height?: number;
  /** Format a Y-axis value for display (default: locale string) */
  formatY?: (v: number) => string;
  /** Format a date for the X-axis tick labels */
  formatX?: (d: Date) => string;
  /** Show percentage on Y axis instead of raw numbers */
  isPercent?: boolean;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Pick "nice" Y-axis tick values that look good on a chart. */
function niceYTicks(min: number, max: number, approxCount = 5): number[] {
  if (max <= min) return [0];
  const range = max - min;
  const roughStep = range / approxCount;

  // Find a "nice" step: 1, 2, 5, 10, 20, 50, 100 …
  const mag = Math.pow(10, Math.floor(Math.log10(roughStep)));
  const residual = roughStep / mag;
  let niceStep: number;
  if (residual <= 1.5) niceStep = 1 * mag;
  else if (residual <= 3) niceStep = 2 * mag;
  else if (residual <= 7) niceStep = 5 * mag;
  else niceStep = 10 * mag;

  if (niceStep === 0) return [0];

  const start = Math.floor(min / niceStep) * niceStep;
  const ticks: number[] = [];
  for (let v = start; v <= max + niceStep * 0.01; v += niceStep) {
    ticks.push(Math.round(v * 1e6) / 1e6); // avoid float drift
  }
  return ticks;
}

/** Default X-axis date formatter — short month + day. */
function defaultFormatX(d: Date): string {
  const months = [
    "Jan", "Feb", "Mar", "Apr", "May", "Jun",
    "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
  ];
  return `${months[d.getMonth()]} ${d.getDate()}`;
}

/** Default Y value formatter. */
function defaultFormatY(v: number): string {
  if (v >= 1_000_000) return (v / 1_000_000).toFixed(1) + "M";
  if (v >= 1_000) return (v / 1_000).toFixed(1) + "k";
  return v.toLocaleString(undefined, { maximumFractionDigits: 1 });
}

function percentFormatY(v: number): string {
  return v.toFixed(1) + "%";
}

/** Build an SVG polyline/polygon point string. */
function pointsToString(pts: Array<{ x: number; y: number }>): string {
  return pts.map((p) => `${p.x},${p.y}`).join(" ");
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TrendChart({
  series,
  title,
  yLabel,
  yMax: yMaxProp,
  showArea = true,
  height = 220,
  formatY,
  formatX,
  isPercent = false,
}: TrendChartProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const [containerWidth, setContainerWidth] = useState(600);

  // Observe container width for responsiveness.
  const resizeObserver = useRef<ResizeObserver | null>(null);
  const containerCallbackRef = useCallback(
    (node: HTMLDivElement | null) => {
      if (resizeObserver.current) {
        resizeObserver.current.disconnect();
        resizeObserver.current = null;
      }
      if (node) {
        (containerRef as React.MutableRefObject<HTMLDivElement | null>).current = node;
        setContainerWidth(node.clientWidth);
        resizeObserver.current = new ResizeObserver((entries) => {
          for (const entry of entries) {
            setContainerWidth(entry.contentRect.width);
          }
        });
        resizeObserver.current.observe(node);
      }
    },
    [],
  );

  // Resolve formatters.
  const fmtY = formatY ?? (isPercent ? percentFormatY : defaultFormatY);
  const fmtX = formatX ?? defaultFormatX;

  // ---------------------------------------------------------------------------
  // Compute unified X timestamps across all series
  // ---------------------------------------------------------------------------
  const { timestamps, seriesPoints, yMinVal, yMaxVal } = useMemo(() => {
    // Collect unique timestamps, sorted ascending.
    const tsSet = new Set<string>();
    for (const s of series) {
      for (const pt of s.data) {
        tsSet.add(pt.timestamp);
      }
    }
    const sortedTs = Array.from(tsSet).sort(
      (a, b) => new Date(a).getTime() - new Date(b).getTime(),
    );

    // Build per-series lookup: timestamp → value (or null if missing).
    const sp: Array<{
      key: string;
      label: string;
      colour: string;
      values: Array<number | null>;
    }> = series.map((s) => {
      const lookup = new Map<string, number>();
      for (const pt of s.data) lookup.set(pt.timestamp, pt.value);
      return {
        key: s.key,
        label: s.label,
        colour: s.colour,
        values: sortedTs.map((ts) => lookup.get(ts) ?? null),
      };
    });

    // Find Y range.
    let minV = Infinity;
    let maxV = -Infinity;
    for (const s of sp) {
      for (const v of s.values) {
        if (v !== null) {
          if (v < minV) minV = v;
          if (v > maxV) maxV = v;
        }
      }
    }
    if (minV === Infinity) {
      minV = 0;
      maxV = isPercent ? 100 : 10;
    }

    // Ensure a sensible range.
    if (maxV === minV) {
      maxV = minV + (isPercent ? 10 : Math.max(1, minV * 0.1));
    }

    return {
      timestamps: sortedTs,
      seriesPoints: sp,
      yMinVal: isPercent ? 0 : Math.max(0, minV),
      yMaxVal: yMaxProp ?? (isPercent ? 100 : maxV * 1.1),
    };
  }, [series, yMaxProp, isPercent]);

  // ---------------------------------------------------------------------------
  // Layout constants
  // ---------------------------------------------------------------------------
  const margin = { top: 12, right: 16, bottom: 36, left: 52 };
  const svgWidth = Math.max(containerWidth, 300);
  const svgHeight = height;
  const plotW = svgWidth - margin.left - margin.right;
  const plotH = svgHeight - margin.top - margin.bottom;

  // ---------------------------------------------------------------------------
  // Scales
  // ---------------------------------------------------------------------------
  const xScale = useCallback(
    (i: number) => {
      if (timestamps.length <= 1) return margin.left + plotW / 2;
      return margin.left + (i / (timestamps.length - 1)) * plotW;
    },
    [timestamps.length, margin.left, plotW],
  );

  const yScale = useCallback(
    (v: number) => {
      const ratio = (v - yMinVal) / (yMaxVal - yMinVal);
      return margin.top + plotH - ratio * plotH;
    },
    [yMinVal, yMaxVal, margin.top, plotH],
  );

  // ---------------------------------------------------------------------------
  // Ticks
  // ---------------------------------------------------------------------------
  const yTicks = useMemo(() => niceYTicks(yMinVal, yMaxVal), [yMinVal, yMaxVal]);

  const xTickIndices = useMemo(() => {
    const n = timestamps.length;
    if (n === 0) return [];
    if (n <= 8) return timestamps.map((_, i) => i);
    const step = Math.max(1, Math.floor(n / 7));
    const indices: number[] = [];
    for (let i = 0; i < n; i += step) indices.push(i);
    if (indices[indices.length - 1] !== n - 1) indices.push(n - 1);
    return indices;
  }, [timestamps]);

  // ---------------------------------------------------------------------------
  // Hover handling — map mouse X to closest data index
  // ---------------------------------------------------------------------------
  const onMouseMove = useCallback(
    (e: React.MouseEvent<SVGSVGElement>) => {
      if (timestamps.length === 0) return;
      const svgRect = e.currentTarget.getBoundingClientRect();
      const mouseX = e.clientX - svgRect.left;

      // Find the closest timestamp index.
      let closest = 0;
      let minDist = Infinity;
      for (let i = 0; i < timestamps.length; i++) {
        const dist = Math.abs(xScale(i) - mouseX);
        if (dist < minDist) {
          minDist = dist;
          closest = i;
        }
      }
      setHoverIndex(closest);
    },
    [timestamps, xScale],
  );

  const onMouseLeave = useCallback(() => setHoverIndex(null), []);

  // ---------------------------------------------------------------------------
  // Empty state
  // ---------------------------------------------------------------------------
  const hasData = series.length > 0 && timestamps.length > 0;

  if (!hasData) {
    return (
      <div ref={containerCallbackRef} className="w-full">
        {title && (
          <h4 className="mb-2 text-sm font-semibold text-gray-700">{title}</h4>
        )}
        <div className="flex items-center justify-center rounded-lg border border-dashed border-gray-300 bg-gray-50 py-12 text-sm text-gray-400">
          No trend data available yet — data will appear after multiple collection runs.
        </div>
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Build SVG paths for each series
  // ---------------------------------------------------------------------------
  const seriesPaths = seriesPoints.map((s) => {
    const linePoints: Array<{ x: number; y: number }> = [];
    for (let i = 0; i < s.values.length; i++) {
      const v = s.values[i];
      if (v !== null) {
        linePoints.push({ x: xScale(i), y: yScale(v) });
      }
    }

    // Area polygon: close the path down to the X-axis.
    const areaPoints =
      showArea && linePoints.length > 1
        ? [
          ...linePoints,
          { x: linePoints[linePoints.length - 1].x, y: yScale(yMinVal) },
          { x: linePoints[0].x, y: yScale(yMinVal) },
        ]
        : [];

    return { key: s.key, colour: s.colour, linePoints, areaPoints };
  });

  // ---------------------------------------------------------------------------
  // Tooltip content
  // ---------------------------------------------------------------------------
  const tooltipContent =
    hoverIndex !== null
      ? {
        date: fmtX(new Date(timestamps[hoverIndex])),
        values: seriesPoints.map((s) => ({
          label: s.label,
          colour: s.colour,
          value: s.values[hoverIndex],
        })),
        x: xScale(hoverIndex),
      }
      : null;

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------
  return (
    <div ref={containerCallbackRef} className="w-full">
      {title && (
        <h4 className="mb-2 text-sm font-semibold text-gray-700">{title}</h4>
      )}

      {/* Legend */}
      {series.length > 1 && (
        <div className="mb-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-600">
          {series.map((s) => (
            <span key={s.key} className="flex items-center gap-1">
              <span
                className="inline-block h-2.5 w-2.5 rounded-full"
                style={{ backgroundColor: s.colour }}
              />
              {s.label}
            </span>
          ))}
        </div>
      )}

      <svg
        width={svgWidth}
        height={svgHeight}
        className="select-none overflow-visible"
        onMouseMove={onMouseMove}
        onMouseLeave={onMouseLeave}
      >
        {/* ---------- Grid lines ---------- */}
        {yTicks.map((tick) => (
          <line
            key={`ygrid-${tick}`}
            x1={margin.left}
            x2={svgWidth - margin.right}
            y1={yScale(tick)}
            y2={yScale(tick)}
            stroke="#e5e7eb"
            strokeDasharray="4 3"
          />
        ))}

        {/* ---------- Y-axis ticks & labels ---------- */}
        {yTicks.map((tick) => (
          <text
            key={`ylabel-${tick}`}
            x={margin.left - 8}
            y={yScale(tick)}
            textAnchor="end"
            dominantBaseline="middle"
            className="fill-gray-400 text-[10px]"
          >
            {fmtY(tick)}
          </text>
        ))}

        {/* Y-axis label */}
        {yLabel && (
          <text
            x={14}
            y={margin.top + plotH / 2}
            textAnchor="middle"
            dominantBaseline="middle"
            transform={`rotate(-90, 14, ${margin.top + plotH / 2})`}
            className="fill-gray-400 text-[10px]"
          >
            {yLabel}
          </text>
        )}

        {/* ---------- X-axis ticks & labels ---------- */}
        {xTickIndices.map((i) => (
          <text
            key={`xlabel-${i}`}
            x={xScale(i)}
            y={svgHeight - margin.bottom + 18}
            textAnchor="middle"
            className="fill-gray-400 text-[10px]"
          >
            {fmtX(new Date(timestamps[i]))}
          </text>
        ))}

        {/* ---------- Axes ---------- */}
        <line
          x1={margin.left}
          x2={margin.left}
          y1={margin.top}
          y2={margin.top + plotH}
          stroke="#d1d5db"
        />
        <line
          x1={margin.left}
          x2={svgWidth - margin.right}
          y1={margin.top + plotH}
          y2={margin.top + plotH}
          stroke="#d1d5db"
        />

        {/* ---------- Area fills ---------- */}
        {showArea &&
          seriesPaths.map(
            (sp) =>
              sp.areaPoints.length > 0 && (
                <polygon
                  key={`area-${sp.key}`}
                  points={pointsToString(sp.areaPoints)}
                  fill={sp.colour}
                  fillOpacity={0.1}
                />
              ),
          )}

        {/* ---------- Lines ---------- */}
        {seriesPaths.map(
          (sp) =>
            sp.linePoints.length > 1 && (
              <polyline
                key={`line-${sp.key}`}
                points={pointsToString(sp.linePoints)}
                fill="none"
                stroke={sp.colour}
                strokeWidth={2}
                strokeLinejoin="round"
                strokeLinecap="round"
              />
            ),
        )}

        {/* ---------- Data points (dots) ---------- */}
        {seriesPaths.map((sp) =>
          sp.linePoints.map((pt, i) => (
            <circle
              key={`dot-${sp.key}-${i}`}
              cx={pt.x}
              cy={pt.y}
              r={timestamps.length <= 20 ? 3 : 2}
              fill={sp.colour}
              stroke="white"
              strokeWidth={1.5}
            />
          )),
        )}

        {/* ---------- Hover indicator ---------- */}
        {hoverIndex !== null && (
          <>
            <line
              x1={xScale(hoverIndex)}
              x2={xScale(hoverIndex)}
              y1={margin.top}
              y2={margin.top + plotH}
              stroke="#9ca3af"
              strokeWidth={1}
              strokeDasharray="4 3"
            />
            {/* Highlight dots at hover position */}
            {seriesPoints.map((s) => {
              const v = s.values[hoverIndex];
              if (v === null) return null;
              return (
                <circle
                  key={`hover-${s.key}`}
                  cx={xScale(hoverIndex)}
                  cy={yScale(v)}
                  r={5}
                  fill={s.colour}
                  stroke="white"
                  strokeWidth={2}
                />
              );
            })}
          </>
        )}

        {/* ---------- Tooltip ---------- */}
        {tooltipContent && (
          <foreignObject
            x={Math.min(
              tooltipContent.x + 12,
              svgWidth - margin.right - 160,
            )}
            y={margin.top + 4}
            width={150}
            height={20 + tooltipContent.values.length * 20 + 8}
            className="pointer-events-none"
          >
            <div className="rounded-md border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-lg">
              <div className="mb-1 font-semibold text-gray-700">
                {tooltipContent.date}
              </div>
              {tooltipContent.values.map((v) => (
                <div
                  key={v.label}
                  className="flex items-center justify-between gap-3"
                >
                  <span className="flex items-center gap-1 text-gray-600">
                    <span
                      className="inline-block h-2 w-2 rounded-full"
                      style={{ backgroundColor: v.colour }}
                    />
                    {v.label}
                  </span>
                  <span className="font-medium text-gray-800">
                    {v.value !== null ? fmtY(v.value) : "—"}
                  </span>
                </div>
              ))}
            </div>
          </foreignObject>
        )}
      </svg>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Convenience: a stacked area variant for version distribution trends where
// each "version" is a series stacked on top of the previous ones.
// ---------------------------------------------------------------------------

export interface StackedTrendInput {
  /** ISO timestamp for the data point */
  timestamp: string;
  /** version_name → count */
  breakdown: Record<string, number>;
}

const STACK_COLOURS = [
  "#3b82f6", // blue-500
  "#22c55e", // green-500
  "#f59e0b", // amber-500
  "#ef4444", // red-500
  "#8b5cf6", // violet-500
  "#06b6d4", // cyan-500
  "#ec4899", // pink-500
  "#f97316", // orange-500
  "#14b8a6", // teal-500
  "#6366f1", // indigo-500
];

/**
 * Given timestamped breakdowns (e.g. version distributions per collection
 * run), produce `TrendSeries[]` suitable for `<TrendChart />`.
 *
 * Each version becomes its own series showing its count over time.
 */
export function breakdownToSeries(
  points: StackedTrendInput[],
): TrendSeries[] {
  // Collect all unique keys across all points.
  const keySet = new Set<string>();
  for (const pt of points) {
    for (const k of Object.keys(pt.breakdown)) keySet.add(k);
  }

  // Sort keys for stable colour assignment — put "unknown" last.
  const keys = Array.from(keySet).sort((a, b) => {
    if (a === "unknown") return 1;
    if (b === "unknown") return -1;
    return a.localeCompare(b, undefined, { numeric: true });
  });

  return keys.map((key, idx) => ({
    key,
    label: key,
    colour: STACK_COLOURS[idx % STACK_COLOURS.length],
    data: points
      .filter((pt) => pt.breakdown[key] !== undefined)
      .map((pt) => ({
        timestamp: pt.timestamp,
        value: pt.breakdown[key],
      })),
  }));
}
