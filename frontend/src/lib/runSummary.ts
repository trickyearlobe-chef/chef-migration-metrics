// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// ---------------------------------------------------------------------------
// How a converge run reads at a glance: how much it changed, out of how much it
// managed, and how long it took.
//
// The duration is end_time - start_time. Both come from the run event, so this
// is the run's own elapsed time rather than anything CMM measured. A run with
// no start time has no duration — it is not zero, and saying "0s" would be a
// measurement nobody took.
// ---------------------------------------------------------------------------

export interface RunTiming {
  start_time?: string | null;
  end_time?: string | null;
  updated_resource_count?: number | null;
  total_resource_count?: number | null;
}

/** How long the run took, or null when it cannot be known. */
export function runDurationSeconds(run: RunTiming): number | null {
  if (!run.start_time || !run.end_time) return null;
  const start = Date.parse(run.start_time);
  const end = Date.parse(run.end_time);
  if (Number.isNaN(start) || Number.isNaN(end)) return null;
  const seconds = (end - start) / 1000;
  // A negative duration means the clocks disagree, which is worth not
  // pretending about: better to show nothing than to show "-4s".
  return seconds < 0 ? null : seconds;
}

/** A duration a person reads without converting units in their head. */
export function formatDuration(seconds: number | null): string {
  if (seconds === null) return "";
  if (seconds < 1) return "<1s";
  if (seconds < 60) return `${Math.round(seconds)}s`;

  const minutes = Math.floor(seconds / 60);
  const rest = Math.round(seconds % 60);
  if (minutes < 60) return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`;

  const hours = Math.floor(minutes / 60);
  const restMinutes = minutes % 60;
  return restMinutes === 0 ? `${hours}h` : `${hours}h ${restMinutes}m`;
}

/**
 * "12/108 in 3m 4s" — what changed, out of what was managed, and how long.
 *
 * Returns "" when the run carries no resource counts at all, so a caller can
 * render a dash rather than "0/0", which asserts a run that managed nothing.
 */
export function runResourceSummary(run: RunTiming): string {
  const total = run.total_resource_count;
  const updated = run.updated_resource_count;
  const duration = formatDuration(runDurationSeconds(run));

  if (total === null || total === undefined) {
    return duration ? `in ${duration}` : "";
  }

  const counts = `${updated ?? 0}/${total}`;
  return duration ? `${counts} in ${duration}` : counts;
}
