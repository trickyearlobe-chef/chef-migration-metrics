// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { StaleBadge, DeploymentStateBadge, ConvergeBadge } from "./StatusBadge";

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

describe("DeploymentStateBadge", () => {
  it("renders 'Current only' for null/undefined state", () => {
    render(<DeploymentStateBadge state={null} />);
    expect(screen.getByText("Current only")).toBeInTheDocument();
  });

  it("renders 'Current only' for 'Current only' label", () => {
    render(<DeploymentStateBadge state="Current only" />);
    expect(screen.getByText("Current only")).toBeInTheDocument();
  });

  it("renders 'Staged' label with amber styling", () => {
    const { container } = render(<DeploymentStateBadge state="Staged" />);
    expect(screen.getByText("Staged")).toBeInTheDocument();
    const badge = container.querySelector("span");
    expect(badge?.className).toContain("bg-amber");
  });

  it("renders 'Activated' label with green styling", () => {
    const { container } = render(<DeploymentStateBadge state="Activated" />);
    expect(screen.getByText("Activated")).toBeInTheDocument();
    const badge = container.querySelector("span");
    expect(badge?.className).toContain("bg-green");
  });

  it("supports sm size", () => {
    render(<DeploymentStateBadge state="Staged" size="sm" />);
    expect(screen.getByText("Staged")).toBeInTheDocument();
  });
});

describe("ConvergeBadge", () => {
  it("renders dash for null status", () => {
    render(<ConvergeBadge status={null} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders dash for undefined status", () => {
    render(<ConvergeBadge status={undefined} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders success badge with green styling", () => {
    const { container } = render(<ConvergeBadge status="success" />);
    expect(screen.getByText("Success")).toBeInTheDocument();
    const badge = container.querySelector("span");
    expect(badge?.className).toContain("bg-green");
  });

  it("renders failed badge with red styling", () => {
    const { container } = render(<ConvergeBadge status="failed" />);
    expect(screen.getByText("Failed")).toBeInTheDocument();
    const badge = container.querySelector("span");
    expect(badge?.className).toContain("bg-red");
  });

  it("supports sm size", () => {
    render(<ConvergeBadge status="success" size="sm" />);
    expect(screen.getByText("Success")).toBeInTheDocument();
  });
});
