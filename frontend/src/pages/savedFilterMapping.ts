// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// ---------------------------------------------------------------------------
// Mapping between a list view's filter-bar state and the query-param selection
// a saved filter stores (journeys/named-cohorts.md).
//
// The mapping stays per-view on purpose. The param names are the view's own
// *request* vocabulary — what its <x>FilterFromValues parser reads and what
// internal/webapi/saved_filter_params.go allows — and they are not always the
// name the view's URL uses (on Nodes the URL says `readiness`, the request says
// `readiness_filter`). A view that gains a filter must add it to its map here,
// to its parser, and to the backend allowlist, or it cannot be saved.
//
// Deliberately unmapped everywhere: sort, order, page, per_page (view state, not
// a filter) and target_chef_version / stale_tiers (the global lens, owned by
// GlobalFilterContext). `organisation` is allowlisted by the backend but is a
// global selector (OrgContext), not a filter-bar control, so no view maps it.
// ---------------------------------------------------------------------------

import type { SavedFilterParams } from "../types";

/** How one view's filter state maps to its query params. */
export interface FilterParamMap<S> {
  /** Multi-value filters: state key -> query param name. */
  lists: ReadonlyArray<readonly [keyof S, string]>;
  /** Single-value filters, stored as a one-element list. */
  scalars: ReadonlyArray<readonly [keyof S, string]>;
  /**
   * On/off filters, stored as ["true"] when set and omitted when not — an
   * absent param and a false one mean the same thing to the request parser.
   */
  booleans?: ReadonlyArray<readonly [keyof S, string]>;
}

/** The current filter-bar selection as a storable param map. Empties omitted. */
export function stateToParams<S extends object>(
  state: S,
  map: FilterParamMap<S>,
): SavedFilterParams {
  const params: SavedFilterParams = {};

  for (const [key, param] of map.lists) {
    const values = state[key] as string[];
    if (values.length > 0) params[param] = [...values];
  }

  for (const [key, param] of map.scalars) {
    const value = state[key] as string;
    if (value) params[param] = [value];
  }

  for (const [key, param] of map.booleans ?? []) {
    if (state[key] as boolean) params[param] = ["true"];
  }

  return params;
}

/**
 * A stored selection as filter-bar state. Params the view does not own are
 * ignored, and params the selection omits are cleared — applying a named cohort
 * gives you that cohort, not that cohort merged with what you had set.
 */
export function paramsToState<S extends object>(
  params: SavedFilterParams,
  empty: S,
  map: FilterParamMap<S>,
): S {
  const state: S = { ...empty };

  for (const [key, param] of map.lists) {
    const values = params[param];
    // Clone even when the selection does not mention this filter: `empty` is a
    // module-level const, and a shallow spread would leave the applied state
    // sharing its arrays.
    (state[key] as string[]) = [...(values ?? (empty[key] as string[]))];
  }

  for (const [key, param] of map.scalars) {
    const values = params[param];
    if (values && values.length > 0) (state[key] as string) = values[0];
  }

  for (const [key, param] of map.booleans ?? []) {
    (state[key] as boolean) = params[param]?.[0] === "true";
  }

  return state;
}
