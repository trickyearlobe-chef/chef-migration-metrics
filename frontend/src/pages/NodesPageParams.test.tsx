// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { stripNodeDrilldownParams } from "./NodesPage";

// NodesPage shares useSearchParams with GlobalFilterContext, which owns
// target_chef_version + stale_tiers. When NodesPage clears the drill-down params
// it consumed, it must NOT wipe those global params — doing so desynced the global
// staleness/target state from the URL (the "only fresh after clear / refresh
// needed / toggle no effect" bug, which was inconsistent because of the race).
describe("stripNodeDrilldownParams", () => {
  it("removes page drill-down params but preserves the global filter params", () => {
    const before = new URLSearchParams(
      "readiness=blocked&target_version=19.3.15&stale_tiers=fresh,warning&target_chef_version=19.3.15",
    );
    const after = stripNodeDrilldownParams(before);

    // Page-owned drill-down params are gone.
    expect(after.has("readiness")).toBe(false);
    expect(after.has("target_version")).toBe(false);

    // Global filter params owned by GlobalFilterContext survive.
    expect(after.get("stale_tiers")).toBe("fresh,warning");
    expect(after.get("target_chef_version")).toBe("19.3.15");
  });

  it("does not mutate the input params", () => {
    const before = new URLSearchParams("readiness=blocked&stale_tiers=fresh");
    stripNodeDrilldownParams(before);
    expect(before.get("readiness")).toBe("blocked");
  });

  it("strips every page param", () => {
    const before = new URLSearchParams(
      "readiness=ready&chef_version=18&platform=ubuntu&environment=prod&role=web&policy_name=p&policy_group=g&migration_state=Staged&target_converge_status=failed&stale_tiers=critical",
    );
    const after = stripNodeDrilldownParams(before);
    for (const p of [
      "readiness",
      "chef_version",
      "platform",
      "environment",
      "role",
      "policy_name",
      "policy_group",
      "migration_state",
      "target_converge_status",
    ]) {
      expect(after.has(p)).toBe(false);
    }
    expect(after.get("stale_tiers")).toBe("critical");
  });
});
