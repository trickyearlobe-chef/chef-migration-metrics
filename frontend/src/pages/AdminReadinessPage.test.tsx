// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import { AdminReadinessPage } from "./AdminReadinessPage";

vi.mock("../api");

const mockReadiness = {
  install_path_linux: "/hab",
  install_path_windows: "C:\\hab",
  install_size_mb_linux: 3072,
  install_size_mb_windows: 6144,
  min_remaining_free_percent: 20,
};

describe("AdminReadinessPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchReadinessConfig).mockResolvedValue(mockReadiness as never);
  });

  it("renders page heading", async () => {
    render(<AdminReadinessPage />);
    await waitFor(() =>
      expect(screen.getByText("Upgrade Readiness")).toBeInTheDocument(),
    );
  });

  it("loads and displays linux install size", async () => {
    render(<AdminReadinessPage />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("3072")).toBeInTheDocument(),
    );
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminReadinessPage />);
    await waitFor(() => screen.getByText("Upgrade Readiness"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("does not show path warning when using defaults", async () => {
    render(<AdminReadinessPage />);
    await waitFor(() => screen.getByText("Upgrade Readiness"));
    expect(screen.queryByText(/Non-default install path/)).not.toBeInTheDocument();
  });
});
