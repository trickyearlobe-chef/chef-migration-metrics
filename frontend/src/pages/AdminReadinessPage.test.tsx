// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AdminReadinessPage } from "./AdminReadinessPage";

vi.mock("../api");

const mockReadiness = {
  install_path_linux: "/hab",
  install_path_windows: "C:\\hab",
  install_size_mb_linux: 3072,
  install_size_mb_windows: 6144,
  min_remaining_free_percent: 20,
  review_blocks_readiness: false,
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

  it("renders the review-blocks-readiness toggle unchecked by default", async () => {
    render(<AdminReadinessPage />);
    await waitFor(() => screen.getByText("Readiness Policy"));
    const toggle = screen.getByRole("checkbox", {
      name: /Review-level cookbooks block readiness/,
    });
    expect(toggle).not.toBeChecked();
  });

  it("enables save and persists review_blocks_readiness when toggled on", async () => {
    const saveSpy = vi
      .mocked(api.saveReadinessConfig)
      .mockResolvedValue({ value: { ...mockReadiness, review_blocks_readiness: true } } as never);

    render(<AdminReadinessPage />);
    await waitFor(() => screen.getByText("Readiness Policy"));

    const toggle = screen.getByRole("checkbox", {
      name: /Review-level cookbooks block readiness/,
    });
    await userEvent.click(toggle);

    const save = screen.getByRole("button", { name: "Save" });
    expect(save).toBeEnabled();
    await userEvent.click(save);

    await waitFor(() => expect(saveSpy).toHaveBeenCalledTimes(1));
    expect(saveSpy).toHaveBeenCalledWith(
      expect.objectContaining({ review_blocks_readiness: true }),
    );
  });
});
