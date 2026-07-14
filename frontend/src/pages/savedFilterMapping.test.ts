// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import {
  stateToParams,
  paramsToState,
  type FilterParamMap,
} from "./savedFilterMapping";

interface DemoState {
  nameFilter: string;
  statuses: string[];
  tiers: string[];
}

const EMPTY: DemoState = { nameFilter: "", statuses: [], tiers: [] };

const MAP: FilterParamMap<DemoState> = {
  scalars: [["nameFilter", "name"]],
  lists: [
    ["statuses", "cookstyle_status"],
    ["tiers", "tk_status"],
  ],
};

describe("stateToParams", () => {
  it("maps state to the view's param vocabulary", () => {
    expect(
      stateToParams(
        { nameFilter: "apache", statuses: ["ready", "blocked"], tiers: [] },
        MAP,
      ),
    ).toEqual({ name: ["apache"], cookstyle_status: ["ready", "blocked"] });
  });

  it("omits empty selections rather than storing empty lists", () => {
    expect(stateToParams(EMPTY, MAP)).toEqual({});
  });

  it("copies the values, so mutating the state later cannot rewrite a stored filter", () => {
    const state: DemoState = { ...EMPTY, statuses: ["ready"] };
    const params = stateToParams(state, MAP);
    state.statuses.push("blocked");
    expect(params.cookstyle_status).toEqual(["ready"]);
  });
});

describe("paramsToState", () => {
  it("round-trips a selection", () => {
    const state: DemoState = {
      nameFilter: "apache",
      statuses: ["ready"],
      tiers: ["passed"],
    };
    expect(paramsToState(stateToParams(state, MAP), EMPTY, MAP)).toEqual(state);
  });

  // Applying a named cohort must give you that cohort exactly, not that cohort
  // merged with whatever you happened to have set.
  it("clears filters the saved selection does not mention", () => {
    const state = paramsToState({ name: ["apache"] }, EMPTY, MAP);
    expect(state).toEqual({ ...EMPTY, nameFilter: "apache" });
  });

  it("ignores params the view does not own", () => {
    const state = paramsToState(
      { name: ["apache"], organisation: ["org-a"], sort: ["name"] },
      EMPTY,
      MAP,
    );
    expect(state).toEqual({ ...EMPTY, nameFilter: "apache" });
  });

  it("takes the first value for a scalar filter, and tolerates an empty list", () => {
    expect(paramsToState({ name: ["a", "b"] }, EMPTY, MAP).nameFilter).toBe("a");
    expect(paramsToState({ name: [] }, EMPTY, MAP).nameFilter).toBe("");
  });

  // EMPTY is a module-level const shared by every apply. A shallow spread would
  // leave any list the saved filter does not mention pointing straight at it, so
  // one mutation anywhere would corrupt the empty state for the whole app.
  it("does not alias the caller's empty state, mentioned or not", () => {
    const state = paramsToState({ cookstyle_status: ["ready"] }, EMPTY, MAP);

    state.statuses.push("blocked"); // a list the selection did mention
    state.tiers.push("passed"); // a list it did not

    expect(EMPTY.statuses).toEqual([]);
    expect(EMPTY.tiers).toEqual([]);
  });
});
