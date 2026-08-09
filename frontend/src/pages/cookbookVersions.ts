// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// ---------------------------------------------------------------------------
// Grouping for the versions listed on the cookbook detail page.
//
// A cookbook accumulates versions on the Chef server long after they stop being
// applied to any node, and during a migration the unused tail is usually the
// majority. Listing every version flat buries the ones still in use among the
// ones that are not.
//
// The active/unused flag itself is defined by the usage analysis
// (specifications/cookbook-compatibility.md) — this is presentation over it.
// ---------------------------------------------------------------------------

import type { ServerCookbookVersionDetail } from "../types";

export interface VersionGroups {
  active: ServerCookbookVersionDetail[];
  inactive: ServerCookbookVersionDetail[];
}

/** Split versions into in-use and unused, keeping the API's ordering within each. */
export function partitionVersionsByActive(
  versions: ServerCookbookVersionDetail[],
): VersionGroups {
  const active: ServerCookbookVersionDetail[] = [];
  const inactive: ServerCookbookVersionDetail[] = [];

  for (const version of versions) {
    (version.cookbook.is_active ? active : inactive).push(version);
  }

  return { active, inactive };
}

/**
 * Whether the unused group starts expanded.
 *
 * Normally it does not — collapsing it is the whole point. But a cookbook whose
 * every version is unused would then render as an empty page, which reads as
 * "nothing here" rather than "nothing in use". So when there is nothing active to
 * show, the unused group opens.
 */
export function inactiveOpenByDefault(activeCount: number): boolean {
  return activeCount === 0;
}
