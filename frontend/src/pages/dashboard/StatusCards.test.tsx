// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../../api";
import { CookbookCompatibilityCard, GitRepoCompatibilityCard } from "./StatusCards";
import type {
  CookbookCompatibilityResponse,
  GitRepoCompatibilityResponse,
} from "../../types";

vi.mock("../../api");

const cookbookResponse: CookbookCompatibilityResponse = {
  data: [
    {
      target_chef_version: "18.0.0",
      total_cookbooks: 4,
      ready_cookbooks: 1,
      needs_review_cookbooks: 1,
      blocked_cookbooks: 1,
      untested_cookbooks: 1,
      untested_errored_cookbooks: 0,
      untested_inactive_cookbooks: 0,
      untested_unscanned_cookbooks: 1,
      ready_percent: 25,
    },
  ],
};

const gitResponse: GitRepoCompatibilityResponse = {
  data: [
    {
      target_chef_version: "18.0.0",
      total_repos: 3,
      ready_repos: 1,
      needs_review_repos: 1,
      blocked_repos: 1,
      untested_repos: 0,
      untested_errored_repos: 0,
      untested_clone_failed_repos: 0,
      untested_pending_scan_repos: 0,
      ready_percent: 33,
    },
  ],
};

function renderInRouter(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("CookStyle compatibility cards use the 4-state rollup", () => {
  afterEach(() => vi.restoreAllMocks());

  it("cookbook card shows Ready/Needs review/Blocked/Untested, not Compatible/Incompatible", async () => {
    vi.mocked(api.fetchCookbookCompatibility).mockResolvedValue(cookbookResponse);
    renderInRouter(<CookbookCompatibilityCard />);

    await waitFor(() =>
      expect(screen.getByText(/Needs review: 1/)).toBeInTheDocument(),
    );
    expect(screen.getByText(/Ready: 1/)).toBeInTheDocument();
    expect(screen.getByText(/Blocked: 1/)).toBeInTheDocument();
    expect(screen.getByText(/Untested: 1/)).toBeInTheDocument();
    // Legacy vocabulary must be gone.
    expect(screen.queryByText(/Compatible:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Incompatible:/)).not.toBeInTheDocument();
  });

  it("cookbook needs_review segment links to the cookstyle_status filter", async () => {
    vi.mocked(api.fetchCookbookCompatibility).mockResolvedValue(cookbookResponse);
    renderInRouter(<CookbookCompatibilityCard />);

    const link = await screen.findByText(/Needs review: 1/);
    expect(link.closest("a")?.getAttribute("href")).toContain(
      "cookstyle_status=needs_review",
    );
  });

  it("git repo card shows the 4 rollup states, not Compatible/Incompatible", async () => {
    vi.mocked(api.fetchGitRepoCompatibility).mockResolvedValue(gitResponse);
    renderInRouter(<GitRepoCompatibilityCard />);

    await waitFor(() =>
      expect(screen.getByText(/Needs review: 1/)).toBeInTheDocument(),
    );
    expect(screen.getByText(/Ready: 1/)).toBeInTheDocument();
    expect(screen.getByText(/Blocked: 1/)).toBeInTheDocument();
    expect(screen.queryByText(/Compatible:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Incompatible:/)).not.toBeInTheDocument();
  });
});
