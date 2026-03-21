// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { highestSemver } from "./semver";

describe("highestSemver", () => {
  it("returns undefined for an empty array", () => {
    expect(highestSemver([])).toBeUndefined();
  });

  it("returns the only version when given a single element", () => {
    expect(highestSemver(["18.5.0"])).toBe("18.5.0");
  });

  it("picks the highest major version", () => {
    expect(highestSemver(["17.0.0", "19.1.164", "18.5.0"])).toBe("19.1.164");
  });

  it("picks the highest minor version when majors are equal", () => {
    expect(highestSemver(["18.5.0", "18.10.17", "18.8.54"])).toBe("18.10.17");
  });

  it("picks the highest patch version when major and minor are equal", () => {
    expect(highestSemver(["19.1.12", "19.1.164", "19.1.3"])).toBe("19.1.164");
  });

  it("handles two-segment versions (missing patch)", () => {
    expect(highestSemver(["18.5", "19.0"])).toBe("19.0");
  });

  it("handles single-segment versions (major only)", () => {
    expect(highestSemver(["17", "19", "18"])).toBe("19");
  });

  it("handles mixed segment counts", () => {
    expect(highestSemver(["18", "17.9.9", "18.0.1"])).toBe("18.0.1");
  });

  it("returns the first occurrence when all versions are equal", () => {
    expect(highestSemver(["18.5.0", "18.5.0", "18.5.0"])).toBe("18.5.0");
  });

  it("does not sort lexicographically (9 < 18 numerically)", () => {
    // Lexicographic sort would put "9.0.0" after "18.0.0"
    expect(highestSemver(["9.0.0", "18.0.0"])).toBe("18.0.0");
  });

  it("compares numerically, not lexicographically (10 > 9)", () => {
    // Lexicographic: "9" > "10", but numeric: 10 > 9
    expect(highestSemver(["18.9.0", "18.10.0"])).toBe("18.10.0");
  });

  it("preserves the original version string (no normalisation)", () => {
    const result = highestSemver(["18.05.0", "17.0.0"]);
    expect(result).toBe("18.05.0");
  });

  it("handles realistic target_chef_versions from config", () => {
    expect(highestSemver(["18.5.0", "19.0.0"])).toBe("19.0.0");
  });
});
