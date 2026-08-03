// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// ---------------------------------------------------------------------------
// The Nodes view's saved-filter mapping, and its stale-reference check.
//
// The generic mapping lives in savedFilterMapping.ts; what is Nodes-specific is
// the param table below (the view's request vocabulary) and the role check —
// Nodes is the only list view whose filters name *entities* that can disappear.
//
// Not mapped: organisation (the org selector, not a filter-bar control) and
// ready_to_activate (no control on this page). The backend allows both, but the
// filter bar does not own them, so they are neither saved nor applied.
// ---------------------------------------------------------------------------

import type { SavedFilterParams } from "../types";
import {
  stateToParams,
  paramsToState,
  type FilterParamMap,
} from "./savedFilterMapping";

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
  ownerNames: string[];
  unowned: boolean;
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
  ownerNames: [],
  unowned: false,
};

export const NODE_FILTER_MAP: FilterParamMap<NodeFilterState> = {
  lists: [
    ["environments", "environment"],
    ["platforms", "platform"],
    ["roles", "role"],
    ["tags", "tags"],
    ["policyNames", "policy_name"],
    ["policyGroups", "policy_group"],
    // The URL drill-down param is `readiness`; the request param — and so the
    // saved one — is `readiness_filter`.
    ["readinessFilter", "readiness_filter"],
    ["cookstyleFilter", "cookstyle_status"],
    ["kitchenFilter", "kitchen_status"],
    ["deploymentStateFilter", "migration_state"],
    ["convergeStatusFilter", "target_converge_status"],
    ["targetVersionFilter", "target_version"],
    ["ownerNames", "owner"],
  ],
  scalars: [
    ["nodeName", "node_name"],
    ["chefVersion", "chef_version"],
  ],
  booleans: [["unowned", "unowned"]],
};

export function nodeStateToParams(state: NodeFilterState): SavedFilterParams {
  return stateToParams(state, NODE_FILTER_MAP);
}

export function paramsToNodeState(params: SavedFilterParams): NodeFilterState {
  return paramsToState(params, EMPTY_NODE_FILTER_STATE, NODE_FILTER_MAP);
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
