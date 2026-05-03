// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { groupByMajorVersion } from "./grouping";

describe("groupByMajorVersion", () => {
  it("groups versions correctly by major version", () => {
    const distribution = [
      { version: "18.5.0", count: 100, percent: 50 },
      { version: "17.10.0", count: 60, percent: 30 },
      { version: "18.2.0", count: 30, percent: 15 },
      { version: "17.0.0", count: 10, percent: 5 },
    ];

    const groups = groupByMajorVersion(distribution, 200, "Chef");

    expect(groups).toHaveLength(2);
    // Newest first
    expect(groups[0].key).toBe("18");
    expect(groups[0].label).toBe("Chef 18");
    expect(groups[1].key).toBe("17");
    expect(groups[1].label).toBe("Chef 17");
  });

  it("sorts groups descending by major version", () => {
    const distribution = [
      { version: "16.0.0", count: 10, percent: 10 },
      { version: "18.0.0", count: 50, percent: 50 },
      { version: "17.0.0", count: 40, percent: 40 },
    ];

    const groups = groupByMajorVersion(distribution, 100, "Chef");
    expect(groups.map((g) => g.key)).toEqual(["18", "17", "16"]);
  });

  it("sorts versions within a group descending", () => {
    const distribution = [
      { version: "18.2.0", count: 30, percent: 30 },
      { version: "18.5.0", count: 50, percent: 50 },
      { version: "18.0.1", count: 20, percent: 20 },
    ];

    const groups = groupByMajorVersion(distribution, 100, "Chef");
    expect(groups[0].entries.map((e) => e.label)).toEqual([
      "18.5.0",
      "18.2.0",
      "18.0.1",
    ]);
  });

  it("calculates totalCount and totalPercentage correctly", () => {
    const distribution = [
      { version: "18.5.0", count: 60, percent: 30 },
      { version: "18.2.0", count: 40, percent: 20 },
    ];

    const groups = groupByMajorVersion(distribution, 200, "Chef");
    expect(groups[0].totalCount).toBe(100);
    expect(groups[0].totalPercentage).toBe(50);
  });

  it("handles single version, single group", () => {
    const distribution = [{ version: "18.5.0", count: 100, percent: 100 }];

    const groups = groupByMajorVersion(distribution, 100, "Chef");
    expect(groups).toHaveLength(1);
    expect(groups[0].key).toBe("18");
    expect(groups[0].totalCount).toBe(100);
    expect(groups[0].entries).toHaveLength(1);
  });

  it("handles empty distribution array", () => {
    const groups = groupByMajorVersion([], 0, "Chef");
    expect(groups).toHaveLength(0);
  });

  it("handles versions with no minor (e.g. '18')", () => {
    const distribution = [
      { version: "18", count: 50, percent: 50 },
      { version: "17.1.0", count: 50, percent: 50 },
    ];

    const groups = groupByMajorVersion(distribution, 100, "Chef");
    expect(groups).toHaveLength(2);
    expect(groups[0].key).toBe("18");
  });

  it("entries have correct filterValue and label", () => {
    const distribution = [
      { version: "18.5.0", count: 100, percent: 100 },
    ];

    const groups = groupByMajorVersion(distribution, 100, "Chef");
    expect(groups[0].entries[0].filterValue).toBe("18.5.0");
    expect(groups[0].entries[0].label).toBe("18.5.0");
    expect(groups[0].entries[0].count).toBe(100);
  });
});
