// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

/**
 * Parse a semver-like version string ("18.5.0", "19.1.164", "18") into
 * a comparable numeric tuple [major, minor, patch]. Non-numeric or missing
 * segments default to 0.
 */
function parseSemver(version: string): [number, number, number] {
  const parts = version.split(".");
  const major = parseInt(parts[0] ?? "0", 10) || 0;
  const minor = parseInt(parts[1] ?? "0", 10) || 0;
  const patch = parseInt(parts[2] ?? "0", 10) || 0;
  return [major, minor, patch];
}

/**
 * Compare two semver tuples. Returns a positive number if a > b,
 * negative if a < b, and 0 if equal.
 */
function compareSemver(
  a: [number, number, number],
  b: [number, number, number],
): number {
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] - b[i];
  }
  return 0;
}

/**
 * Return the highest semver string from an array of version strings.
 * Returns `undefined` if the array is empty.
 *
 * @example
 *   highestSemver(["17.0.0", "19.1.164", "18.5.0"]) // "19.1.164"
 *   highestSemver(["18.5.0"])                         // "18.5.0"
 *   highestSemver([])                                 // undefined
 */
export function highestSemver(versions: string[]): string | undefined {
  if (versions.length === 0) return undefined;

  let best = versions[0];
  let bestParsed = parseSemver(best);

  for (let i = 1; i < versions.length; i++) {
    const parsed = parseSemver(versions[i]);
    if (compareSemver(parsed, bestParsed) > 0) {
      best = versions[i];
      bestParsed = parsed;
    }
  }

  return best;
}

/**
 * Returns true if the given string is a valid MAJOR.MINOR.PATCH semver.
 *
 * @example
 *   isValidSemver("18.5.0")  // true
 *   isValidSemver("18")      // false
 *   isValidSemver("foo")     // false
 */
export function isValidSemver(version: string): boolean {
  return /^\d+\.\d+\.\d+$/.test(version);
}
