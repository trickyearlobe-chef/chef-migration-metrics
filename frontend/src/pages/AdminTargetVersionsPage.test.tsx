// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AdminTargetVersionsPage } from "./AdminTargetVersionsPage";

vi.mock("../api");

const mockVersions = ["18.5.0", "19.1.0"];

describe("AdminTargetVersionsPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchTargetVersions).mockResolvedValue(mockVersions as never);
  });

  it("renders page heading", async () => {
    render(<AdminTargetVersionsPage />);
    await waitFor(() =>
      expect(screen.getByText("Target Chef Versions")).toBeInTheDocument(),
    );
  });

  it("loads and displays existing versions as chips", async () => {
    render(<AdminTargetVersionsPage />);
    await waitFor(() => {
      expect(screen.getByText("18.5.0")).toBeInTheDocument();
      expect(screen.getByText("19.1.0")).toBeInTheDocument();
    });
  });

  it("validates semver: entering an invalid version shows validation error", async () => {
    const user = userEvent.setup();
    render(<AdminTargetVersionsPage />);
    await waitFor(() => screen.getByText("Target Chef Versions"));

    await user.type(screen.getByPlaceholderText("e.g. 18.5.0"), "not-a-version");
    await user.click(screen.getByRole("button", { name: "Add" }));

    expect(
      screen.getByText(/invalid version format/i),
    ).toBeInTheDocument();
  });

  it("adds a valid semver version as a chip", async () => {
    const user = userEvent.setup();
    render(<AdminTargetVersionsPage />);
    await waitFor(() => screen.getByText("Target Chef Versions"));

    await user.type(screen.getByPlaceholderText("e.g. 18.5.0"), "20.0.0");
    await user.click(screen.getByRole("button", { name: "Add" }));

    expect(screen.getByText("20.0.0")).toBeInTheDocument();
  });

  it("removes a version by clicking its remove button", async () => {
    const user = userEvent.setup();
    render(<AdminTargetVersionsPage />);
    await waitFor(() => screen.getByText("18.5.0"));

    const removeButtons = screen.getAllByTitle("Remove");
    await user.click(removeButtons[0]);

    expect(screen.queryByText("18.5.0")).not.toBeInTheDocument();
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminTargetVersionsPage />);
    await waitFor(() => screen.getByText("Target Chef Versions"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});
