// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import * as api from "../api";
import type { CookbookRemediationResponse } from "../types";

vi.mock("../api");

vi.mock("../context/GlobalFilterContext", () => ({
  useGlobalFilters: () => ({
    targetChefVersion: "18.0",
    targetVersions: ["18.0"],
    setTargetChefVersion: vi.fn(),
    staleTiers: [],
    setStaleTiers: vi.fn(),
    versionsLoading: false,
  }),
}));

// Import after mocks
import { CookbookRemediationPage } from "./CookbookRemediationPage";

function baseResponse(): CookbookRemediationResponse {
  return {
    cookbook_name: "apt",
    cookbook_version: "1.0.0",
    target_chef_version: "18.0",
    complexity_score: 0,
    complexity_label: "low",
    cookstyle_passed: false,
    cookstyle_status: "needs_review",
    scanned_at: "2026-01-01T00:00:00Z",
    statistics: {
      total_offenses: 0,
      correctable_offenses: 0,
      remaining_offenses: 0,
      auto_correctable_count: 0,
      manual_fix_count: 0,
      deprecation_count: 0,
      error_count: 0,
      offense_groups: 0,
    },
    offense_groups: [],
    autocorrect_preview: {
      available: false,
      total_offenses: 0,
      correctable_offenses: 0,
      remaining_offenses: 0,
      files_modified: 0,
      diff_output: "",
    },
  };
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/cookbooks/apt/1.0.0/remediation"]}>
      <Routes>
        <Route
          path="/cookbooks/:name/:version/remediation"
          element={<CookbookRemediationPage />}
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe("CookbookRemediationPage won't-parse banner", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the won't-parse banner when cookstyle_wont_parse is true", async () => {
    vi.mocked(api.fetchCookbookRemediation).mockResolvedValue({
      ...baseResponse(),
      cookstyle_wont_parse: true,
    } as never);

    renderPage();

    await waitFor(() =>
      expect(screen.getByText("Won't parse — fix first")).toBeInTheDocument(),
    );
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("does not show the banner when cookstyle_wont_parse is false", async () => {
    vi.mocked(api.fetchCookbookRemediation).mockResolvedValue({
      ...baseResponse(),
      cookstyle_wont_parse: false,
    } as never);

    renderPage();

    await waitFor(() =>
      expect(screen.getByText(/Remediation detail for target Chef/)).toBeInTheDocument(),
    );
    expect(screen.queryByText("Won't parse — fix first")).not.toBeInTheDocument();
  });
});
