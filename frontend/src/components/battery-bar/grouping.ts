// SPDX-License-Identifier: Apache-2.0

import type { VersionCount } from "../../types/dashboard";

/** Generic group structure used by BatteryBarChart. */
export interface BarGroup {
  /** Unique key for expand/collapse state. */
  key: string;
  /** Label displayed on the group row. */
  label: string;
  totalCount: number;
  totalPercentage: number;
  entries: BarGroupEntry[];
}

export interface BarGroupEntry {
  /** Label displayed in the expanded child row. */
  label: string;
  /** Value used for link building (e.g. version string or platform filter value). */
  filterValue: string;
  count: number;
  percent: number;
}

/** Legacy interface kept for backward compatibility with existing tests. */
export interface GroupedMajorVersion {
  majorVersion: number;
  totalCount: number;
  totalPercentage: number;
  versions: VersionCount[];
}

/**
 * Group version distribution entries by major version.
 * Returns BarGroup[] for direct use with BatteryBarChart.
 * Groups are sorted descending by majorVersion (newest first).
 * Versions within each group are sorted descending by semver.
 */
export function groupByMajorVersion(
  distribution: VersionCount[],
  totalNodes: number,
  labelPrefix = "Chef",
): BarGroup[] {
  const groups = new Map<number, VersionCount[]>();

  for (const entry of distribution) {
    const major = parseMajor(entry.version);
    const list = groups.get(major) ?? [];
    list.push(entry);
    groups.set(major, list);
  }

  const result: BarGroup[] = [];
  for (const [majorVersion, versions] of groups) {
    versions.sort((a, b) => compareSemverDesc(a.version, b.version));

    const totalCount = versions.reduce((sum, v) => sum + v.count, 0);
    const totalPercentage = totalNodes > 0 ? (totalCount / totalNodes) * 100 : 0;

    result.push({
      key: String(majorVersion),
      label: `${labelPrefix} ${majorVersion}`,
      totalCount,
      totalPercentage,
      entries: versions.map((v) => ({
        label: v.version,
        filterValue: v.version,
        count: v.count,
        percent: v.percent,
      })),
    });
  }

  // Sort groups descending by major version (newest first).
  result.sort((a, b) => Number(b.key) - Number(a.key));

  return result;
}

/** Parse major version from a version string like "18.5.0" → 18. */
function parseMajor(version: string): number {
  const first = version.split(".")[0];
  return parseInt(first, 10) || 0;
}

/** Compare two version strings for descending sort. */
function compareSemverDesc(a: string, b: string): number {
  const pa = a.split(".").map((s) => parseInt(s, 10) || 0);
  const pb = b.split(".").map((s) => parseInt(s, 10) || 0);
  const len = Math.max(pa.length, pb.length);
  for (let i = 0; i < len; i++) {
    const diff = (pb[i] ?? 0) - (pa[i] ?? 0);
    if (diff !== 0) return diff;
  }
  return 0;
}
