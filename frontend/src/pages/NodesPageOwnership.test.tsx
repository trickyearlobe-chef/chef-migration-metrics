// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchNodes: vi.fn(),
    fetchOwners: vi.fn(),
    fetchFilterPolicyNames: vi.fn(),
    fetchFilterPolicyGroups: vi.fn(),
    fetchFilterEnvironments: vi.fn(),
    fetchFilterPlatforms: vi.fn(),
    fetchFilterRoles: vi.fn(),
  };
});

// SavedFilterBar reaches for these directly, not through the api barrel.
vi.mock("../api/savedFilters", () => ({
  listSavedFilters: vi.fn(),
  createSavedFilter: vi.fn(),
  updateSavedFilter: vi.fn(),
  deleteSavedFilter: vi.fn(),
}));
import * as savedFiltersApi from "../api/savedFilters";

vi.mock("../context/AuthContext", () => ({
  useAuth: () => ({
    isOperator: true,
    isAdmin: true,
    user: { role: "admin", username: "test" },
  }),
}));

vi.mock("../context/OrgContext", () => ({
  useOrg: () => ({
    selectedOrg: "",
    organisations: [],
    loading: false,
    error: null,
    setSelectedOrg: vi.fn(),
    refresh: vi.fn(),
  }),
}));

vi.mock("../context/GlobalFilterContext", () => ({
  useGlobalFilters: () => ({
    staleTiers: [],
    setStaleTiers: vi.fn(),
    targetChefVersion: "",
    setTargetChefVersion: vi.fn(),
  }),
  GlobalFilterProvider: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock("../hooks/useTargetChefVersion", () => ({
  useTargetChefVersion: () => ({ selectedTargetVersion: "", loading: false }),
}));

import { NodesPage } from "./NodesPage";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

// Node ownership answers "which of my machines am I on the hook for" — the same
// pair of questions the git repo and cookbook lists answer, on the list that
// matters most for planning an upgrade.
describe("NodesPage — the ownership filter", () => {
  beforeEach(() => {
    vi.mocked(api.fetchNodes).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 25, total_items: 0, total_pages: 0 },
    } as never);
    for (const fn of [
      api.fetchFilterPolicyNames,
      api.fetchFilterPolicyGroups,
      api.fetchFilterEnvironments,
      api.fetchFilterPlatforms,
      api.fetchFilterRoles,
    ]) {
      vi.mocked(fn as never as () => Promise<string[]>).mockResolvedValue([]);
    }
    vi.mocked(savedFiltersApi.listSavedFilters).mockResolvedValue([]);
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
    } as never);
  });

  it("asks the API for one person's nodes", async () => {
    const user = userEvent.setup();
    render(<NodesPage />, { wrapper: Wrapper });
    await waitFor(() => expect(api.fetchNodes).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );

    await waitFor(() => {
      expect(vi.mocked(api.fetchNodes).mock.calls.at(-1)?.[0]?.owner).toBe(
        "alice.brown",
      );
    });
  });

  it("asks the API for the nodes with nobody", async () => {
    const user = userEvent.setup();
    render(<NodesPage />, { wrapper: Wrapper });
    await waitFor(() => expect(api.fetchNodes).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));

    await waitFor(() => {
      const last = vi.mocked(api.fetchNodes).mock.calls.at(-1)?.[0];
      expect(last?.unowned).toBe("true");
      expect(last?.owner).toBeUndefined();
    });
  });

  // The API answers 400 to both together, so the pair must never leave the page.
  it("never sends an owner and the no-owner question together", async () => {
    const user = userEvent.setup();
    render(<NodesPage />, { wrapper: Wrapper });
    await waitFor(() => expect(api.fetchNodes).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));

    for (const [call] of vi.mocked(api.fetchNodes).mock.calls) {
      expect(call?.owner && call?.unowned).toBeFalsy();
    }
  });

  it("shows the chosen owner as a chip below the filter bar, not inside it", async () => {
    const user = userEvent.setup();
    render(<NodesPage />, { wrapper: Wrapper });
    await waitFor(() => expect(api.fetchNodes).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );

    const chip = await screen.findByRole("button", {
      name: "Remove alice.brown",
    });
    expect(screen.getByTestId("filter-bar")).not.toContainElement(chip);
  });

  it("drops the ownership filter from the request when cleared", async () => {
    const user = userEvent.setup();
    render(<NodesPage />, { wrapper: Wrapper });
    await waitFor(() => expect(api.fetchNodes).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));
    await waitFor(() =>
      expect(vi.mocked(api.fetchNodes).mock.calls.at(-1)?.[0]?.unowned).toBe(
        "true",
      ),
    );

    await user.click(screen.getByRole("button", { name: /Clear \(/i }));

    await waitFor(() => {
      const last = vi.mocked(api.fetchNodes).mock.calls.at(-1)?.[0];
      expect(last?.unowned).toBeUndefined();
      expect(last?.owner).toBeUndefined();
    });
  });
});
