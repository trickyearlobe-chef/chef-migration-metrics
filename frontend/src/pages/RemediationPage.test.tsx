// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

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

// Import after mocks
import { RemediationPage } from "./RemediationPage";

const mockPriorityResponse = {
  target_chef_version: "18.5.0",
  total_cookbooks: 1,
  total_auto_correctable: 5,
  total_manual_fix: 2,
  total_deprecations: 3,
  total_errors: 1,
  data: [
    {
      cookbook_name: "test-cookbook",
      cookbook_version: "1.0.0",
      cookbook_id: "cb-1",
      complexity_score: 10,
      complexity_label: "low",
      affected_node_count: 5,
      affected_role_count: 1,
      priority_score: 50,
      auto_correctable_count: 3,
      manual_fix_count: 1,
      deprecation_count: 2,
      error_count: 0,
      target_chef_version: "18.5.0",
      version_count: 1,
    },
  ],
  pagination: { page: 1, per_page: 50, total_items: 1, total_pages: 1 },
};

const mockSummaryResponse = {
  target_chef_version: "18.5.0",
  total_cookbooks_evaluated: 10,
  total_needing_remediation: 3,
  quick_wins: 2,
  manual_fixes: 1,
  blocked_nodes_by_complexity: 5,
  blocked_nodes_by_readiness: 2,
  total_auto_correctable: 8,
  total_manual_fix: 3,
};

const mockCopAnalysisResponse = {
  summary: {
    blocker_cops: 2,
    blocker_cookbooks: 5,
    review_cops: 1,
    review_cookbooks: 2,
    noise_cops: 3,
    unclassified_cops: 1,
  },
  data: [
    {
      cop_name: "Chef/Deprecations/NodeSet",
      description: "Do not use node.set",
      category: "Chef/Deprecations/",
      severity: "warning",
      classification: "blocker",
      classification_source: "curated_default",
      removed_in: "14.0",
      cookbooks_affected: 5,
      total_offences: 12,
      auto_correctable_pct: 100,
      unblocks: 3,
      is_custom: false,
    },
  ],
  pagination: { page: 1, per_page: 50, total_items: 1, total_pages: 1 },
};

function renderPage(initialEntries: string[] = ["/remediation"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <RemediationPage />
    </MemoryRouter>,
  );
}

describe("RemediationPage tabs", () => {
  beforeEach(() => {
    vi.mocked(api.fetchRemediationPriority).mockResolvedValue(
      mockPriorityResponse as never,
    );
    vi.mocked(api.fetchRemediationSummary).mockResolvedValue(
      mockSummaryResponse as never,
    );
    vi.mocked(api.fetchFilterComplexityLabels).mockResolvedValue({
      data: ["low", "medium", "high"],
    } as never);
    vi.mocked(api.fetchCookstyleCops).mockResolvedValue(
      mockCopAnalysisResponse as never,
    );
  });

  it("shows Priority tab as default", async () => {
    renderPage();
    await waitFor(() =>
      expect(screen.getByText("Remediation Priority")).toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: "Priority" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cop Analysis" })).toBeInTheDocument();
  });

  it("switches to Cop Analysis tab on click", async () => {
    renderPage();
    await waitFor(() =>
      expect(screen.getByText("Remediation Priority")).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByText("Cop Analysis"));

    await waitFor(() =>
      expect(screen.getByText("Chef/Deprecations/NodeSet")).toBeInTheDocument(),
    );
  });

  it("renders cop analysis tab directly when ?tab=cop-analysis", async () => {
    renderPage(["/remediation?tab=cop-analysis"]);
    await waitFor(() =>
      expect(screen.getByText("Chef/Deprecations/NodeSet")).toBeInTheDocument(),
    );
  });

  it("switching back to Priority tab shows priority content", async () => {
    renderPage(["/remediation?tab=cop-analysis"]);
    await waitFor(() =>
      expect(screen.getByText("Chef/Deprecations/NodeSet")).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByText("Priority"));

    await waitFor(() =>
      expect(screen.getByText("Remediation Priority")).toBeInTheDocument(),
    );
  });
});
