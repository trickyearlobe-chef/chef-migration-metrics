// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { partitionVersionsByActive } from "./cookbookVersions";
import type { ServerCookbookVersionDetail } from "../types";

function version(v: string, isActive: boolean): ServerCookbookVersionDetail {
  return {
    cookbook: {
      name: "apache2",
      version: v,
      is_active: isActive,
    } as ServerCookbookVersionDetail["cookbook"],
  };
}

describe("partitionVersionsByActive", () => {
  it("splits the versions in two, preserving the order within each group", () => {
    const { active, inactive } = partitionVersionsByActive([
      version("1.0.0", false),
      version("2.0.0", true),
      version("1.5.0", false),
      version("2.1.0", true),
    ]);

    expect(active.map((v) => v.cookbook.version)).toEqual(["2.0.0", "2.1.0"]);
    expect(inactive.map((v) => v.cookbook.version)).toEqual(["1.0.0", "1.5.0"]);
  });

  it("handles a cookbook with no inactive versions", () => {
    const { active, inactive } = partitionVersionsByActive([version("1.0.0", true)]);
    expect(active).toHaveLength(1);
    expect(inactive).toEqual([]);
  });

  it("handles a cookbook whose every version is unused", () => {
    const { active, inactive } = partitionVersionsByActive([
      version("1.0.0", false),
      version("1.1.0", false),
    ]);
    expect(active).toEqual([]);
    expect(inactive).toHaveLength(2);
  });

  it("handles no versions at all", () => {
    expect(partitionVersionsByActive([])).toEqual({ active: [], inactive: [] });
  });
});

// The inactive tail is collapsed so it stops burying the versions that still matter
// — but a cookbook whose versions are ALL unused would then render as an empty page,
// which reads as "nothing here" rather than "nothing in use". In that case the
// inactive group opens.
describe("inactiveOpenByDefault", () => {
  it("is closed when there is something active to show", async () => {
    const { inactiveOpenByDefault } = await import("./cookbookVersions");
    expect(inactiveOpenByDefault(2)).toBe(false);
  });

  it("is open when every version is unused, so the page is never blank", async () => {
    const { inactiveOpenByDefault } = await import("./cookbookVersions");
    expect(inactiveOpenByDefault(0)).toBe(true);
  });
});
