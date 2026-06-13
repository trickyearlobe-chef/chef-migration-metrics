// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Two stacked, same-scale bars that make disk readiness easy to reason about:
//   • "On disk now"      — used + free (= total volume capacity)
//   • "Needed to upgrade" — used + install size + the min-free-% buffer
// Both bars share one scale (the larger of the two), so when the requirement
// exceeds the volume capacity the bottom bar visibly overflows past the top
// bar's end — and that overflow IS the shortfall (shown in red). When it fits,
// the bottom bar ends at the capacity line with any leftover free space.

export type DiskSegmentKind =
  | "used"
  | "free"
  | "install"
  | "buffer"
  | "freeAfter";

export interface DiskSegment {
  kind: DiskSegmentKind;
  mb: number;
}

export interface DiskBars {
  scaleMB: number; // common denominator for both bars
  capacityMB: number; // the actual volume total
  bufferMB: number;
  shortfallMB: number;
  current: DiskSegment[]; // top bar
  required: DiskSegment[]; // bottom bar
}

/**
 * Builds the two-bar model from the install-path mount's figures. Returns null
 * when the inputs are insufficient to draw bars (missing total/available/required
 * — e.g. indeterminate or stale verdict), so the caller can fall back.
 */
export function computeDiskBars(opts: {
  totalMB?: number | null;
  availableMB?: number | null;
  requiredMB?: number | null;
  minRemainingFreePercent?: number | null;
}): DiskBars | null {
  const { totalMB, availableMB, requiredMB } = opts;
  if (totalMB == null || availableMB == null || requiredMB == null) return null;
  if (totalMB <= 0) return null;

  const minPct = opts.minRemainingFreePercent ?? 0;
  const total = totalMB;
  const free = Math.max(0, Math.min(availableMB, total));
  const used = Math.max(0, total - free);
  const required = Math.max(0, requiredMB);
  const buffer = minPct > 0 ? Math.round((minPct / 100) * total) : 0;

  const need = used + required + buffer;
  const scaleMB = Math.max(total, need);
  const shortfallMB = Math.max(0, need - total);
  const freeAfter = Math.max(0, total - need);

  // The requirement bar is one contiguous block: used + install + the full
  // buffer. When need > capacity it simply extends past the capacity line; the
  // overflow is buffer that doesn't fit (no separate "shortfall" colour). When
  // it fits, a trailing "free after" segment fills to the capacity line.
  const current: DiskSegment[] = [
    { kind: "used", mb: used },
    { kind: "free", mb: free },
  ];

  const requiredBar: DiskSegment[] = [
    { kind: "used", mb: used },
    { kind: "install", mb: required },
    { kind: "buffer", mb: buffer },
  ];
  if (freeAfter > 0) requiredBar.push({ kind: "freeAfter", mb: freeAfter });

  return {
    scaleMB,
    capacityMB: total,
    bufferMB: buffer,
    shortfallMB,
    current,
    required: requiredBar,
  };
}

function fmt(mb: number): string {
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
  return `${Math.round(mb)} MB`;
}

const SEGMENT_STYLE: Record<DiskSegmentKind, { colour: string; label: string }> = {
  used: { colour: "bg-gray-400", label: "Used" },
  free: { colour: "bg-green-400", label: "Free" },
  install: { colour: "bg-blue-500", label: "Install" },
  buffer: { colour: "bg-amber-400", label: "Buffer" },
  freeAfter: { colour: "bg-green-400", label: "Free after" },
};

function Bar({
  segments,
  scaleMB,
  capacityLinePct,
}: {
  segments: DiskSegment[];
  scaleMB: number;
  // When set, draws a dashed vertical "volume capacity" line at this % — anything
  // to the right of it is the part of the requirement that doesn't fit.
  capacityLinePct?: number;
}) {
  return (
    <div className="relative">
      <div className="flex h-4 w-full overflow-hidden rounded bg-gray-100">
        {segments.map((s, i) =>
          s.mb <= 0 ? null : (
            <div
              key={`${s.kind}-${i}`}
              className={SEGMENT_STYLE[s.kind].colour}
              style={{ width: `${(s.mb / scaleMB) * 100}%` }}
              title={`${SEGMENT_STYLE[s.kind].label}: ${fmt(s.mb)}`}
            />
          ),
        )}
      </div>
      {capacityLinePct != null && (
        <div
          className="pointer-events-none absolute -top-0.5 -bottom-0.5 border-l-2 border-dashed border-gray-700"
          style={{ left: `${capacityLinePct}%` }}
          title="Volume capacity"
        />
      )}
    </div>
  );
}

export function DiskUsageBars({ bars }: { bars: DiskBars }) {
  const { scaleMB, capacityMB, bufferMB, shortfallMB } = bars;
  const fits = shortfallMB === 0;
  // Only meaningful when the requirement overflows: marks where the volume ends.
  const capacityLinePct = fits ? undefined : (capacityMB / scaleMB) * 100;

  return (
    <div className="mt-2 space-y-2 text-xs">
      <div>
        <div className="mb-0.5 flex justify-between text-gray-500">
          <span>On disk now</span>
          <span className="text-gray-400">{fmt(capacityMB)} total</span>
        </div>
        <Bar segments={bars.current} scaleMB={scaleMB} />
      </div>

      <div>
        <div className="mb-0.5 flex justify-between text-gray-500">
          <span>Needed to upgrade</span>
          {fits ? (
            <span className="text-green-600">fits</span>
          ) : (
            <span className="font-medium text-red-600">
              {fmt(shortfallMB)} over capacity
            </span>
          )}
        </div>
        <Bar
          segments={bars.required}
          scaleMB={scaleMB}
          capacityLinePct={capacityLinePct}
        />
      </div>

      {/* Legend */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-gray-500">
        <Legend kind="used" />
        <Legend kind="install" />
        {bufferMB > 0 && <Legend kind="buffer" />}
        <Legend kind="free" />
        {!fits && (
          <span className="inline-flex items-center gap-1">
            <span className="inline-block h-2.5 w-0 border-l-2 border-dashed border-gray-700" />
            Capacity
          </span>
        )}
      </div>
    </div>
  );
}

function Legend({ kind }: { kind: DiskSegmentKind }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span className={`inline-block h-2.5 w-2.5 rounded-sm ${SEGMENT_STYLE[kind].colour}`} />
      {SEGMENT_STYLE[kind].label}
    </span>
  );
}
