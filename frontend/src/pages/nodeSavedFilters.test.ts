// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import {
  EMPTY_NODE_FILTER_STATE,
  nodeStateToParams,
  paramsToNodeState,
  missingRoles,
  staleRoleWarning,
  type NodeFilterState,
} from "./nodeSavedFilters";

const windowsCohort: NodeFilterState = {
  ...EMPTY_NODE_FILTER_STATE,
  roles: ["win-base", "win-iis"],
  platforms: ["windows"],
  chefVersion: "18.4.12",
};

describe("nodeStateToParams", () => {
  it("emits the view's param vocabulary, lists for multi-value params", () => {
    expect(nodeStateToParams(windowsCohort)).toEqual({
      role: ["win-base", "win-iis"],
      platform: ["windows"],
      chef_version: ["18.4.12"],
    });
  });

  it("omits empty selections rather than storing empty lists", () => {
    expect(nodeStateToParams(EMPTY_NODE_FILTER_STATE)).toEqual({});
  });

  // The backend rejects these outright (saved_filter_params.go), but the point
  // is that the UI never even offers them: "which nodes" and "how I'm reading
  // them" are different concerns, and the global lens is the operator's, not
  // the cohort's.
  it("never emits sort, page, or the global lens params", () => {
    const params = nodeStateToParams(windowsCohort);
    for (const forbidden of [
      "sort",
      "order",
      "page",
      "per_page",
      "target_chef_version",
      "stale",
      "stale_tiers",
      "organisation",
    ]) {
      expect(params).not.toHaveProperty(forbidden);
    }
  });

  it("uses readiness_filter, the name the backend allowlist and API speak", () => {
    // The URL drill-down param is `readiness`; the request/allowlist param is
    // `readiness_filter`. A saved filter records the latter.
    const params = nodeStateToParams({
      ...EMPTY_NODE_FILTER_STATE,
      readinessFilter: ["blocked"],
    });
    expect(params).toEqual({ readiness_filter: ["blocked"] });
    expect(params).not.toHaveProperty("readiness");
  });
});

describe("paramsToNodeState", () => {
  it("round-trips a selection", () => {
    expect(paramsToNodeState(nodeStateToParams(windowsCohort))).toEqual(
      windowsCohort,
    );
  });

  // Applying a named cohort must give you that cohort exactly — not that cohort
  // unioned with whatever you happened to have set. Params absent from the
  // saved filter clear their filter.
  it("clears filters the saved selection does not mention", () => {
    const state = paramsToNodeState({ role: ["win-base"] });
    expect(state.roles).toEqual(["win-base"]);
    expect(state.platforms).toEqual([]);
    expect(state.chefVersion).toBe("");
    expect(state.tags).toEqual([]);
  });

  it("ignores params the Nodes filter bar does not own", () => {
    const state = paramsToNodeState({
      role: ["win-base"],
      organisation: ["org-a"],
      ready_to_activate: ["true"],
      sort: ["node_name"],
      target_chef_version: ["19.3.15"],
    });
    expect(state).toEqual({ ...EMPTY_NODE_FILTER_STATE, roles: ["win-base"] });
  });

  it("takes the first value for a scalar filter", () => {
    expect(paramsToNodeState({ chef_version: ["18.4.12"] }).chefVersion).toBe(
      "18.4.12",
    );
    expect(paramsToNodeState({ chef_version: [] }).chefVersion).toBe("");
  });
});

// Stale references are the feature, not an edge case: roles vanish during a
// migration and that is signal. Silently filtering on the survivors would shrink
// the cohort while keeping its name.
describe("missingRoles", () => {
  it("reports saved roles that no longer exist, in saved order", () => {
    expect(
      missingRoles(["win-base", "win-iis", "gone-a", "gone-b"], [
        "win-iis",
        "win-base",
        "linux-base",
      ]),
    ).toEqual(["gone-a", "gone-b"]);
  });

  it("reports nothing when every role still exists", () => {
    expect(missingRoles(["win-base"], ["win-base", "linux-base"])).toEqual([]);
  });

  // An empty catalogue means "we could not load the roles", not "every role is
  // gone" — claiming 20 of 20 roles vanished on a failed fetch is worse than
  // saying nothing.
  it("reports nothing when the role catalogue is unavailable", () => {
    expect(missingRoles(["win-base", "win-iis"], [])).toEqual([]);
  });
});

describe("staleRoleWarning", () => {
  it("counts the shortfall against the saved total", () => {
    const saved = Array.from({ length: 20 }, (_, i) => `role-${i}`);
    const existing = saved.slice(3);
    expect(staleRoleWarning(saved, existing)).toBe(
      "3 of 20 roles in this filter no longer exist: role-0, role-1, role-2",
    );
  });

  it("is silent when nothing is missing", () => {
    expect(staleRoleWarning(["win-base"], ["win-base"])).toBeNull();
  });

  it("uses the singular for a single missing role", () => {
    expect(staleRoleWarning(["a", "b"], ["b"])).toBe(
      "1 of 2 roles in this filter no longer exists: a",
    );
  });

  it("truncates a long list of missing roles", () => {
    const saved = Array.from({ length: 12 }, (_, i) => `role-${i}`);
    expect(staleRoleWarning(saved, [])).toBeNull(); // empty catalogue: unknown, not gone
    expect(staleRoleWarning(saved, ["role-11"])).toBe(
      "11 of 12 roles in this filter no longer exist: role-0, role-1, role-2, role-3, role-4 and 6 more",
    );
  });
});

// Node ownership: the same cohort question as the git repo and cookbook lists.
describe("node ownership in a saved cohort", () => {
  it("carries the chosen owners", () => {
    const params = nodeStateToParams({
      ...EMPTY_NODE_FILTER_STATE,
      ownerNames: ["alice.brown", "bob.jones"],
    });
    expect(params.owner).toEqual(["alice.brown", "bob.jones"]);
    expect(paramsToNodeState(params).ownerNames).toEqual([
      "alice.brown",
      "bob.jones",
    ]);
  });

  it("carries the nobody-owns-it question, and drops it when off", () => {
    const on = nodeStateToParams({ ...EMPTY_NODE_FILTER_STATE, unowned: true });
    expect(on.unowned).toEqual(["true"]);
    expect(paramsToNodeState(on).unowned).toBe(true);

    const off = nodeStateToParams(EMPTY_NODE_FILTER_STATE);
    expect(off.unowned).toBeUndefined();
    expect(paramsToNodeState(off).unowned).toBe(false);
  });
});
