// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import { AdminAnalysisToolsPage } from "./AdminAnalysisToolsPage";

vi.mock("../api");

const mockAnalysisTools = {
  embedded_bin_dir: "/opt/chef/embedded/bin",
  cookstyle_enabled: true,
  cookstyle_timeout_minutes: 30,
};

describe("AdminAnalysisToolsPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchAnalysisTools).mockResolvedValue(mockAnalysisTools as never);
  });

  it("renders page heading", async () => {
    render(<AdminAnalysisToolsPage />);
    await waitFor(() =>
      expect(screen.getByText("Analysis Tools")).toBeInTheDocument(),
    );
  });

  it("loads and displays embedded_bin_dir value", async () => {
    render(<AdminAnalysisToolsPage />);
    await waitFor(() =>
      expect(
        screen.getByDisplayValue("/opt/chef/embedded/bin"),
      ).toBeInTheDocument(),
    );
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminAnalysisToolsPage />);
    await waitFor(() => screen.getByText("Analysis Tools"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});
