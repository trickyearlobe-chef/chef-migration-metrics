// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return { ...actual, fetchOwners: vi.fn(), createOwner: vi.fn() };
});

const mockUseAuth = vi.fn();
vi.mock("../context/AuthContext", () => ({ useAuth: () => mockUseAuth() }));

import { OwnersPage } from "./OwnersPage";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

function asRole(role: "admin" | "operator") {
  mockUseAuth.mockReturnValue({
    isAdmin: role === "admin",
    isOperator: true,
    user: { role, username: "test" },
  });
}

// Importing owners and reconciling duplicate people became administrator
// functions on 2026-08-06. The page has to stop offering them to everybody
// else: a button that bounces you back to the dashboard is worse than no
// button, because it reads as a fault in the product rather than as a
// permission you do not have.
describe("owner administration entry points", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchOwners).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 50, total_items: 0, total_pages: 0 },
    } as Awaited<ReturnType<typeof api.fetchOwners>>);
  });

  it("offers import and duplicates to an admin, under the admin section", async () => {
    asRole("admin");
    render(<OwnersPage />, { wrapper: Wrapper });

    expect(
      await screen.findByRole("link", { name: "Import owners" }),
    ).toHaveAttribute("href", "/admin/ownership/import");
    expect(
      screen.getByRole("link", { name: "Possible Duplicates" }),
    ).toHaveAttribute("href", "/admin/ownership/duplicates");
  });

  it("offers neither to somebody who is not an admin", async () => {
    asRole("operator");
    render(<OwnersPage />, { wrapper: Wrapper });

    // Aliases stays available — it is not one of the two being restricted,
    // so its presence proves the page rendered rather than failed.
    expect(await screen.findByRole("link", { name: "Aliases" })).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: "Import owners" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Possible Duplicates" }),
    ).not.toBeInTheDocument();
  });
});
