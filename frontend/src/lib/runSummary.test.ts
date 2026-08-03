// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import {
  runDurationSeconds,
  formatDuration,
  runResourceSummary,
} from "./runSummary";

// Both timestamps come from chef-client's own clock, so the difference is the
// run's elapsed time rather than anything CMM measured or inferred.

describe("runDurationSeconds", () => {
  it("measures between the run's own start and end", () => {
    expect(
      runDurationSeconds({
        start_time: "2026-08-03T10:00:00Z",
        end_time: "2026-08-03T10:03:04Z",
      }),
    ).toBe(184);
  });

  // The producers send offsets, so the same instant written two ways is the
  // same instant. This is the case that would break a naive string compare.
  it("is not confused by different time zone offsets", () => {
    expect(
      runDurationSeconds({
        start_time: "2026-08-03T11:00:00+01:00",
        end_time: "2026-08-03T10:03:04Z",
      }),
    ).toBe(184);
  });

  it("has no duration when the run never reported a start", () => {
    expect(
      runDurationSeconds({ start_time: null, end_time: "2026-08-03T10:03:04Z" }),
    ).toBeNull();
  });

  // Clocks that disagree produce a negative span. Showing "-4s" would be
  // asserting something impossible; showing nothing is honest.
  it("reports nothing rather than a negative duration", () => {
    expect(
      runDurationSeconds({
        start_time: "2026-08-03T10:03:04Z",
        end_time: "2026-08-03T10:00:00Z",
      }),
    ).toBeNull();
  });
});

describe("formatDuration", () => {
  it.each([
    [0.4, "<1s"],
    [1, "1s"],
    [59, "59s"],
    [60, "1m"],
    [184, "3m 4s"],
    [3600, "1h"],
    [3725, "1h 2m"],
  ])("%ss reads as %s", (seconds, want) => {
    expect(formatDuration(seconds)).toBe(want);
  });

  it("says nothing when there is no duration", () => {
    expect(formatDuration(null)).toBe("");
  });
});

describe("runResourceSummary", () => {
  it("says what changed, out of what was managed, and how long it took", () => {
    expect(
      runResourceSummary({
        start_time: "2026-08-03T10:00:00Z",
        end_time: "2026-08-03T10:03:04Z",
        updated_resource_count: 12,
        total_resource_count: 108,
      }),
    ).toBe("12/108 in 3m 4s");
  });

  it("still gives the counts when the run has no usable timing", () => {
    expect(
      runResourceSummary({
        updated_resource_count: 0,
        total_resource_count: 108,
      }),
    ).toBe("0/108");
  });

  // "0/0" would assert a run that managed nothing, which is a claim. An absent
  // count means the run did not tell us.
  it("says nothing rather than 0/0 when no counts were reported", () => {
    expect(
      runResourceSummary({
        start_time: "2026-08-03T10:00:00Z",
        end_time: "2026-08-03T10:00:20Z",
      }),
    ).toBe("in 20s");
    expect(runResourceSummary({})).toBe("");
  });
});
