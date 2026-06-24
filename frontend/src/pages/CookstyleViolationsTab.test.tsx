// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import type { CookstyleViolationsResponse } from "../types";

vi.mock("../api");

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
    targetChefVersion: "18.5.0",
    targetVersions: ["18.5.0"],
    setTargetChefVersion: vi.fn(),
    staleTiers: [],
    setStaleTiers: vi.fn(),
    versionsLoading: false,
  }),
}));

// Import after mocks are set up
import { CookstyleViolationsTab } from "./CookstyleViolationsTab";

const mockViolationsResponse: CookstyleViolationsResponse = {
  data: [
    {
      source: "server",
      name: "example-cookbook",
      version: "1.2.3",
      organisation: "acme",
      target_chef_version: "18.5.0",
      passed: false,
      offence_count: 12,
      deprecation_count: 5,
      correctness_count: 2,
      scanned_at: "2026-06-20T10:00:00Z",
      namespace_counts: {
        "Chef/Deprecations/": 5,
        "Chef/Correctness/": 2,
      },
      severity_counts: { warning: 4, error: 8 },
      top_cops: [
        "Chef/Deprecations/NodeSet",
        "Chef/Correctness/InvalidDefaultAction",
        "Chef/Deprecations/ResourceWithoutUnifiedMode",
      ],
    },
    {
      source: "server",
      name: "passed-cookbook",
      version: "2.0.0",
      organisation: "acme",
      target_chef_version: "18.5.0",
      passed: true,
      offence_count: 0,
      deprecation_count: 0,
      correctness_count: 0,
      scanned_at: "2026-06-20T10:00:00Z",
      namespace_counts: {},
      severity_counts: {},
      top_cops: [],
    },
  ],
  pagination: {
    page: 1,
    per_page: 50,
    total_items: 2,
    total_pages: 1,
  },
};

const emptyResponse: CookstyleViolationsResponse = {
  data: [],
  pagination: { page: 1, per_page: 50, total_items: 0, total_pages: 0 },
};

function renderTab(initialEntries: string[] = ["/remediation?tab=violations"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <CookstyleViolationsTab />
    </MemoryRouter>,
  );
}

describe("CookstyleViolationsTab", () => {
  beforeEach(() => {
    vi.mocked(api.fetchCookstyleViolations).mockResolvedValue(
      mockViolationsResponse,
    );
  });

  it("renders the source toggle with Server Cookbooks active by default", async () => {
    renderTab();
    await waitFor(() =>
      expect(screen.getByText("example-cookbook")).toBeInTheDocument(),
    );
    expect(screen.getByText("Server Cookbooks")).toBeInTheDocument();
    expect(screen.getByText("Git Repos")).toBeInTheDocument();
  });

  it("renders violations in the table", async () => {
    renderTab();
    await waitFor(() =>
      expect(screen.getByText("example-cookbook")).toBeInTheDocument(),
    );
    expect(screen.getByText("passed-cookbook")).toBeInTheDocument();
    expect(screen.getAllByText("Failed").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Passed").length).toBeGreaterThan(0);
  });

  it("shows empty state when no results", async () => {
    vi.mocked(api.fetchCookstyleViolations).mockResolvedValue(emptyResponse);
    renderTab();
    await waitFor(() =>
      expect(
        screen.getByText("No violations match the current filters"),
      ).toBeInTheDocument(),
    );
  });

  it("switches source to git repos", async () => {
    renderTab();
    await waitFor(() =>
      expect(screen.getByText("example-cookbook")).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByText("Git Repos"));

    await waitFor(() => {
      expect(api.fetchCookstyleViolations).toHaveBeenCalledWith(
        expect.objectContaining({ source: "git" }),
      );
    });
  });

  it("passes filters to the API", async () => {
    renderTab();
    await waitFor(() =>
      expect(screen.getByText("example-cookbook")).toBeInTheDocument(),
    );

    // Change namespace filter
    const namespaceSelect = screen.getByLabelText("Namespace");
    fireEvent.change(namespaceSelect, {
      target: { value: "Chef/Deprecations/" },
    });

    await waitFor(() => {
      expect(api.fetchCookstyleViolations).toHaveBeenCalledWith(
        expect.objectContaining({ namespace: "Chef/Deprecations/" }),
      );
    });
  });

  it("renders top cops with truncation", async () => {
    renderTab();
    await waitFor(() =>
      expect(screen.getByText("example-cookbook")).toBeInTheDocument(),
    );
    // First 3 cops should be shown
    expect(
      screen.getByText(/Chef\/Deprecations\/NodeSet/),
    ).toBeInTheDocument();
  });

  it("renders organisation column for server source", async () => {
    renderTab();
    await waitFor(() =>
      expect(screen.getByText("example-cookbook")).toBeInTheDocument(),
    );
    expect(screen.getAllByText("acme").length).toBeGreaterThan(0);
  });
});
