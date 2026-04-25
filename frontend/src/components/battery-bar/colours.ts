// SPDX-License-Identifier: Apache-2.0

/**
 * Base colour configuration for major version groups.
 * Index 0 = newest/latest major, 1 = latest-1, etc.
 */
const BASE_COLOURS: { name: string; shades: string[] }[] = [
  {
    name: "green",
    shades: ["#059669", "#10b981", "#34d399", "#6ee7b7", "#a7f3d0", "#d1fae5"],
  },
  {
    name: "blue",
    shades: ["#2563eb", "#3b82f6", "#60a5fa", "#93c5fd", "#bfdbfe", "#dbeafe"],
  },
  {
    name: "amber",
    shades: ["#d97706", "#f59e0b", "#fbbf24", "#fcd34d", "#fde68a", "#fef3c7"],
  },
  {
    name: "red",
    shades: ["#dc2626", "#ef4444", "#f87171", "#fca5a5", "#fecaca", "#fee2e2"],
  },
];

/**
 * Get the base colour for a major version group.
 * @param groupIndex 0 = newest major, 1 = next, etc.
 * @returns Colour name and shade array.
 */
export function getBaseColour(groupIndex: number): { name: string; shades: string[] } {
  if (groupIndex < BASE_COLOURS.length) {
    return BASE_COLOURS[groupIndex];
  }
  // Everything older than latest-3 gets red.
  return BASE_COLOURS[BASE_COLOURS.length - 1];
}

/**
 * Get the shade colour for a specific minor version within a group.
 * @param groupIndex 0 = newest major version group
 * @param versionIndex 0 = newest minor within the group
 * @param totalVersions Total number of versions in this group
 * @returns Hex colour string
 */
export function getSegmentColour(
  groupIndex: number,
  versionIndex: number,
  totalVersions: number,
): string {
  const base = getBaseColour(groupIndex);
  const maxShadeIdx = base.shades.length - 1;

  if (totalVersions <= 1) return base.shades[0];

  // Map versionIndex to a shade: newest minor → darkest (index 0),
  // oldest minor → lightest. Cap at available shades.
  const step = maxShadeIdx / Math.max(totalVersions - 1, 1);
  const shadeIdx = Math.min(Math.round(versionIndex * step), maxShadeIdx);
  return base.shades[shadeIdx];
}
