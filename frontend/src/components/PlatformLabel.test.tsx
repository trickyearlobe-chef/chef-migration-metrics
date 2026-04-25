// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { PlatformLabel } from "./PlatformLabel";

describe("PlatformLabel", () => {
  it("shows friendly name with tooltip when display name is provided", () => {
    render(
      <PlatformLabel
        platform="windows"
        platformVersion="10.0.22631"
        platformDisplayName="Win11 23H2"
      />,
    );
    expect(screen.getByText("Win11 23H2")).toBeInTheDocument();
    expect(screen.getByText("Win11 23H2")).toHaveAttribute(
      "title",
      "windows 10.0.22631",
    );
  });

  it("shows raw platform and version when no display name", () => {
    render(
      <PlatformLabel platform="windows" platformVersion="10.0.22631" />,
    );
    expect(screen.getByText("windows 10.0.22631")).toBeInTheDocument();
  });

  it("shows dash when no platform", () => {
    render(<PlatformLabel />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("shows platform without version", () => {
    render(<PlatformLabel platform="ubuntu" />);
    expect(screen.getByText("ubuntu")).toBeInTheDocument();
  });

  it("shows raw when display name is null", () => {
    render(
      <PlatformLabel
        platform="centos"
        platformVersion="7.9"
        platformDisplayName={null}
      />,
    );
    expect(screen.getByText("centos 7.9")).toBeInTheDocument();
  });
});
