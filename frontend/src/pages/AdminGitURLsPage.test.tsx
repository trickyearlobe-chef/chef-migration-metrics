// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AdminGitURLsPage } from "./AdminGitURLsPage";

vi.mock("../api");

const mockURLs = ["https://github.com/my-org"];

describe("AdminGitURLsPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchGitURLs).mockResolvedValue(mockURLs as never);
    vi.mocked(api.saveGitURLs).mockResolvedValue({
      value: mockURLs,
      restartRequired: false,
    } as never);
  });

  it("renders page heading", async () => {
    render(<AdminGitURLsPage />);
    await waitFor(() =>
      expect(screen.getByText("Git Base URLs")).toBeInTheDocument(),
    );
  });

  it("loads and displays the existing URL", async () => {
    render(<AdminGitURLsPage />);
    await waitFor(() =>
      expect(
        screen.getByDisplayValue("https://github.com/my-org"),
      ).toBeInTheDocument(),
    );
  });

  it("add URL button adds an empty input", async () => {
    const user = userEvent.setup();
    render(<AdminGitURLsPage />);
    await waitFor(() => screen.getByText("Git Base URLs"));

    const inputsBefore = screen.getAllByRole("textbox");
    await user.click(screen.getByRole("button", { name: /add url/i }));

    expect(screen.getAllByRole("textbox")).toHaveLength(inputsBefore.length + 1);
  });

  it("remove button removes the URL", async () => {
    const user = userEvent.setup();
    render(<AdminGitURLsPage />);
    await waitFor(() =>
      screen.getByDisplayValue("https://github.com/my-org"),
    );

    await user.click(screen.getByTitle("Remove"));

    expect(
      screen.queryByDisplayValue("https://github.com/my-org"),
    ).not.toBeInTheDocument();
  });

  it("save button is disabled when no changes (isDirty = false)", async () => {
    render(<AdminGitURLsPage />);
    await waitFor(() => screen.getByText("Git Base URLs"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("successful save shows success banner", async () => {
    const user = userEvent.setup();
    render(<AdminGitURLsPage />);
    await waitFor(() =>
      screen.getByDisplayValue("https://github.com/my-org"),
    );

    // Make a change to enable the save button
    await user.click(screen.getByTitle("Remove"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(
        screen.getByText(/git urls saved successfully/i),
      ).toBeInTheDocument(),
    );
  });

  it("move up reorders the URLs in place", async () => {
    vi.mocked(api.fetchGitURLs).mockResolvedValue([
      "https://github.com/org-a",
      "https://github.com/org-b",
    ] as never);
    const user = userEvent.setup();
    render(<AdminGitURLsPage />);
    await waitFor(() =>
      screen.getByDisplayValue("https://github.com/org-b"),
    );

    // Row 0's "Move up" is disabled; row 1's moves org-b above org-a.
    await user.click(screen.getAllByTitle("Move up")[1]);

    const inputs = screen.getAllByRole("textbox");
    expect(inputs[0]).toHaveValue("https://github.com/org-b");
    expect(inputs[1]).toHaveValue("https://github.com/org-a");
    // Reordering makes the form dirty so it can be saved.
    expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
  });

  it("save error from rejected promise shows error alert", async () => {
    vi.mocked(api.saveGitURLs).mockRejectedValue(new Error("Server error"));
    const user = userEvent.setup();
    render(<AdminGitURLsPage />);
    await waitFor(() =>
      screen.getByDisplayValue("https://github.com/my-org"),
    );

    await user.click(screen.getByTitle("Remove"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(screen.getByText(/server error/i)).toBeInTheDocument(),
    );
  });
});
