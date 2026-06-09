// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import * as api from "../api";
import type { RoleDetailResponse, RoleGraphResponse } from "../types";

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

import { RoleDetailPage } from "./RoleDetailPage";

const mockDetail: RoleDetailResponse = {
  role_name: "webserver",
  organisations: ["org-a", "org-b"],
  node_count: 42,
  direct_cookbooks: ["nginx", "ssl"],
  direct_roles: ["base"],
  transitive_cookbooks: ["nginx", "ssl", "apt", "base-cookbook"],
  blocking_cookbooks: [
    {
      cookbook_name: "nginx",
      cookbook_version: "5.1.0",
      target_chef_version: "18.5.0",
      complexity_score: 30,
      complexity_label: "medium",
      auto_correctable: 4,
      manual_fix: 3,
      dependency_path: ["role:webserver", "cookbook:nginx"],
    },
  ],
  nested_role_chain: {
    name: "webserver",
    type: "role",
    children: [
      {
        name: "base",
        type: "role",
        children: [
          { name: "apt", type: "cookbook", compatibility_status: "compatible" },
        ],
      },
      { name: "nginx", type: "cookbook", compatibility_status: "incompatible" },
      { name: "ssl", type: "cookbook", compatibility_status: "compatible" },
    ],
  },
  nodes_by_organisation: [
    { organisation: "org-a", count: 40 },
    { organisation: "org-b", count: 2 },
  ],
  nodes_by_environment: [{ environment: "production", count: 42 }],
  nodes_by_platform: [
    { platform: "ubuntu", platform_version: "22.04", count: 42 },
  ],
};

const mockGraph: RoleGraphResponse = {
  nodes: [
    { id: "role:webserver", type: "role", name: "webserver" },
    { id: "role:base", type: "role", name: "base" },
    {
      id: "cookbook:nginx",
      type: "cookbook",
      name: "nginx",
      compatibility_status: "incompatible",
    },
    {
      id: "cookbook:apt",
      type: "cookbook",
      name: "apt",
      compatibility_status: "compatible",
    },
  ],
  edges: [
    { from: "role:webserver", to: "role:base", type: "includes_role" },
    { from: "role:webserver", to: "cookbook:nginx", type: "includes_cookbook" },
    { from: "role:base", to: "cookbook:apt", type: "includes_cookbook" },
  ],
  metadata: {
    total_roles: 2,
    total_cookbooks: 2,
    incompatible_cookbooks: 1,
  },
};

function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter initialEntries={["/roles/webserver"]}>
      <Routes>
        <Route path="/roles/:name" element={children} />
      </Routes>
    </MemoryRouter>
  );
}

describe("RoleDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchRoleDetail).mockResolvedValue(mockDetail);
  });

  it("shows loading spinner", () => {
    vi.mocked(api.fetchRoleDetail).mockReturnValue(new Promise(() => {}));
    render(<RoleDetailPage />, { wrapper: Wrapper });
    expect(screen.getByText("Loading role detail…")).toBeInTheDocument();
  });

  it("renders role detail after loading", async () => {
    render(<RoleDetailPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "webserver" }),
      ).toBeInTheDocument();
    });
    expect(screen.getByText(/org-a, org-b/)).toBeInTheDocument();
    expect(screen.getAllByText("42").length).toBeGreaterThanOrEqual(1);
  });

  it("shows error on fetch failure", async () => {
    vi.mocked(api.fetchRoleDetail).mockRejectedValue(
      new Error("Network error"),
    );
    render(<RoleDetailPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByText("Network error")).toBeInTheDocument();
  });

  it("renders blocking cookbooks table", async () => {
    render(<RoleDetailPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByText("Blocking Cookbooks")).toBeInTheDocument();
    });
    const table = screen.getByRole("table");
    expect(table).toBeInTheDocument();
    // nginx appears as a link in the blocking cookbooks table
    const links = screen.getAllByRole("link", { name: "nginx" });
    expect(links.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("5.1.0")).toBeInTheDocument();
  });

  it("back link to roles list", async () => {
    render(<RoleDetailPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "webserver" }),
      ).toBeInTheDocument();
    });
    const backLink = screen.getByRole("link", { name: "← Roles" });
    expect(backLink).toHaveAttribute("href", "/roles");
  });

  it("dependency tree renders", async () => {
    render(<RoleDetailPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(screen.getByText("Dependency Tree")).toBeInTheDocument();
    });
    expect(screen.getAllByText("base").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("apt").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("ssl").length).toBeGreaterThanOrEqual(1);
  });

  it("tab bar renders with Overview and Dependency Graph", async () => {
    render(<RoleDetailPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "webserver" }),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: "Overview" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Dependency Graph" }),
    ).toBeInTheDocument();
  });

  it("overview tab is active by default", async () => {
    render(<RoleDetailPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "webserver" }),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("Compatibility Summary")).toBeInTheDocument();
    expect(screen.getByText("Blocking Cookbooks")).toBeInTheDocument();
    expect(screen.getByText("Blast Radius")).toBeInTheDocument();
  });

  describe("graph tab", () => {
    beforeEach(() => {
      let rafId = 0;
      vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => {
        return ++rafId;
      });
      vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});
      vi.mocked(api.fetchRoleDependencyGraph).mockResolvedValue(mockGraph);
    });

    afterEach(() => {
      vi.restoreAllMocks();
    });

    it("clicking Dependency Graph tab fetches graph data", async () => {
      const user = userEvent.setup();
      render(<RoleDetailPage />, { wrapper: Wrapper });
      await waitFor(() => {
        expect(
          screen.getByRole("heading", { name: "webserver" }),
        ).toBeInTheDocument();
      });

      await user.click(
        screen.getByRole("button", { name: "Dependency Graph" }),
      );

      await waitFor(() => {
        expect(vi.mocked(api.fetchRoleDependencyGraph)).toHaveBeenCalledWith(
          "webserver",
          expect.any(Object),
        );
      });

      await waitFor(() => {
        const svg = document.querySelector("svg");
        expect(svg).toBeInTheDocument();
      });
    });

    it("graph tab shows metadata summary", async () => {
      const user = userEvent.setup();
      render(<RoleDetailPage />, { wrapper: Wrapper });
      await waitFor(() => {
        expect(
          screen.getByRole("heading", { name: "webserver" }),
        ).toBeInTheDocument();
      });

      await user.click(
        screen.getByRole("button", { name: "Dependency Graph" }),
      );

      await waitFor(() => {
        expect(screen.getByText(/2 Roles/)).toBeInTheDocument();
      });
      expect(screen.getByText(/2 Cookbooks/)).toBeInTheDocument();
      expect(screen.getByText(/1 Incompatible/)).toBeInTheDocument();
    });

    it("graph tab shows loading state", async () => {
      vi.mocked(api.fetchRoleDependencyGraph).mockReturnValue(
        new Promise(() => {}),
      );
      const user = userEvent.setup();
      render(<RoleDetailPage />, { wrapper: Wrapper });
      await waitFor(() => {
        expect(
          screen.getByRole("heading", { name: "webserver" }),
        ).toBeInTheDocument();
      });

      await user.click(
        screen.getByRole("button", { name: "Dependency Graph" }),
      );

      await waitFor(() => {
        expect(screen.getByText(/Loading/i)).toBeInTheDocument();
      });
    });
  });
});
