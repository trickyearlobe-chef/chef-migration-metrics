// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { guessMapping, previewBlockedReason } from "./OwnershipMappedImport";
import type { IntakeSourceProfile } from "../api";

// journeys/ownership-intake.md: "Something guessing the field names for me,
// which already helps — as long as I can see what it chose and change it. A
// guess I cannot override is worse than no guess."
//
// The guess is the first thing an administrator sees after profiling a source
// they cannot inspect any other way, so a wrong one costs more than none: it
// turns mapping into auditing, and an auditor who trusts it imports against the
// wrong column.

function profileWith(...columns: string[]): IntakeSourceProfile {
  return {
    columns: columns.map((name) => ({
      name,
      fill_rate: 1,
      distinct_count: 1,
      sample_values: [],
    })),
    row_count: 1,
    warnings: [],
  } as unknown as IntakeSourceProfile;
}

describe("guessMapping", () => {
  it("finds the owner and the thing owned in an ordinary table", () => {
    const guess = guessMapping(profileWith("owner_email", "repo_name", "team"));
    expect(guess.owner?.column).toBe("owner_email");
    expect(guess.entity_key?.column).toBe("repo_name");
  });

  it("leaves the owner unmapped rather than guessing wrongly", () => {
    // The customer's table: nothing here names an owner in words the guess
    // knows, so it must say so rather than pick the nearest column.
    const guess = guessMapping(profileWith("asset_name", "asset_id", "cost_centre"));
    expect(guess.owner).toBeUndefined();
    expect(guess.entity_key?.column).toBe("asset_name");
  });

  // The failure that matters: importing every assignment against the owner's
  // own name as if it were the thing owned. Silent, and wrong in a way a
  // preview of four rows would not obviously show.
  it("never maps the owner and the thing owned to the same column", () => {
    const guess = guessMapping(profileWith("owner_name", "cost_centre"));
    expect(guess.entity_key?.column).not.toBe(guess.owner?.column);
  });

  it("guesses nothing when nothing matches, rather than picking the first column", () => {
    const guess = guessMapping(profileWith("col_a", "col_b"));
    expect(guess.owner).toBeUndefined();
    expect(guess.entity_key).toBeUndefined();
  });
});

// A disabled button with no reason is the same fault as an import that drops
// rows silently: the screen knows something the person does not. Reported by
// the owner testing against a customer database, where the guess could not find
// an owner column and Preview greyed out saying nothing.
describe("previewBlockedReason", () => {
  const ready = {
    sourceKind: "database" as const,
    sourceReady: true,
    hasOwner: true,
    hasEntityKey: true,
  };

  it("says nothing is wrong when the form is ready", () => {
    expect(previewBlockedReason(ready)).toBeNull();
  });

  it("names the missing required field", () => {
    const reason = previewBlockedReason({ ...ready, hasOwner: false });
    expect(reason).toMatch(/Owner/);
    expect(reason).not.toMatch(/Entity key/);
  });

  it("names both when both are missing", () => {
    const reason = previewBlockedReason({ ...ready, hasOwner: false, hasEntityKey: false });
    expect(reason).toMatch(/Owner and Entity key/);
  });

  // The button used to look available with no source chosen, and do nothing at
  // all when clicked — worse than being disabled, because nothing happens and
  // nothing explains it.
  it("asks for the source before the mapping", () => {
    expect(previewBlockedReason({ ...ready, sourceReady: false })).toMatch(/credential|query/);
    expect(
      previewBlockedReason({ ...ready, sourceKind: "file", sourceReady: false }),
    ).toMatch(/file/);
  });
});
