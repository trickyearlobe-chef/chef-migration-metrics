// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// ---------------------------------------------------------------------------
// Saved-filter mappings for the Roles, Cookbooks and Git Repos views. The Nodes
// one lives in nodeSavedFilters.ts, which also carries the stale-role check —
// Nodes is the only list view whose filters name entities that can disappear.
//
// Each param name is that view's request vocabulary, and must stay in step with
// its <x>FilterFromValues parser and the allowlist in
// internal/webapi/saved_filter_params.go. Params those allow but the page has no
// control for (cookbooks/git-repos `compatibility`; `organisation`, which is the
// global org selector rather than a filter-bar control) are not mapped: a saved
// filter records what the view's filter bar can actually select.
// ---------------------------------------------------------------------------

import type { SavedFilterParams } from "../types";
import {
  stateToParams,
  paramsToState,
  type FilterParamMap,
} from "./savedFilterMapping";

// --- Roles -----------------------------------------------------------------

export interface RoleFilterState {
  nameFilter: string;
  compatibility: string[];
  tkStatus: string[];
}

export const EMPTY_ROLE_FILTER_STATE: RoleFilterState = {
  nameFilter: "",
  compatibility: [],
  tkStatus: [],
};

export const ROLE_FILTER_MAP: FilterParamMap<RoleFilterState> = {
  lists: [
    ["compatibility", "compatibility_status"],
    ["tkStatus", "tk_status"],
  ],
  scalars: [["nameFilter", "name"]],
};

export function roleStateToParams(state: RoleFilterState): SavedFilterParams {
  return stateToParams(state, ROLE_FILTER_MAP);
}

export function paramsToRoleState(params: SavedFilterParams): RoleFilterState {
  return paramsToState(params, EMPTY_ROLE_FILTER_STATE, ROLE_FILTER_MAP);
}

// --- Cookbooks -------------------------------------------------------------

export interface CookbookFilterState {
  nameFilter: string;
  active: string[];
  cookstyleStatus: string[];
  downloadStatus: string[];
  tkStatus: string[];
}

export const EMPTY_COOKBOOK_FILTER_STATE: CookbookFilterState = {
  nameFilter: "",
  active: [],
  cookstyleStatus: [],
  downloadStatus: [],
  tkStatus: [],
};

export const COOKBOOK_FILTER_MAP: FilterParamMap<CookbookFilterState> = {
  lists: [
    ["active", "active"],
    ["cookstyleStatus", "cookstyle_status"],
    ["downloadStatus", "download_status"],
    ["tkStatus", "tk_status"],
  ],
  scalars: [["nameFilter", "name"]],
};

export function cookbookStateToParams(
  state: CookbookFilterState,
): SavedFilterParams {
  return stateToParams(state, COOKBOOK_FILTER_MAP);
}

export function paramsToCookbookState(
  params: SavedFilterParams,
): CookbookFilterState {
  return paramsToState(params, EMPTY_COOKBOOK_FILTER_STATE, COOKBOOK_FILTER_MAP);
}

// --- Git Repos -------------------------------------------------------------

export interface GitRepoFilterState {
  nameFilter: string;
  cookstyleStatus: string[];
  tkStatus: string[];
  cloneStatus: string[];
  kitchenFilter: string[];
}

export const EMPTY_GIT_REPO_FILTER_STATE: GitRepoFilterState = {
  nameFilter: "",
  cookstyleStatus: [],
  tkStatus: [],
  cloneStatus: [],
  kitchenFilter: [],
};

export const GIT_REPO_FILTER_MAP: FilterParamMap<GitRepoFilterState> = {
  lists: [
    ["cookstyleStatus", "cookstyle_status"],
    ["tkStatus", "tk_status"],
    ["cloneStatus", "clone_status"],
    ["kitchenFilter", "has_test_suite"],
  ],
  scalars: [["nameFilter", "name"]],
};

export function gitRepoStateToParams(
  state: GitRepoFilterState,
): SavedFilterParams {
  return stateToParams(state, GIT_REPO_FILTER_MAP);
}

export function paramsToGitRepoState(
  params: SavedFilterParams,
): GitRepoFilterState {
  return paramsToState(params, EMPTY_GIT_REPO_FILTER_STATE, GIT_REPO_FILTER_MAP);
}
