// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return { ...actual, fetchOwnerAliases: vi.fn(), createOwnerAlias: vi.fn(), deleteOwnerAlias: vi.fn() };
});

import { OwnerAliasesCard } from "./OwnerAliasesCard";

const alias = {
  id: "alias-1",
  owner_name: "thomas-smith",
  alias_type: "custom",
  alias_value: "Fat Tommy",
  source: "import",
  created_at: "2026-01-01T00:00:00Z",
};

describe("OwnerAliasesCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchOwnerAliases).mockResolvedValue({ aliases: [alias] });
    vi.mocked(api.createOwnerAlias).mockResolvedValue({
      ...alias,
      id: "alias-2",
      alias_type: "email",
      alias_value: "thomas.smith@example-corp.test",
    });
    vi.mocked(api.deleteOwnerAlias).mockResolvedValue(undefined);
  });

  it("lists the aliases this owner is known by", async () => {
    render(<OwnerAliasesCard ownerName="thomas-smith" canEdit />);

    expect(await screen.findByText("Fat Tommy")).toBeInTheDocument();
    expect(api.fetchOwnerAliases).toHaveBeenCalledWith("thomas-smith");
  });

  // The whole complaint about the old screen: two owner boxes, one filled and
  // one not. On the person's own page there is nothing to ask.
  it("has no owner field — the page is already the owner", async () => {
    render(<OwnerAliasesCard ownerName="thomas-smith" canEdit />);
    await screen.findByText("Fat Tommy");

    expect(screen.queryByLabelText(/owner name/i)).not.toBeInTheDocument();
  });

  it("offers only the alias types the database accepts", async () => {
    render(<OwnerAliasesCard ownerName="thomas-smith" canEdit />);
    await screen.findByText("Fat Tommy");

    const select = screen.getByLabelText("Alias type") as HTMLSelectElement;
    const offered = Array.from(select.options).map((o) => o.value);
    expect(offered).toEqual(["email", "git_email", "git_name", "username", "custom"]);
  });

  it("adds an alias against this owner and reloads the list", async () => {
    const user = userEvent.setup();
    render(<OwnerAliasesCard ownerName="thomas-smith" canEdit />);
    await screen.findByText("Fat Tommy");

    await user.selectOptions(screen.getByLabelText("Alias type"), "email");
    await user.type(screen.getByLabelText("Alias value"), "thomas.smith@example-corp.test");
    await user.click(screen.getByRole("button", { name: "Add alias" }));

    await waitFor(() =>
      expect(api.createOwnerAlias).toHaveBeenCalledWith({
        owner_name: "thomas-smith",
        alias_type: "email",
        alias_value: "thomas.smith@example-corp.test",
      }),
    );
    expect(api.fetchOwnerAliases).toHaveBeenCalledTimes(2);
  });

  it("removes an alias", async () => {
    const user = userEvent.setup();
    render(<OwnerAliasesCard ownerName="thomas-smith" canEdit />);
    await screen.findByText("Fat Tommy");

    await user.click(screen.getByRole("button", { name: /remove/i }));

    await waitFor(() => expect(api.deleteOwnerAlias).toHaveBeenCalledWith("alias-1"));
  });

  it("shows the list but no editing controls to a viewer", async () => {
    render(<OwnerAliasesCard ownerName="thomas-smith" canEdit={false} />);

    expect(await screen.findByText("Fat Tommy")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add alias" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /remove/i })).not.toBeInTheDocument();
  });

  it("says an alias is already taken rather than failing silently", async () => {
    const user = userEvent.setup();
    vi.mocked(api.createOwnerAlias).mockRejectedValue(
      new Error("This alias is already assigned to an owner."),
    );
    render(<OwnerAliasesCard ownerName="thomas-smith" canEdit />);
    await screen.findByText("Fat Tommy");

    await user.type(screen.getByLabelText("Alias value"), "taken@example-corp.test");
    await user.click(screen.getByRole("button", { name: "Add alias" }));

    expect(
      await screen.findByText("This alias is already assigned to an owner."),
    ).toBeInTheDocument();
  });
});
