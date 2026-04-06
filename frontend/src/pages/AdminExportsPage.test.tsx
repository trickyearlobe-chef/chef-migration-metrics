// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import { AdminExportsPage } from "./AdminExportsPage";

vi.mock("../api");

const mockExports = {
  max_rows: 100000,
  async_threshold: 10000,
  output_directory: "",
  retention_hours: 24,
};

describe("AdminExportsPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchExportsConfig).mockResolvedValue(mockExports as never);
  });

  it("renders page heading", async () => {
    render(<AdminExportsPage />);
    await waitFor(() =>
      expect(screen.getByText("Export Settings")).toBeInTheDocument(),
    );
  });

  it("loads and displays max_rows value", async () => {
    render(<AdminExportsPage />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("100000")).toBeInTheDocument(),
    );
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminExportsPage />);
    await waitFor(() => screen.getByText("Export Settings"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});
