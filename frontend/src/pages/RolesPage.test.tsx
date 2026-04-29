// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import type { RoleListResponse } from "../types";

vi.mock("../api");

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

// RolesPage must be imported after the mocks are set up.
// eslint-disable-next-line import/first
import { RolesPage } from "./RolesPage";

const mockResponse: RoleListResponse = {
  data: [
    {
      role_name: "webserver",
      organisations: ["org-a", "org-b"],
      node_count: 42,
      direct_cookbook_count: 3,
      transitive_cookbook_count: 5,
      total_cookbook_count: 5,
      compatibility_status: "incompatible",
      compatible_count: 3,
      incompatible_count: 2,
      untested_count: 0,
    },
    {
      role_name: "base",
      organisations: ["org-a"],
      node_count: 100,
      direct_cookbook_count: 2,
      transitive_cookbook_count: 2,
      total_cookbook_count: 2,
      compatibility_status: "compatible",
      compatible_count: 2,
      incompatible_count: 0,
      untested_count: 0,
    },
  ],
  summary: {
    target_chef_version: "18.5.0",
    compatible_roles: 1,
    incompatible_roles: 1,
    untested_roles: 0,
    total_roles: 2,
  },
  pagination: {
    page: 1,
    per_page: 50,
    total_items: 2,
    total_pages: 1,
  },
};

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

describe("RolesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchRoles).mockResolvedValue(mockResponse);
  });

  it("shows loading spinner initially", () => {
    vi.mocked(api.fetchRoles).mockReturnValue(new Promise(() => {}));
    render(<RolesPage />, { wrapper: Wrapper });
    expect(screen.getByText("Loading roles…")).toBeInTheDocument();
  });

  it("renders role table after loading", async () => {
    render(<RolesPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByRole("table")).toBeInTheDocument();
    });
    const table = screen.getByRole("table");
    expect(within(table).getByText("webserver")).toBeInTheDocument();
    expect(within(table).getByText("base")).toBeInTheDocument();
    expect(within(table).getByText("Name")).toBeInTheDocument();
    expect(within(table).getByText("Nodes")).toBeInTheDocument();
    expect(within(table).getByText("Cookbooks")).toBeInTheDocument();
    expect(within(table).getByText("CookStyle")).toBeInTheDocument();
    expect(within(table).getByText("Test Kitchen")).toBeInTheDocument();
  });

  it("role names are links to role detail pages", async () => {
    render(<RolesPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByRole("table")).toBeInTheDocument();
    });
    const webserverLink = screen.getByRole("link", { name: "webserver" });
    expect(webserverLink).toHaveAttribute("href", "/roles/webserver");
    const baseLink = screen.getByRole("link", { name: "base" });
    expect(baseLink).toHaveAttribute("href", "/roles/base");
  });

  it("shows error on fetch failure", async () => {
    vi.mocked(api.fetchRoles).mockRejectedValue(new Error("Network error"));
    render(<RolesPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByText("Network error")).toBeInTheDocument();
  });

  it("summary bar renders compatible and incompatible counts", async () => {
    render(<RolesPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByRole("table")).toBeInTheDocument();
    });
    expect(screen.getByText(/Compatible:\s*1/)).toBeInTheDocument();
    expect(screen.getByText(/Incompatible:\s*1/)).toBeInTheDocument();
  });

  it("clicking summary bar compatible segment sets compatibility filter", async () => {
    const user = userEvent.setup();
    render(<RolesPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByRole("table")).toBeInTheDocument();
    });
    vi.mocked(api.fetchRoles).mockClear();
    const compatibleButton = screen.getByTitle(/Compatible:.*click to filter/);
    await user.click(compatibleButton);
    await waitFor(() => {
      expect(vi.mocked(api.fetchRoles)).toHaveBeenCalledWith(
        expect.objectContaining({ compatibility_status: "compatible" }),
      );
    });
  });

  it("clicking summary bar incompatible label sets filter", async () => {
    const user = userEvent.setup();
    render(<RolesPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByRole("table")).toBeInTheDocument();
    });
    vi.mocked(api.fetchRoles).mockClear();
    const incompatibleLabel = screen.getByText(/Incompatible:\s*1/);
    await user.click(incompatibleLabel);
    await waitFor(() => {
      expect(vi.mocked(api.fetchRoles)).toHaveBeenCalledWith(
        expect.objectContaining({ compatibility_status: "incompatible" }),
      );
    });
  });
});
