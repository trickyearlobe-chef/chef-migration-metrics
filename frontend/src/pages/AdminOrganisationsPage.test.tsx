// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AdminOrganisationsPage } from "./AdminOrganisationsPage";

vi.mock("../api");

const mockCredentials = {
  data: [
    {
      name: "my-key",
      credential_type: "chef_client_key",
      created_by: "admin",
      created_at: "",
      updated_at: "",
    },
  ],
  pagination: { page: 1, per_page: 50, total: 1, total_pages: 1 },
};

describe("AdminOrganisationsPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchConfigOrganisations).mockResolvedValue([] as never);
    vi.mocked(api.fetchCredentials).mockResolvedValue(mockCredentials as never);
  });

  it("renders page heading", async () => {
    render(<AdminOrganisationsPage />);
    await waitFor(() =>
      expect(screen.getByText("Organisations")).toBeInTheDocument(),
    );
  });

  it("shows empty state message when no organisations are configured", async () => {
    render(<AdminOrganisationsPage />);
    await waitFor(() =>
      expect(
        screen.getByText(/no organisations configured/i),
      ).toBeInTheDocument(),
    );
  });

  it("add button adds a new organisation card", async () => {
    const user = userEvent.setup();
    render(<AdminOrganisationsPage />);
    await waitFor(() => screen.getByText("Organisations"));

    await user.click(screen.getByRole("button", { name: /add organisation/i }));

    expect(screen.getByText("New Organisation")).toBeInTheDocument();
  });

  it("remove button on a card removes it", async () => {
    const user = userEvent.setup();
    render(<AdminOrganisationsPage />);
    await waitFor(() => screen.getByText("Organisations"));

    await user.click(screen.getByRole("button", { name: /add organisation/i }));
    expect(screen.getByText("New Organisation")).toBeInTheDocument();

    await user.click(screen.getByTitle("Remove"));
    expect(screen.queryByText("New Organisation")).not.toBeInTheDocument();
  });
});
