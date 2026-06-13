// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { computeDiskBars, type DiskSegment } from "./DiskUsageBars";

const mb = (segs: DiskSegment[], kind: DiskSegment["kind"]) =>
  segs.find((s) => s.kind === kind)?.mb;

describe("computeDiskBars", () => {
  // homekube001: passes the absolute install size but fails the 20% buffer, so
  // the requirement bar overflows the volume capacity — the overflow is the
  // real shortfall (the old "required - available" gave a nonsensical -4148).
  it("models the buffer overflow as a shortfall, not a negative", () => {
    const bars = computeDiskBars({
      totalMB: 28396,
      availableMB: 7220,
      requiredMB: 3072,
      minRemainingFreePercent: 20,
    })!;
    expect(bars).not.toBeNull();
    expect(bars.capacityMB).toBe(28396);
    expect(bars.bufferMB).toBe(5679); // round(0.20 * 28396)
    expect(bars.shortfallMB).toBe(1531); // need(29927) - total(28396)
    expect(bars.scaleMB).toBe(29927); // bottom bar longer than the volume
    // Top bar: used + free = total.
    expect(mb(bars.current, "used")).toBe(21176);
    expect(mb(bars.current, "free")).toBe(7220);
    // Bottom bar is one contiguous block: used + install + FULL buffer (no
    // separate shortfall segment), no free-after. The buffer overflows past the
    // capacity line; the bar's length (= scale) exceeds the volume capacity.
    expect(mb(bars.required, "install")).toBe(3072);
    expect(mb(bars.required, "buffer")).toBe(5679); // full buffer, not split
    expect(mb(bars.required, "freeAfter")).toBeUndefined();
    const sum = bars.required.reduce((a, s) => a + s.mb, 0);
    expect(sum).toBe(bars.scaleMB); // bottom bar fills the (overflowing) scale
    expect(sum).toBeGreaterThan(bars.capacityMB); // ...past the volume capacity
  });

  it("fits: requirement bar ends at capacity with leftover free-after, no shortfall", () => {
    const bars = computeDiskBars({
      totalMB: 28396,
      availableMB: 15000,
      requiredMB: 3072,
      minRemainingFreePercent: 20,
    })!;
    expect(bars.shortfallMB).toBe(0);
    expect(bars.scaleMB).toBe(28396); // same scale as capacity → bars equal length
    expect(mb(bars.required, "freeAfter")).toBe(6249); // 28396 - (13396+3072+5679)
    // Segments sum to the volume capacity.
    const sum = bars.required.reduce((a, s) => a + s.mb, 0);
    expect(sum).toBe(28396);
  });

  it("no buffer when min-free-% is 0", () => {
    const bars = computeDiskBars({
      totalMB: 10240,
      availableMB: 5000,
      requiredMB: 3072,
      minRemainingFreePercent: 0,
    })!;
    expect(bars.bufferMB).toBe(0);
    expect(bars.shortfallMB).toBe(0); // 5000 free covers 3072 install with 0 buffer
  });

  it("returns null when the install-path total is unavailable", () => {
    expect(
      computeDiskBars({ totalMB: null, availableMB: 7220, requiredMB: 3072 }),
    ).toBeNull();
    expect(
      computeDiskBars({ totalMB: 0, availableMB: 7220, requiredMB: 3072 }),
    ).toBeNull();
  });
});
