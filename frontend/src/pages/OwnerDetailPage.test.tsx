// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import * as api from "../api";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchOwnerDetail: vi.fn(),
    fetchAssignments: vi.fn(),
    fetchOwnerAliases: vi.fn(),
    createOwnerAlias: vi.fn(),
    deleteOwnerAlias: vi.fn(),
    fetchOwners: vi.fn(),
    mergeOwners: vi.fn(),
  };
});

const mockUseAuth = vi.fn();
vi.mock("../context/AuthContext", () => ({ useAuth: () => mockUseAuth() }));

import { OwnerDetailPage } from "./OwnerDetailPage";

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/ownership/thomas-smith"]}>
      <Routes>
        <Route path="/ownership/:name" element={<OwnerDetailPage />} />
        <Route path="/ownership/tommy-smith" element={<div>tommy-smith page</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("OwnerDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({ user: { role: "admin", username: "test" } });
    vi.mocked(api.fetchOwnerDetail).mockResolvedValue({
      name: "thomas-smith",
      owner_type: "individual",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    } as never);
    vi.mocked(api.fetchAssignments).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 25, total_items: 0, total_pages: 0 },
    } as never);
    vi.mocked(api.fetchOwnerAliases).mockResolvedValue({
      aliases: [
        {
          id: "alias-1",
          owner_name: "thomas-smith",
          alias_type: "custom",
          alias_value: "Fat Tommy",
          source: "import",
          created_at: "2026-01-01T00:00:00Z",
        },
      ],
    });
    vi.mocked(api.fetchOwners).mockResolvedValue({
      data: [{ name: "tommy-smith", owner_type: "individual" }],
      pagination: { page: 1, per_page: 8, total_items: 1, total_pages: 1 },
    } as never);
    vi.mocked(api.mergeOwners).mockResolvedValue({
      from_owner: "thomas-smith",
      into_owner: "tommy-smith",
      reassigned: 0,
      skipped: 0,
      aliases_moved: 1,
      aliases_dropped: 0,
      source_name_aliased: true,
    });
  });

  // What they own and what they are called are different questions, and both
  // belong on the person.
  it("shows what this owner is called elsewhere, on their own page", async () => {
    renderPage();

    expect(await screen.findByText("Fat Tommy")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add alias" })).toBeInTheDocument();
  });

  it("merges this owner into one picked by searching owners, not aliases", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Fat Tommy");

    await user.click(screen.getByRole("button", { name: "Merge into…" }));
    await user.type(screen.getByLabelText("Merge into"), "tommy");

    // Owners are searched by name, which is the only search that can find an
    // owner that has no alias yet.
    await waitFor(() =>
      expect(api.fetchOwners).toHaveBeenCalledWith({ search: "tommy", per_page: 8 }),
    );
    await user.click(await screen.findByRole("button", { name: /tommy-smith/ }));
    await user.click(screen.getByRole("button", { name: "Merge owners" }));

    await waitFor(() =>
      expect(api.mergeOwners).toHaveBeenCalledWith({
        from_owner: "thomas-smith",
        into_owner: "tommy-smith",
      }),
    );
    // The merged-away owner no longer exists, so staying on its page would
    // show a 404.
    expect(await screen.findByText("tommy-smith page")).toBeInTheDocument();
  });

  it("offers no merge to an operator, who cannot delete an owner", async () => {
    mockUseAuth.mockReturnValue({ user: { role: "operator", username: "test" } });
    renderPage();
    await screen.findByText("Fat Tommy");

    expect(screen.queryByRole("button", { name: "Merge into…" })).not.toBeInTheDocument();
    // Aliases stay editable — that is an operator's job.
    expect(screen.getByRole("button", { name: "Add alias" })).toBeInTheDocument();
  });
});
