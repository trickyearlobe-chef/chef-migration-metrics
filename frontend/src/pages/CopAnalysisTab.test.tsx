// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render,
  screen,
  waitFor,
  fireEvent,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api");

vi.mock("../context/GlobalFilterContext", () => ({
  useGlobalFilters: () => ({
    targetChefVersion: "18.5.0",
    targetVersions: ["18.5.0"],
    setTargetChefVersion: vi.fn(),
    staleTiers: [],
    setStaleTiers: vi.fn(),
    versionsLoading: false,
  }),
}));

// Import after mocks
import { CopAnalysisTab } from "./CopAnalysisTab";

const COP = "Chef/Deprecations/NodeSet";

const mockCopList = {
  summary: {
    blocker_cops: 1,
    blocker_cookbooks: 2,
    review_cops: 0,
    review_cookbooks: 0,
    noise_cops: 0,
    unclassified_cops: 0,
  },
  data: [
    {
      cop_name: COP,
      description: "Do not use node.set",
      category: "Chef/Deprecations/",
      severity: "warning",
      classification: "blocker",
      classification_source: "verified_removal",
      removed_in: "14.0",
      cookbooks_affected: 2,
      total_offences: 12,
      auto_correctable_pct: 100,
      unblocks: 3,
      is_custom: false,
    },
  ],
  pagination: { page: 1, per_page: 50, total_items: 1, total_pages: 1 },
};

const mockServerGroups = {
  cop_name: COP,
  grouped: true,
  data: [
    {
      source: "server",
      name: "cb-one",
      version_count: 2,
      offence_count: 5,
      auto_correctable: 2,
      would_pass_without: true,
      versions: [
        {
          source: "server",
          name: "cb-one",
          version: "2.0.0",
          organisation: "org-a",
          offence_count: 3,
          auto_correctable: 1,
          would_pass_without: true,
        },
        {
          source: "server",
          name: "cb-one",
          version: "1.0.0",
          organisation: "org-b",
          offence_count: 2,
          auto_correctable: 1,
          would_pass_without: true,
        },
      ],
    },
  ],
  pagination: { page: 1, per_page: 20, total_items: 42, total_pages: 3 },
};

const mockGitRepos = {
  cop_name: COP,
  data: [
    {
      source: "git",
      name: "repo-x",
      version: "abc123",
      offence_count: 4,
      auto_correctable: 0,
      would_pass_without: false,
    },
  ],
  pagination: { page: 1, per_page: 20, total_items: 1, total_pages: 1 },
};

function renderTab(source: "server" | "git") {
  return render(
    <MemoryRouter initialEntries={["/remediation"]}>
      <CopAnalysisTab source={source} />
    </MemoryRouter>,
  );
}

describe("CopAnalysisTab", () => {
  beforeEach(() => {
    vi.mocked(api.fetchCookstyleCops).mockResolvedValue(mockCopList as never);
    vi.mocked(api.fetchCookstyleServerCopCookbooks).mockResolvedValue(
      mockServerGroups as never,
    );
    vi.mocked(api.fetchCookstyleCopCookbooks).mockResolvedValue(
      mockGitRepos as never,
    );
  });

  it("passes the fixed source to the cop list query (server)", async () => {
    renderTab("server");
    await waitFor(() => expect(screen.getByText(COP)).toBeInTheDocument());
    expect(api.fetchCookstyleCops).toHaveBeenCalledWith(
      expect.objectContaining({ source: "server" }),
    );
  });

  it("server drill-down groups by name and expands to version/org detail", async () => {
    renderTab("server");
    await waitFor(() => expect(screen.getByText(COP)).toBeInTheDocument());

    // Expand the cop → grouped-by-name drill-down.
    fireEvent.click(screen.getByText(COP));
    await waitFor(() => expect(screen.getByText("cb-one")).toBeInTheDocument());
    expect(api.fetchCookstyleServerCopCookbooks).toHaveBeenCalledWith(
      COP,
      expect.objectContaining({ page: 1, per_page: 20 }),
    );
    // Version detail is hidden until the group is expanded.
    expect(screen.queryByText("2.0.0")).not.toBeInTheDocument();

    // Expand the cookbook name → its version/org rows.
    fireEvent.click(screen.getByRole("button", { name: /cb-one/ }));
    await waitFor(() => expect(screen.getByText("2.0.0")).toBeInTheDocument());
    expect(screen.getByText("org-a")).toBeInTheDocument();
    expect(screen.getByText("1.0.0")).toBeInTheDocument();
  });

  it("surfaces drill-down pagination (showing X of N)", async () => {
    renderTab("server");
    await waitFor(() => expect(screen.getByText(COP)).toBeInTheDocument());

    fireEvent.click(screen.getByText(COP));
    // The drill-down has 42 total items across 3 pages — the total must be shown,
    // not silently dropped (the "1942 vs 20" bug).
    await waitFor(() => expect(screen.getByText(/of 42/)).toBeInTheDocument());
  });

  it("resets the open drill-down when the classification filter changes", async () => {
    renderTab("server");
    await waitFor(() => expect(screen.getByText(COP)).toBeInTheDocument());

    fireEvent.click(screen.getByText(COP));
    await waitFor(() => expect(screen.getByText("cb-one")).toBeInTheDocument());

    // Changing the classification filter must collapse the stale panel.
    fireEvent.click(screen.getByRole("button", { name: "Blockers" }));
    await waitFor(() =>
      expect(screen.queryByText("cb-one")).not.toBeInTheDocument(),
    );
  });

  it("git tab lists repositories flat (no grouping)", async () => {
    renderTab("git");
    await waitFor(() => expect(screen.getByText(COP)).toBeInTheDocument());
    expect(api.fetchCookstyleCops).toHaveBeenCalledWith(
      expect.objectContaining({ source: "git" }),
    );

    fireEvent.click(screen.getByText(COP));
    await waitFor(() => expect(screen.getByText("repo-x")).toBeInTheDocument());
    expect(api.fetchCookstyleCopCookbooks).toHaveBeenCalledWith(
      COP,
      expect.objectContaining({ source: "git", page: 1, per_page: 20 }),
    );
    // Flat list header, and no version-grouping affordance.
    const drill = screen.getByText("repo-x").closest("table")!;
    expect(within(drill).getByText("Repository")).toBeInTheDocument();
  });
});
