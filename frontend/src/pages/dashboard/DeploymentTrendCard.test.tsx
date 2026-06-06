// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../../api";
import { DeploymentTrendCard } from "./TrendCards";
import type { DeploymentTrendResponse } from "../../types";

vi.mock("../../api");
vi.mock("../../context/GlobalFilterContext", () => ({
  useGlobalFilters: () => ({ staleTiers: [] }),
}));

const emptyResponse: DeploymentTrendResponse = { data: [] };

const trendResponse: DeploymentTrendResponse = {
  data: [
    {
      organisation_name: "prod",
      collection_run_org: "run-1",
      completed_at: "2025-06-14T12:00:00Z",
      total_nodes: 100,
      staged_or_activated: 30,
      converge_passing: 20,
    },
    {
      organisation_name: "prod",
      collection_run_org: "run-2",
      completed_at: "2025-06-15T12:00:00Z",
      total_nodes: 100,
      staged_or_activated: 50,
      converge_passing: 45,
    },
  ],
};

describe("DeploymentTrendCard", () => {
  beforeEach(() => {
    vi.mocked(api.fetchDeploymentTrend).mockResolvedValue(emptyResponse);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("shows loading state then renders chart title", async () => {
    vi.mocked(api.fetchDeploymentTrend).mockResolvedValue(trendResponse);
    render(<DeploymentTrendCard />);
    expect(
      screen.getByText("Loading deployment trend…"),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(
        screen.getByText("Deployment Progress — Trend"),
      ).toBeInTheDocument(),
    );
  });

  it("shows description text", async () => {
    vi.mocked(api.fetchDeploymentTrend).mockResolvedValue(trendResponse);
    render(<DeploymentTrendCard />);
    await waitFor(() =>
      expect(
        screen.getByText(/nightly speculative converge is passing/),
      ).toBeInTheDocument(),
    );
  });

  it("renders error state on API failure", async () => {
    vi.mocked(api.fetchDeploymentTrend).mockRejectedValue(
      new Error("network error"),
    );
    render(<DeploymentTrendCard />);
    await waitFor(() =>
      expect(screen.getByText("network error")).toBeInTheDocument(),
    );
  });

  it("renders empty chart when no data", async () => {
    vi.mocked(api.fetchDeploymentTrend).mockResolvedValue(emptyResponse);
    render(<DeploymentTrendCard />);
    await waitFor(() =>
      expect(
        screen.getByText("Deployment Progress — Trend"),
      ).toBeInTheDocument(),
    );
    // Chart still renders (empty series) — no crash
  });

  it("passes organisation to API", async () => {
    vi.mocked(api.fetchDeploymentTrend).mockResolvedValue(emptyResponse);
    render(<DeploymentTrendCard organisation="staging" />);
    await waitFor(() =>
      expect(api.fetchDeploymentTrend).toHaveBeenCalledWith("staging"),
    );
  });
});
