// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api");

const mockUseAuth = vi.fn();

vi.mock("../context/AuthContext", () => ({
  useAuth: () => mockUseAuth(),
}));

// eslint-disable-next-line import/first
import { OwnerAliasesPage } from "./OwnerAliasesPage";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

describe("OwnerAliasesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchOwnerAliases).mockResolvedValue({ aliases: [] });
    vi.mocked(api.suggestOwnerAliases).mockResolvedValue({ suggestions: [] });
    vi.mocked(api.createOwnerAlias).mockResolvedValue({
      id: "alias-1",
      owner_name: "platform-team",
      alias_type: "email",
      alias_value: "platform-team@example.com",
      source: "manual",
      created_at: "2025-01-01T00:00:00Z",
    });
    vi.mocked(api.deleteOwnerAlias).mockResolvedValue(undefined);
  });

  it("renders page title", () => {
    render(<OwnerAliasesPage />, { wrapper: Wrapper });

    expect(
      screen.getByRole("heading", { name: "Owner Aliases" }),
    ).toBeInTheDocument();
  });

  it("shows Add Alias form for operators", () => {
    render(<OwnerAliasesPage />, { wrapper: Wrapper });

    expect(
      screen.getByRole("heading", { name: "Add Alias" }),
    ).toBeInTheDocument();
  });

  it("hides Add Alias form for viewers", () => {
    mockUseAuth.mockReturnValue({
      isOperator: false,
      isAdmin: false,
      user: { role: "viewer", username: "test" },
    });

    render(<OwnerAliasesPage />, { wrapper: Wrapper });

    expect(
      screen.queryByRole("heading", { name: "Add Alias" }),
    ).not.toBeInTheDocument();
  });

  it("renders Browse tab by default", () => {
    render(<OwnerAliasesPage />, { wrapper: Wrapper });

    expect(screen.getByText("Browse aliases by owner")).toBeInTheDocument();
  });
});
