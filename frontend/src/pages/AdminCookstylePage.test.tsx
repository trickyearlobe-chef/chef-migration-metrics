// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import * as api from "../api";
import { AdminCookstylePage } from "./AdminCookstylePage";

vi.mock("../api", async (importOriginal) => {
  const actual = await importOriginal<typeof api>();
  return {
    ...actual,
    fetchAnalysisTools: vi.fn(),
    saveAnalysisTools: vi.fn(),
    fetchTargetVersions: vi.fn(),
    fetchCookstyleCops: vi.fn(),
  };
});

const mockResponse = {
  value: {
    embedded_bin_dir: "/opt/chef/embedded/bin",
    cookstyle_enabled: true,
    cookstyle_timeout_minutes: 30,
  },
  effective_failure_rules: { "*": ["error", "fatal"] },
};

describe("AdminCookstylePage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchAnalysisTools).mockResolvedValue(mockResponse as never);
    vi.mocked(api.fetchTargetVersions).mockResolvedValue(["18.5.0"]);
    vi.mocked(api.fetchCookstyleCops).mockResolvedValue({
      summary: {
        blocker_cops: 0,
        blocker_cookbooks: 0,
        review_cops: 0,
        review_cookbooks: 0,
        noise_cops: 0,
        unclassified_cops: 0,
      },
      data: [],
      pagination: { page: 1, per_page: 200, total_items: 0, total_pages: 0 },
    });
  });

  it("renders page heading", async () => {
    render(<AdminCookstylePage />);
    await waitFor(() =>
      expect(screen.getByText("CookStyle")).toBeInTheDocument(),
    );
  });

  it("renders the enabled toggle", async () => {
    render(<AdminCookstylePage />);
    await waitFor(() =>
      expect(screen.getByRole("switch")).toBeInTheDocument(),
    );
  });

  it("renders timeout field with loaded value", async () => {
    render(<AdminCookstylePage />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("30")).toBeInTheDocument(),
    );
  });

  it("renders the fallback rules grid", async () => {
    render(<AdminCookstylePage />);
    await waitFor(() =>
      expect(screen.getByText("Fallback Rules")).toBeInTheDocument(),
    );
    expect(screen.getByRole("combobox", { name: /preset/i })).toHaveValue("default");
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminCookstylePage />);
    await waitFor(() => screen.getByText("CookStyle"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("enables save when failure rules change", async () => {
    render(<AdminCookstylePage />);
    await waitFor(() => screen.getByText("CookStyle"));
    fireEvent.change(screen.getByRole("combobox", { name: /preset/i }), {
      target: { value: "strict" },
    });
    expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
  });

  it("shows verdict count toast after save", async () => {
    vi.mocked(api.saveAnalysisTools).mockResolvedValue({
      value: mockResponse.value,
      restartRequired: false,
      verdictsChanged: 3,
    } as never);
    render(<AdminCookstylePage />);
    await waitFor(() => screen.getByText("CookStyle"));
    fireEvent.change(screen.getByRole("combobox", { name: /preset/i }), {
      target: { value: "strict" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(screen.getByText(/3 cookbook verdicts changed/)).toBeInTheDocument(),
    );
  });

  it("shows success message without verdict count when zero", async () => {
    vi.mocked(api.saveAnalysisTools).mockResolvedValue({
      value: mockResponse.value,
      restartRequired: false,
      verdictsChanged: 0,
    } as never);
    render(<AdminCookstylePage />);
    await waitFor(() => screen.getByText("CookStyle"));
    fireEvent.change(screen.getByRole("combobox", { name: /preset/i }), {
      target: { value: "strict" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(screen.getByText("Settings saved successfully.")).toBeInTheDocument(),
    );
  });
});
