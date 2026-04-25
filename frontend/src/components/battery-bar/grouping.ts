// SPDX-License-Identifier: Apache-2.0

import type { VersionCount } from "../../types/dashboard";

export interface GroupedMajorVersion {
  majorVersion: number;
  totalCount: number;
  totalPercentage: number;
  versions: VersionCount[];
}

/**
 * Group version distribution entries by major version.
 * Groups are sorted descending by majorVersion (newest first).
 * Versions within each group are sorted descending by semver.
 */
export function groupByMajorVersion(
  distribution: VersionCount[],
  totalNodes: number,
): GroupedMajorVersion[] {
  const groups = new Map<number, VersionCount[]>();

  for (const entry of distribution) {
    const major = parseMajor(entry.version);
    const list = groups.get(major) ?? [];
    list.push(entry);
    groups.set(major, list);
  }

  const result: GroupedMajorVersion[] = [];
  for (const [majorVersion, versions] of groups) {
    // Sort versions descending within the group.
    versions.sort((a, b) => compareSemverDesc(a.version, b.version));

    const totalCount = versions.reduce((sum, v) => sum + v.count, 0);
    const totalPercentage = totalNodes > 0 ? (totalCount / totalNodes) * 100 : 0;

    result.push({ majorVersion, totalCount, totalPercentage, versions });
  }

  // Sort groups descending by major version (newest first).
  result.sort((a, b) => b.majorVersion - a.majorVersion);

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
