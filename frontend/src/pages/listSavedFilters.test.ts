// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import {
  EMPTY_ROLE_FILTER_STATE,
  ROLE_FILTER_MAP,
  roleStateToParams,
  paramsToRoleState,
  EMPTY_COOKBOOK_FILTER_STATE,
  COOKBOOK_FILTER_MAP,
  cookbookStateToParams,
  paramsToCookbookState,
  EMPTY_GIT_REPO_FILTER_STATE,
  GIT_REPO_FILTER_MAP,
  gitRepoStateToParams,
  paramsToGitRepoState,
} from "./listSavedFilters";
import { NODE_FILTER_MAP } from "./nodeSavedFilters";
import type { FilterParamMap } from "./savedFilterMapping";

describe("roles saved filters", () => {
  it("maps the view's selection to its params", () => {
    expect(
      roleStateToParams({
        nameFilter: "win-base",
        compatibility: ["compatible", "untested"],
        tkStatus: ["passed"],
      }),
    ).toEqual({
      name: ["win-base"],
      compatibility_status: ["compatible", "untested"],
      tk_status: ["passed"],
    });
  });

  it("round-trips, and clears what a selection omits", () => {
    const params = roleStateToParams({
      ...EMPTY_ROLE_FILTER_STATE,
      compatibility: ["incompatible"],
    });
    expect(paramsToRoleState(params)).toEqual({
      ...EMPTY_ROLE_FILTER_STATE,
      compatibility: ["incompatible"],
    });
  });
});

describe("cookbooks saved filters", () => {
  it("maps the view's selection to its params", () => {
    expect(
      cookbookStateToParams({
        ...EMPTY_COOKBOOK_FILTER_STATE,
        nameFilter: "apache",
        active: ["true"],
        cookstyleStatus: ["ready"],
        downloadStatus: ["ok"],
        tkStatus: ["passed", "partial"],
      }),
    ).toEqual({
      name: ["apache"],
      active: ["true"],
      cookstyle_status: ["ready"],
      download_status: ["ok"],
      tk_status: ["passed", "partial"],
    });
  });

  it("round-trips, and clears what a selection omits", () => {
    const params = cookbookStateToParams({
      ...EMPTY_COOKBOOK_FILTER_STATE,
      cookstyleStatus: ["blocked"],
    });
    expect(paramsToCookbookState(params)).toEqual({
      ...EMPTY_COOKBOOK_FILTER_STATE,
      cookstyleStatus: ["blocked"],
    });
  });
});

describe("git repos saved filters", () => {
  it("maps the view's selection to its params", () => {
    expect(
      gitRepoStateToParams({
        ...EMPTY_GIT_REPO_FILTER_STATE,
        nameFilter: "cookbook-repo",
        cookstyleStatus: ["ready"],
        tkStatus: ["failed"],
        cloneStatus: ["ok"],
        kitchenFilter: ["yes"],
      }),
    ).toEqual({
      name: ["cookbook-repo"],
      cookstyle_status: ["ready"],
      tk_status: ["failed"],
      clone_status: ["ok"],
      has_test_suite: ["yes"],
    });
  });

  it("round-trips, and clears what a selection omits", () => {
    const params = gitRepoStateToParams({
      ...EMPTY_GIT_REPO_FILTER_STATE,
      cloneStatus: ["failed"],
    });
    expect(paramsToGitRepoState(params)).toEqual({
      ...EMPTY_GIT_REPO_FILTER_STATE,
      cloneStatus: ["failed"],
    });
  });

  // "What's mine" is the question the ownership work exists to answer, and it
  // was the one thing a named cohort could not hold: the page built its saved
  // selection without ownership, so it went missing without an error.
  it("carries the chosen owners and the team verdict", () => {
    const params = gitRepoStateToParams({
      ...EMPTY_GIT_REPO_FILTER_STATE,
      ownerNames: ["alice.brown", "bob.jones"],
      humanVerdict: ["broken"],
    });
    expect(params.owner).toEqual(["alice.brown", "bob.jones"]);
    expect(params.human_verdict).toEqual(["broken"]);
    expect(paramsToGitRepoState(params)).toEqual({
      ...EMPTY_GIT_REPO_FILTER_STATE,
      ownerNames: ["alice.brown", "bob.jones"],
      humanVerdict: ["broken"],
    });
  });

  it("carries the nobody-owns-it question, and drops it when it is off", () => {
    const on = gitRepoStateToParams({
      ...EMPTY_GIT_REPO_FILTER_STATE,
      unowned: true,
    });
    expect(on.unowned).toEqual(["true"]);
    expect(paramsToGitRepoState(on).unowned).toBe(true);

    const off = gitRepoStateToParams(EMPTY_GIT_REPO_FILTER_STATE);
    expect(off.unowned).toBeUndefined();
    expect(paramsToGitRepoState(off).unowned).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Contract test against the backend allowlist.
//
// The backend rejects a save whose payload carries a param the view's vocabulary
// does not list. A frontend map that drifts out of that vocabulary therefore does
// not fail loudly — it makes saving fail at runtime, for that view only, with a
// 400 the operator cannot act on. Pin the two together against the Go source.
// ---------------------------------------------------------------------------
describe("the mapped params are in the backend's allowlist", () => {
  const source = readFileSync(
    resolve(__dirname, "../../../internal/webapi/saved_filter_params.go"),
    "utf8",
  );

  /** The params in one view's setOf(...) block in savedFilterVocabulary. */
  function allowlistFor(view: string): string[] {
    const block = new RegExp(`"${view}":\\s*setOf\\(([^)]*)\\)`, "s").exec(source);
    if (!block) throw new Error(`no allowlist block for view ${view}`);
    return [...block[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]);
  }

  const views: [string, FilterParamMap<never>][] = [
    ["nodes", NODE_FILTER_MAP as FilterParamMap<never>],
    ["roles", ROLE_FILTER_MAP as FilterParamMap<never>],
    ["cookbooks", COOKBOOK_FILTER_MAP as FilterParamMap<never>],
    ["git-repos", GIT_REPO_FILTER_MAP as FilterParamMap<never>],
  ];

  it.each(views)("%s", (view, map) => {
    const allowed = allowlistFor(view);
    const mapped = [...map.lists, ...map.scalars, ...(map.booleans ?? [])].map(
      ([, param]) => param,
    );

    expect(mapped.length).toBeGreaterThan(0);
    for (const param of mapped) {
      expect(allowed, `${view}: "${param}" is not in the backend allowlist`).toContain(
        param,
      );
    }
  });

  // The reverse direction is deliberately NOT an equality check: the allowlist is
  // a superset. `organisation` is the global org selector, not a filter-bar
  // control; `compatibility` (cookbooks, git-repos) and `ready_to_activate`
  // (nodes) are allowed but have no control on their page. A saved filter records
  // what the filter bar can actually select.
  it("does not map params the filter bar has no control for", () => {
    const mapped = views.flatMap(([, map]) =>
      [...map.lists, ...map.scalars, ...(map.booleans ?? [])].map(
        ([, param]) => param,
      ),
    );
    expect(mapped).not.toContain("organisation");
    expect(mapped).not.toContain("ready_to_activate");
    expect(mapped).not.toContain("compatibility");
  });
});
