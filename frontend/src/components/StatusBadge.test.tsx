// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { StaleBadge } from "./StatusBadge";

describe("StaleBadge", () => {
  it("renders Fresh text when stalenesTier is fresh", () => {
    render(<StaleBadge isStale={false} stalenesTier="fresh" />);
    expect(screen.getByText("Fresh")).toBeInTheDocument();
  });

  it("renders Missing text when stalenesTier is warning", () => {
    render(<StaleBadge isStale={false} stalenesTier="warning" />);
    expect(screen.getByText("Missing")).toBeInTheDocument();
  });

  it("renders Missing with days when stalenesTier is warning and ageHours >= 48", () => {
    render(
      <StaleBadge isStale={false} stalenesTier="warning" ageHours={52} />,
    );
    expect(screen.getByText("Missing (2d)")).toBeInTheDocument();
  });

  it("renders Missing with minutes when stalenesTier is warning and ageHours < 1", () => {
    render(
      <StaleBadge isStale={false} stalenesTier="warning" ageHours={0.5} />,
    );
    expect(screen.getByText("Missing (30m)")).toBeInTheDocument();
  });

  it("renders Missing with hours when stalenesTier is warning and ageHours in hours range", () => {
    render(
      <StaleBadge isStale={false} stalenesTier="warning" ageHours={5} />,
    );
    expect(screen.getByText("Missing (5h)")).toBeInTheDocument();
  });

  it("renders Gone text when stalenesTier is critical", () => {
    render(<StaleBadge isStale={false} stalenesTier="critical" />);
    expect(screen.getByText("Gone")).toBeInTheDocument();
  });

  it("renders Gone with days when stalenesTier is critical and ageHours provided", () => {
    render(
      <StaleBadge isStale={false} stalenesTier="critical" ageHours={240} />,
    );
    expect(screen.getByText("Gone (10d)")).toBeInTheDocument();
  });

  it("falls back to Gone when isStale is true and no tier is provided", () => {
    render(<StaleBadge isStale={true} />);
    expect(screen.getByText("Gone")).toBeInTheDocument();
  });

  it("falls back to Fresh when isStale is false and no tier is provided", () => {
    render(<StaleBadge isStale={false} />);
    expect(screen.getByText("Fresh")).toBeInTheDocument();
  });

  it("tier overrides isStale — stalenesTier fresh wins over isStale true", () => {
    render(<StaleBadge isStale={true} stalenesTier="fresh" />);
    expect(screen.getByText("Fresh")).toBeInTheDocument();
  });
});
