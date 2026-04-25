// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AdminPlatformDisplayNamesPage } from "./AdminPlatformDisplayNamesPage";

vi.mock("../api", () => ({
  fetchPlatformDisplayNames: vi.fn(),
  updatePlatformDisplayNames: vi.fn(),
  resetPlatformDisplayNames: vi.fn(),
}));

import {
  fetchPlatformDisplayNames,
  updatePlatformDisplayNames,
  resetPlatformDisplayNames,
} from "../api";

const defaultMappings = [
  { platform: "windows", version_prefix: "10.0.22631", display_name: "Win11 23H2" },
  { platform: "centos", version_prefix: "8", display_name: "CentOS 8 (EOL)" },
];

describe("AdminPlatformDisplayNamesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(fetchPlatformDisplayNames).mockResolvedValue({
      mappings: defaultMappings,
      is_default: false,
    });
    vi.mocked(updatePlatformDisplayNames).mockResolvedValue({
      mappings: defaultMappings,
      is_default: false,
    });
    vi.mocked(resetPlatformDisplayNames).mockResolvedValue({
      mappings: defaultMappings,
      is_default: true,
    });
  });

  it("renders loading state then displays mappings", async () => {
    render(<AdminPlatformDisplayNamesPage />);
    expect(screen.getByText(/loading/i)).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("windows")).toBeInTheDocument();
      expect(screen.getByText("10.0.22631")).toBeInTheDocument();
      expect(screen.getByText("Win11 23H2")).toBeInTheDocument();
      expect(screen.getByText("centos")).toBeInTheDocument();
      expect(screen.getByText("CentOS 8 (EOL)")).toBeInTheDocument();
    });
  });

  it("shows Reset button only when not default", async () => {
    render(<AdminPlatformDisplayNamesPage />);
    await waitFor(() => screen.getByText("windows"));

    expect(screen.getByRole("button", { name: /reset to defaults/i })).toBeInTheDocument();
  });

  it("hides Reset button when mappings are default", async () => {
    vi.mocked(fetchPlatformDisplayNames).mockResolvedValue({
      mappings: defaultMappings,
      is_default: true,
    });
    render(<AdminPlatformDisplayNamesPage />);
    await waitFor(() => screen.getByText("windows"));

    expect(screen.queryByRole("button", { name: /reset to defaults/i })).not.toBeInTheDocument();
  });

  it("can add a new mapping", async () => {
    const user = userEvent.setup();
    render(<AdminPlatformDisplayNamesPage />);
    await waitFor(() => screen.getByText("windows"));

    await user.click(screen.getByRole("button", { name: /add mapping/i }));

    const platformInput = screen.getByTestId("edit-platform");
    const versionInput = screen.getByTestId("edit-version-prefix");
    const displayInput = screen.getByTestId("edit-display-name");

    await user.type(platformInput, "ubuntu");
    await user.type(versionInput, "22.04");
    await user.type(displayInput, "Ubuntu 22.04 LTS");

    await user.click(screen.getByRole("button", { name: "OK" }));

    expect(screen.getByText("ubuntu")).toBeInTheDocument();
    expect(screen.getByText("22.04")).toBeInTheDocument();
    expect(screen.getByText("Ubuntu 22.04 LTS")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() => {
      expect(updatePlatformDisplayNames).toHaveBeenCalledWith([
        ...defaultMappings,
        { platform: "ubuntu", version_prefix: "22.04", display_name: "Ubuntu 22.04 LTS" },
      ]);
    });
  });

  it("can delete a mapping", async () => {
    const user = userEvent.setup();
    render(<AdminPlatformDisplayNamesPage />);
    await waitFor(() => screen.getByText("windows"));

    const deleteButtons = screen.getAllByRole("button", { name: /delete/i });
    await user.click(deleteButtons[0]);

    expect(screen.queryByText("windows")).not.toBeInTheDocument();
    expect(screen.getByText("centos")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() => {
      expect(updatePlatformDisplayNames).toHaveBeenCalledWith([
        { platform: "centos", version_prefix: "8", display_name: "CentOS 8 (EOL)" },
      ]);
    });
  });

  it("shows error on fetch failure", async () => {
    vi.mocked(fetchPlatformDisplayNames).mockRejectedValue(new Error("Network error"));
    render(<AdminPlatformDisplayNamesPage />);

    await waitFor(() => {
      expect(screen.getByText("Network error")).toBeInTheDocument();
    });
  });

  it("reset calls API and updates state", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);

    render(<AdminPlatformDisplayNamesPage />);
    await waitFor(() => screen.getByText("windows"));

    await user.click(screen.getByRole("button", { name: /reset to defaults/i }));

    await waitFor(() => {
      expect(resetPlatformDisplayNames).toHaveBeenCalled();
      expect(screen.getByText(/reset to defaults/i)).toBeInTheDocument();
    });

    vi.mocked(window.confirm).mockRestore();
  });
});
