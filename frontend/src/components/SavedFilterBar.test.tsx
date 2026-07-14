// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SavedFilterBar } from "./SavedFilterBar";
import {
  listSavedFilters,
  createSavedFilter,
  updateSavedFilter,
  deleteSavedFilter,
} from "../api/savedFilters";
import { ApiError } from "../api/client";
import type { SavedFilter } from "../types";

vi.mock("../api/savedFilters", () => ({
  listSavedFilters: vi.fn(),
  createSavedFilter: vi.fn(),
  updateSavedFilter: vi.fn(),
  deleteSavedFilter: vi.fn(),
}));

vi.mock("../context/AuthContext", () => ({
  useAuth: () => ({ user: { username: "me" } }),
}));

const mine: SavedFilter = {
  id: "id-mine",
  name: "All Windows OS",
  view: "nodes",
  filters: { role: ["win-base", "win-iis"] },
  owner_username: "me",
  shared: false,
  created_at: "2026-07-14T10:00:00Z",
  updated_at: "2026-07-14T10:00:00Z",
};

const theirs: SavedFilter = {
  id: "id-theirs",
  name: "All RHEL OS",
  view: "nodes",
  filters: { role: ["rhel-base"] },
  owner_username: "org-a-user",
  shared: true,
  created_at: "2026-07-14T10:00:00Z",
  updated_at: "2026-07-14T10:00:00Z",
};

function renderBar(props: Partial<React.ComponentProps<typeof SavedFilterBar>> = {}) {
  const onApply = props.onApply ?? vi.fn();
  const result = render(
    <SavedFilterBar
      view="nodes"
      currentParams={{ platform: ["windows"] }}
      onApply={onApply}
      {...props}
    />,
  );
  return { ...result, onApply };
}

/** Open the panel — the list is only fetched when the operator opens it. */
async function open() {
  fireEvent.click(screen.getByRole("button", { name: /saved filters/i }));
  await screen.findByText("All Windows OS");
}

describe("SavedFilterBar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listSavedFilters).mockResolvedValue([mine, theirs]);
    vi.mocked(createSavedFilter).mockResolvedValue(mine);
    vi.mocked(updateSavedFilter).mockResolvedValue({ ...mine, shared: true });
    vi.mocked(deleteSavedFilter).mockResolvedValue(undefined);
  });

  it("lists the caller's own filters and shared ones, attributing the shared", async () => {
    renderBar();
    await open();

    expect(listSavedFilters).toHaveBeenCalledWith("nodes");
    expect(screen.getByText("All Windows OS")).toBeTruthy();
    expect(screen.getByText("All RHEL OS")).toBeTruthy();
    // A shared filter shows whose it is; your own does not need attributing.
    expect(screen.getByText(/shared by org-a-user/i)).toBeTruthy();
  });

  it("applies the stored selection verbatim", async () => {
    const { onApply } = renderBar();
    await open();

    fireEvent.click(screen.getByRole("button", { name: /apply All Windows OS/i }));

    expect(onApply).toHaveBeenCalledWith({ role: ["win-base", "win-iis"] });
  });

  it("saves the current selection under a name", async () => {
    renderBar({ currentParams: { platform: ["windows"], role: ["win-base"] } });
    await open();

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "All Windows OS" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));

    await waitFor(() =>
      expect(createSavedFilter).toHaveBeenCalledWith({
        name: "All Windows OS",
        view: "nodes",
        filters: { platform: ["windows"], role: ["win-base"] },
        shared: false,
      }),
    );
    // The list refreshes so the new filter is immediately applicable.
    await waitFor(() => expect(listSavedFilters).toHaveBeenCalledTimes(2));
  });

  it("surfaces the backend's duplicate-name message rather than inventing one", async () => {
    vi.mocked(createSavedFilter).mockRejectedValue(
      new ApiError(409, 'a saved filter named "All Windows OS" already exists', ""),
    );
    renderBar();
    await open();

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "All Windows OS" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));

    expect(await screen.findByText(/already exists/i)).toBeTruthy();
  });

  it("will not save an unnamed filter", async () => {
    renderBar();
    await open();

    expect(screen.getByRole("button", { name: /^save$/i })).toHaveProperty(
      "disabled",
      true,
    );
    expect(createSavedFilter).not.toHaveBeenCalled();
  });

  // Shared is read-only to a non-owner: they may apply it, but the manage
  // controls are the owner's alone (the backend enforces this with a 403; the
  // UI must not offer the action in the first place).
  it("offers manage controls on own filters only", async () => {
    renderBar();
    await open();

    expect(screen.getByRole("button", { name: /rename All Windows OS/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /delete All Windows OS/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /share All Windows OS/i })).toBeTruthy();

    expect(screen.queryByRole("button", { name: /rename All RHEL OS/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /delete All RHEL OS/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /unshare All RHEL OS/i })).toBeNull();
    // ...but a non-owner can still apply it.
    expect(screen.getByRole("button", { name: /apply All RHEL OS/i })).toBeTruthy();
  });

  it("shares an own filter with one PATCH", async () => {
    renderBar();
    await open();

    fireEvent.click(screen.getByRole("button", { name: /share All Windows OS/i }));

    await waitFor(() =>
      expect(updateSavedFilter).toHaveBeenCalledWith("id-mine", { shared: true }),
    );
  });

  it("renames an own filter with one PATCH", async () => {
    renderBar();
    await open();

    fireEvent.click(screen.getByRole("button", { name: /rename All Windows OS/i }));
    fireEvent.change(screen.getByDisplayValue("All Windows OS"), {
      target: { value: "All Windows Server" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^confirm rename$/i }));

    await waitFor(() =>
      expect(updateSavedFilter).toHaveBeenCalledWith("id-mine", {
        name: "All Windows Server",
      }),
    );
  });

  it("deletes an own filter and refreshes", async () => {
    renderBar();
    await open();

    fireEvent.click(screen.getByRole("button", { name: /delete All Windows OS/i }));
    fireEvent.click(screen.getByRole("button", { name: /^confirm delete$/i }));

    await waitFor(() => expect(deleteSavedFilter).toHaveBeenCalledWith("id-mine"));
    await waitFor(() => expect(listSavedFilters).toHaveBeenCalledTimes(2));
  });

  it("updates an existing filter's selection to the current one", async () => {
    renderBar({ currentParams: { role: ["win-base", "win-iis", "win-sql"] } });
    await open();

    fireEvent.click(
      screen.getByRole("button", { name: /update All Windows OS to current selection/i }),
    );

    await waitFor(() =>
      expect(updateSavedFilter).toHaveBeenCalledWith("id-mine", {
        filters: { role: ["win-base", "win-iis", "win-sql"] },
      }),
    );
  });
});
