// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import * as api from "../api";
import { AdminAnalysisToolsPage } from "./AdminAnalysisToolsPage";

vi.mock("../api", async (importOriginal) => {
  const actual = await importOriginal<typeof api>();
  return {
    ...actual,
    fetchAnalysisTools: vi.fn(),
    saveAnalysisTools: vi.fn(),
  };
});

const mockAnalysisToolsResponse = {
  value: {
    embedded_bin_dir: "/opt/chef/embedded/bin",
    cookstyle_enabled: true,
    cookstyle_timeout_minutes: 30,
  },
  effective_failure_rules: { "*": ["error", "fatal"] },
};

describe("AdminAnalysisToolsPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchAnalysisTools).mockResolvedValue(mockAnalysisToolsResponse as never);
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

  it("renders the CookStyle Failure Rules section", async () => {
    render(<AdminAnalysisToolsPage />);
    await waitFor(() =>
      expect(screen.getByText("CookStyle Failure Rules")).toBeInTheDocument(),
    );
  });

  it("renders the failure rules grid with preset dropdown", async () => {
    render(<AdminAnalysisToolsPage />);
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: /preset/i })).toBeInTheDocument(),
    );
    expect(screen.getByRole("combobox", { name: /preset/i })).toHaveValue("default");
  });

  it("enables save button when failure rules change", async () => {
    render(<AdminAnalysisToolsPage />);
    await waitFor(() => screen.getByText("CookStyle Failure Rules"));
    const presetSelect = screen.getByRole("combobox", { name: /preset/i });
    fireEvent.change(presetSelect, { target: { value: "strict" } });
    expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
  });

  it("shows verdict count in success message after save", async () => {
    vi.mocked(api.saveAnalysisTools).mockResolvedValue({
      value: mockAnalysisToolsResponse.value,
      restartRequired: false,
      verdictsChanged: 5,
    } as never);
    render(<AdminAnalysisToolsPage />);
    await waitFor(() => screen.getByText("CookStyle Failure Rules"));
    // Change preset to make dirty
    fireEvent.change(screen.getByRole("combobox", { name: /preset/i }), {
      target: { value: "strict" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(screen.getByText(/5 cookbook verdicts changed/)).toBeInTheDocument(),
    );
  });
});

