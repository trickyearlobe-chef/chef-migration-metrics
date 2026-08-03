// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { OwnershipCard } from "./OwnershipCard";

function show(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("OwnershipCard", () => {
  it("names the owner, and links to them", () => {
    show(<OwnershipCard ownership={{ owners: ["priya-raman"] }} />);

    const link = screen.getByRole("link", { name: "priya-raman" });
    expect(link).toHaveAttribute("href", "/ownership/priya-raman");
  });

  // An unowned thing is exactly what somebody has to go and find an owner for,
  // so it is said out loud. A blank space reads as "not applicable".
  it("says when nobody owns it, rather than showing nothing", () => {
    show(<OwnershipCard ownership={{ owners: [] }} />);

    expect(screen.getByText(/Nobody owns this yet/)).toBeInTheDocument();
  });

  // Absent is not empty: the API could not say, so asserting "nobody" would be
  // a claim nobody has earned.
  it("shows nothing at all when ownership could not be read", () => {
    const { container } = show(<OwnershipCard />);
    expect(container).toBeEmptyDOMElement();
  });

  it("says where derived ownership came from", () => {
    show(<OwnershipCard ownership={{ owners: ["alice"], derived: true }} />);

    expect(screen.getByText(/built from/)).toBeInTheDocument();
  });

  it("does not claim derivation for a direct assignment", () => {
    show(<OwnershipCard ownership={{ owners: ["alice"] }} />);

    expect(screen.queryByText(/built from/)).not.toBeInTheDocument();
  });
});
