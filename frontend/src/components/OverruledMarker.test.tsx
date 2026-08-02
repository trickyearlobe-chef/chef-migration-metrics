// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { OverruledMarker } from "./OverruledMarker";

describe("OverruledMarker", () => {
  // A repo nobody has an opinion about must look exactly as it did before the
  // register existed.
  it("shows nothing when nobody has recorded a verdict", () => {
    const { container } = render(<OverruledMarker verdict={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });

  // The expensive case: CookStyle says blocked, a person says it is fine, and
  // without this the list just says blocked.
  it("marks a scan a person has overruled as fine", () => {
    render(
      <OverruledMarker
        verdict="not_broken"
        reason="kitchen never converged; this runs on 4000 nodes"
      />,
    );

    const marker = screen.getByTestId("overruled-marker");
    expect(marker).toHaveTextContent(/person says OK/i);
    // The reason travels with it — a marker with no reason is unreadable.
    expect(marker).toHaveAttribute(
      "title",
      expect.stringContaining("kitchen never converged"),
    );
    expect(marker).toHaveAttribute(
      "title",
      expect.stringContaining("readiness follows them"),
    );
  });

  it("marks a repo a person has called broken", () => {
    render(<OverruledMarker verdict="broken" reason="fails on converge" />);

    expect(screen.getByTestId("overruled-marker")).toHaveTextContent(
      /person says broken/i,
    );
  });

  // A verdict recorded with no reason cannot happen through the API, but the
  // marker must not render the word "undefined" if it ever does.
  it("survives a verdict with no reason attached", () => {
    render(<OverruledMarker verdict="broken" />);

    const marker = screen.getByTestId("overruled-marker");
    expect(marker.getAttribute("title")).not.toMatch(/undefined/i);
  });
});
