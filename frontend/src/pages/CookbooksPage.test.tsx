// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchCookbooks: vi.fn(),
    fetchSavedFilters: vi.fn(),
    fetchOwners: vi.fn(),
  };
});

// SavedFilterBar imports these directly, not through the api barrel.
vi.mock("../api/savedFilters", () => ({
  listSavedFilters: vi.fn(),
  createSavedFilter: vi.fn(),
  updateSavedFilter: vi.fn(),
  deleteSavedFilter: vi.fn(),
}));
import * as savedFiltersApi from "../api/savedFilters";

vi.mock("../context/OrgContext", () => ({
  useOrg: vi.fn().mockReturnValue({
    selectedOrg: "",
    organisations: [],
    loading: false,
    error: null,
    setSelectedOrg: vi.fn(),
    refresh: vi.fn(),
  }),
}));

vi.mock("../hooks/useTargetChefVersion", () => ({
  useTargetChefVersion: vi.fn().mockReturnValue({
    selectedVersion: "18.5.0",
    targetVersions: ["18.5.0"],
    versionsLoading: false,
  }),
}));

vi.mock("../context/AuthContext", () => ({
  useAuth: vi.fn().mockReturnValue({ user: { username: "test" } }),
}));

import { CookbooksPage } from "./CookbooksPage";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

// ---------------------------------------------------------------------------
// The ownership filter. As on the git repo list, the backend has answered both
// ownership questions for some time and no screen could ask either.
// ---------------------------------------------------------------------------

describe("CookbooksPage — the ownership filter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 25, total_items: 0, total_pages: 0 },
    });
    if (vi.isMockFunction(api.fetchSavedFilters)) {
      vi.mocked(api.fetchSavedFilters).mockResolvedValue({ data: [] });
    }
    vi.mocked(api.fetchOwners).mockResolvedValue({
      data: [
        {
          name: "alice.brown",
          display_name: "Alice Brown",
          owner_type: "person",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      pagination: { page: 1, per_page: 50, total_items: 1, total_pages: 1 },
    });
  });

  it("asks the API for one person's cookbooks", async () => {
    const user = userEvent.setup();
    render(<CookbooksPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchCookbooks).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );

    await waitFor(() => {
      expect(api.fetchCookbooks).toHaveBeenLastCalledWith(
        expect.objectContaining({ owner: "alice.brown" }),
      );
    });
  });

  it("asks the API for the cookbooks with nobody", async () => {
    const user = userEvent.setup();
    render(<CookbooksPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchCookbooks).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));

    await waitFor(() => {
      const last = vi.mocked(api.fetchCookbooks).mock.calls.at(-1)?.[0];
      expect(last?.unowned).toBe("true");
      expect(last?.owner).toBeUndefined();
    });
  });

  // The API returns 400 when both are sent.
  it("never sends an owner and the no-owner question together", async () => {
    const user = userEvent.setup();
    render(<CookbooksPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchCookbooks).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));

    for (const [call] of vi.mocked(api.fetchCookbooks).mock.calls) {
      expect(call?.owner && call?.unowned).toBeFalsy();
    }
  });

  // The chips get a row of their own under the bar, so picking several owners
  // cannot push the other filters sideways.
  it("shows the chosen owner as a chip below the filter bar, not inside it", async () => {
    const user = userEvent.setup();
    render(<CookbooksPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchCookbooks).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );

    const chip = await screen.findByRole("button", {
      name: "Remove alice.brown",
    });
    expect(screen.getByTestId("filter-bar")).not.toContainElement(chip);
  });

  // The same cohort question on the cookbook list.
  it("saves the chosen owner as part of the cohort", async () => {
    vi.mocked(savedFiltersApi.listSavedFilters).mockResolvedValue([]);
    const user = userEvent.setup();
    render(<CookbooksPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchCookbooks).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );
    await screen.findByRole("button", { name: "Remove alice.brown" });

    await user.click(screen.getByRole("button", { name: /Saved filters/ }));
    fireEvent.change(screen.getByPlaceholderText(/save current selection as/i), {
      target: { value: "Alice's cookbooks" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(savedFiltersApi.createSavedFilter).toHaveBeenCalledWith(
        expect.objectContaining({
          filters: expect.objectContaining({ owner: ["alice.brown"] }),
        }),
      );
    });
  });

  it("drops the ownership filter from the request when cleared", async () => {
    const user = userEvent.setup();
    render(<CookbooksPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchCookbooks).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));
    await waitFor(() => {
      expect(vi.mocked(api.fetchCookbooks).mock.calls.at(-1)?.[0]?.unowned).toBe(
        "true",
      );
    });

    await user.click(screen.getByRole("button", { name: /Clear \(/i }));

    await waitFor(() => {
      const last = vi.mocked(api.fetchCookbooks).mock.calls.at(-1)?.[0];
      expect(last?.unowned).toBeUndefined();
      expect(last?.owner).toBeUndefined();
    });
  });
});
