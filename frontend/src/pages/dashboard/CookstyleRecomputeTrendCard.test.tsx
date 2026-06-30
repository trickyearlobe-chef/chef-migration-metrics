// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../../api";
import { CookstyleRecomputeTrendCard } from "./TrendCards";
import type { CookstyleRecomputeTrendResponse } from "../../types";

vi.mock("../../api");

const emptyResponse: CookstyleRecomputeTrendResponse = {
  recompute_available_from: null,
  data: [],
};

const trendResponse: CookstyleRecomputeTrendResponse = {
  recompute_available_from: "2026-06-01T12:00:00Z",
  data: [
    {
      target_chef_version: "19.3.15",
      source: "server",
      completed_at: "2026-06-01T12:00:00Z",
      total_results: 10,
      ready: 6,
      needs_review: 3,
      blocked: 1,
      untested: 0,
      total_complexity: 42,
    },
    {
      target_chef_version: "19.3.15",
      source: "git",
      completed_at: "2026-06-01T12:00:00Z",
      total_results: 4,
      ready: 2,
      needs_review: 1,
      blocked: 1,
      untested: 0,
      total_complexity: 18,
    },
  ],
};

describe("CookstyleRecomputeTrendCard", () => {
  beforeEach(() => {
    vi.mocked(api.fetchCookstyleRecomputeTrend).mockResolvedValue(emptyResponse);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the recomputed-trend title", async () => {
    vi.mocked(api.fetchCookstyleRecomputeTrend).mockResolvedValue(trendResponse);
    render(<CookstyleRecomputeTrendCard />);
    await waitFor(() =>
      expect(
        screen.getByText("CookStyle Rollup — Recomputed Trend"),
      ).toBeInTheDocument(),
    );
  });

  it("renders separate server-cookbook and git-repo charts", async () => {
    vi.mocked(api.fetchCookstyleRecomputeTrend).mockResolvedValue(trendResponse);
    render(<CookstyleRecomputeTrendCard />);
    await waitFor(() => {
      expect(screen.getByTestId("recompute-server-chart")).toBeInTheDocument();
      expect(screen.getByTestId("recompute-git-chart")).toBeInTheDocument();
    });
    expect(screen.getByText("Server cookbooks")).toBeInTheDocument();
    expect(screen.getByText("Git repos")).toBeInTheDocument();
  });

  it("surfaces the frozen/recomputable boundary when history exists", async () => {
    vi.mocked(api.fetchCookstyleRecomputeTrend).mockResolvedValue(trendResponse);
    render(<CookstyleRecomputeTrendCard />);
    await waitFor(() => {
      const note = screen.getByTestId("recompute-boundary-note");
      expect(note.textContent).toMatch(/frozen and cannot be recomputed/);
    });
  });

  it("notes the absence of fingerprint history when none exists", async () => {
    vi.mocked(api.fetchCookstyleRecomputeTrend).mockResolvedValue(emptyResponse);
    render(<CookstyleRecomputeTrendCard />);
    await waitFor(() => {
      const note = screen.getByTestId("recompute-boundary-note");
      expect(note.textContent).toMatch(/No fingerprint history yet/);
    });
  });

  it("renders error state on API failure", async () => {
    vi.mocked(api.fetchCookstyleRecomputeTrend).mockRejectedValue(
      new Error("network error"),
    );
    render(<CookstyleRecomputeTrendCard />);
    await waitFor(() =>
      expect(screen.getByText("network error")).toBeInTheDocument(),
    );
  });
});
