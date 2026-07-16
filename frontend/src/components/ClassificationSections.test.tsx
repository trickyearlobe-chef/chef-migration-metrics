// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ClassificationSections } from "./ClassificationSections";
import type { OffenseGroup } from "../types/remediation";

function group(
  cop: string,
  classification: string,
  count: number,
): OffenseGroup {
  return {
    group_key: cop,
    cop_name: cop,
    severity: "warning",
    classification,
    classification_source: "review_default",
    count,
    correctable_count: 0,
    offenses: [],
  };
}

const groups: OffenseGroup[] = [
  group("Chef/Blocker/A", "blocker", 2),
  group("Chef/Review/B", "review", 1),
  group("Chef/Noise/C", "noise", 3),
];

function renderSections(g: OffenseGroup[], classFilter = "") {
  return render(
    <ClassificationSections
      groups={g}
      classFilter={classFilter}
      renderGroup={(grp) => <div key={grp.cop_name}>{grp.cop_name}</div>}
    />,
  );
}

describe("ClassificationSections", () => {
  it("renders a section per non-empty classification with cop + offense counts", () => {
    renderSections(groups);
    expect(screen.getByText("Blockers")).toBeInTheDocument();
    expect(screen.getByText("Review")).toBeInTheDocument();
    expect(screen.getByText("Noise")).toBeInTheDocument();
    // There is no Unclassified bucket under the trustworthy-reds model.
    expect(screen.queryByText("Unclassified")).not.toBeInTheDocument();
    // Blocker section count: 1 cop, 2 offenses.
    expect(screen.getByText(/1 cop, 2 offenses/)).toBeInTheDocument();
  });

  it("folds groups with no/unknown classification into Review", () => {
    renderSections([group("Chef/Unknown/D", "", 4)]);
    // Review is the honest default; the group is visible in the (open) Review section.
    expect(screen.getByText("Review")).toBeInTheDocument();
    expect(screen.getByText("Chef/Unknown/D")).toBeInTheDocument();
    expect(screen.queryByText("Blockers")).not.toBeInTheDocument();
    expect(screen.queryByText("Noise")).not.toBeInTheDocument();
  });

  it("expands Blockers and Review by default, collapses Noise", () => {
    renderSections(groups);
    // Blocker + Review groups are visible (sections open).
    expect(screen.getByText("Chef/Blocker/A")).toBeInTheDocument();
    expect(screen.getByText("Chef/Review/B")).toBeInTheDocument();
    // Noise group is hidden (section collapsed by default).
    expect(screen.queryByText("Chef/Noise/C")).not.toBeInTheDocument();
  });

  it("toggles a collapsed section open on click", async () => {
    const user = userEvent.setup();
    renderSections(groups);
    await user.click(screen.getByText("Noise"));
    expect(screen.getByText("Chef/Noise/C")).toBeInTheDocument();
  });

  it("shows a filter-aware empty state when nothing matches the filter", () => {
    // Only a blocker group exists; filter on noise → empty.
    renderSections([group("Chef/Blocker/A", "blocker", 2)], "noise");
    expect(
      screen.getByText("No offenses match this filter"),
    ).toBeInTheDocument();
  });

  it("shows only the matching section when a classFilter is set", () => {
    renderSections(groups, "blocker");
    expect(screen.getByText("Blockers")).toBeInTheDocument();
    expect(screen.queryByText("Review")).not.toBeInTheDocument();
    expect(screen.queryByText("Noise")).not.toBeInTheDocument();
  });
});
