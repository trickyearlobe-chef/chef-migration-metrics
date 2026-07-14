// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// ---------------------------------------------------------------------------
// Mapping between the Nodes filter bar's state and the query-param selection a
// saved filter stores (see specifications/saved-filters.md).
//
// The param names here are the Nodes view's request vocabulary — the same names
// handle_nodes.go parses and internal/webapi/saved_filter_params.go allows.
// Adding a filter to the Nodes page means adding it in all three places, or it
// cannot be saved.
//
// Deliberately absent: sort, order, page, per_page (view state, not a filter);
// target_chef_version and stale_tiers (the global lens, owned by
// GlobalFilterContext); organisation (the org selector) and ready_to_activate
// (no control on this page). The backend allows the last two but the filter bar
// does not own them, so they are neither saved nor applied.
// ---------------------------------------------------------------------------

import type { SavedFilterParams } from "../types";

/** The Nodes filter bar's selection, keyed as NodesPage holds it. */
export interface NodeFilterState {
  nodeName: string;
  environments: string[];
  platforms: string[];
  chefVersion: string;
  roles: string[];
  tags: string[];
  policyNames: string[];
  policyGroups: string[];
  readinessFilter: string[];
  cookstyleFilter: string[];
  kitchenFilter: string[];
  deploymentStateFilter: string[];
  convergeStatusFilter: string[];
  targetVersionFilter: string[];
}

export const EMPTY_NODE_FILTER_STATE: NodeFilterState = {
  nodeName: "",
  environments: [],
  platforms: [],
  chefVersion: "",
  roles: [],
  tags: [],
  policyNames: [],
  policyGroups: [],
  readinessFilter: [],
  cookstyleFilter: [],
  kitchenFilter: [],
  deploymentStateFilter: [],
  convergeStatusFilter: [],
  targetVersionFilter: [],
};

/** Multi-value filters: state key -> query param name. */
const LIST_PARAMS: [keyof NodeFilterState, string][] = [
  ["environments", "environment"],
  ["platforms", "platform"],
  ["roles", "role"],
  ["tags", "tags"],
  ["policyNames", "policy_name"],
  ["policyGroups", "policy_group"],
  ["readinessFilter", "readiness_filter"],
  ["cookstyleFilter", "cookstyle_status"],
  ["kitchenFilter", "kitchen_status"],
  ["deploymentStateFilter", "migration_state"],
  ["convergeStatusFilter", "target_converge_status"],
  ["targetVersionFilter", "target_version"],
];

/** Single-value filters, stored as a one-element list. */
const SCALAR_PARAMS: [keyof NodeFilterState, string][] = [
  ["nodeName", "node_name"],
  ["chefVersion", "chef_version"],
];

/** The current filter-bar selection as a storable param map. Empties omitted. */
export function nodeStateToParams(state: NodeFilterState): SavedFilterParams {
  const params: SavedFilterParams = {};

  for (const [key, param] of LIST_PARAMS) {
    const values = state[key] as string[];
    if (values.length > 0) params[param] = [...values];
  }

  for (const [key, param] of SCALAR_PARAMS) {
    const value = state[key] as string;
    if (value) params[param] = [value];
  }

  return params;
}

/**
 * A stored selection as filter-bar state. Params the filter bar does not own
 * are ignored, and params the selection omits are cleared — applying a named
 * cohort gives you that cohort, not that cohort merged with what you had set.
 */
export function paramsToNodeState(params: SavedFilterParams): NodeFilterState {
  const state: NodeFilterState = { ...EMPTY_NODE_FILTER_STATE };

  for (const [key, param] of LIST_PARAMS) {
    const values = params[param];
    if (values) (state[key] as string[]) = [...values];
  }

  for (const [key, param] of SCALAR_PARAMS) {
    const values = params[param];
    if (values && values.length > 0) (state[key] as string) = values[0];
  }

  return state;
}

/**
 * Saved roles that no longer exist in the fleet.
 *
 * An empty catalogue means the role list could not be loaded, not that every
 * role vanished — reporting "20 of 20 roles are gone" on a failed fetch is a
 * worse lie than staying quiet, so that case reports nothing.
 */
export function missingRoles(saved: string[], existing: string[]): string[] {
  if (existing.length === 0) return [];
  const live = new Set(existing);
  return saved.filter((role) => !live.has(role));
}

const MAX_NAMED = 5;

/**
 * The shortfall, stated plainly, or null when there is none. A vanished role is
 * signal — a migration is removing roles — so the cohort must never quietly
 * shrink behind its name.
 */
export function staleRoleWarning(
  saved: string[],
  existing: string[],
): string | null {
  const missing = missingRoles(saved, existing);
  if (missing.length === 0) return null;

  const verb = missing.length === 1 ? "no longer exists" : "no longer exist";
  const named = missing.slice(0, MAX_NAMED).join(", ");
  const rest = missing.length - MAX_NAMED;
  const tail = rest > 0 ? `${named} and ${rest} more` : named;

  return `${missing.length} of ${saved.length} roles in this filter ${verb}: ${tail}`;
}
