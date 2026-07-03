// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import type { CopDriftReport } from "../types";

vi.mock("../api");

import { AdminCopInventorySection } from "./AdminCopInventorySection";

const AVAILABLE_REPORT: CopDriftReport = {
  registry_available: true,
  registry_version: "8.6.10",
  coverage_gaps: [
    { cop_name: "Chef/Modernize/FooBar", department: "Chef/Modernize", enabled: true },
  ],
  stale: [
    { cop_name: "Chef/Deprecations/OldGone", source: "removed_in_mapping" },
    { cop_name: "Lint/AlsoGone", source: "removed_in_mapping" },
  ],
};

beforeEach(() => {
  vi.mocked(api.fetchTargetVersions).mockResolvedValue(["18.0", "16.0"]);
  vi.mocked(api.fetchCopDrift).mockResolvedValue(AVAILABLE_REPORT);
});

describe("AdminCopInventorySection", () => {
  it("renders coverage gaps and stale entries with the registry version", async () => {
    render(<AdminCopInventorySection />);

    await waitFor(() => {
      expect(screen.getByText("Chef/Modernize/FooBar")).toBeInTheDocument();
    });
    // Stale entries with human-readable source labels. The removed-in mapping is
    // the only static classification table, so both rows share that label.
    expect(screen.getByText("Chef/Deprecations/OldGone")).toBeInTheDocument();
    expect(screen.getByText("Lint/AlsoGone")).toBeInTheDocument();
    expect(screen.getAllByText("RemovedIn mapping")).toHaveLength(2);
    // Registry version chip.
    expect(screen.getByText(/cookstyle 8\.6\.10/)).toBeInTheDocument();
    // Section counts.
    expect(screen.getByText(/Coverage gaps/)).toBeInTheDocument();
    expect(screen.getByText(/Stale entries/)).toBeInTheDocument();
  });

  it("shows an unavailable notice when the registry is missing", async () => {
    vi.mocked(api.fetchCopDrift).mockResolvedValue({
      registry_available: false,
      registry_version: "",
      coverage_gaps: null,
      stale: null,
    });

    render(<AdminCopInventorySection />);

    await waitFor(() => {
      expect(screen.getByText(/cookstyle binary is unavailable/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/Coverage gaps/)).not.toBeInTheDocument();
  });

  it("renders empty-state messages when there is no drift", async () => {
    vi.mocked(api.fetchCopDrift).mockResolvedValue({
      registry_available: true,
      registry_version: "8.6.10",
      coverage_gaps: [],
      stale: [],
    });

    render(<AdminCopInventorySection />);

    await waitFor(() => {
      expect(screen.getByText(/No coverage gaps/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/No stale entries/i)).toBeInTheDocument();
  });

  it("surfaces a load error with a retry affordance", async () => {
    vi.mocked(api.fetchCopDrift).mockRejectedValue(new Error("boom"));

    render(<AdminCopInventorySection />);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("boom");
    });
  });
});
