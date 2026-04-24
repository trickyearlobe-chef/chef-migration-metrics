// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { GitKitchenResultsPage } from "./GitKitchenResultsPage";
import type { GitKitchenResult } from "../types";

vi.mock("../api");

const mockResults: GitKitchenResult[] = [
  {
    id: "r1",
    git_repo_name: "apache2",
    git_repo_url: "https://example.com/apache2",
    target_chef_version: "18.4.2",
    commit_sha: "abcdef1234567890",
    platform_name: "ubuntu-22.04",
    suite_name: "default",
    converge_passed: true,
    tests_passed: true,
    timed_out: false,
    duration_seconds: 120,
    completed_at: "2025-01-15T10:30:00Z",
    created_at: "2025-01-15T10:00:00Z",
  },
  {
    id: "r2",
    git_repo_name: "apache2",
    git_repo_url: "https://example.com/apache2",
    target_chef_version: "18.4.2",
    commit_sha: "bbbbbbbb22222222",
    platform_name: "centos-9",
    suite_name: "default",
    converge_passed: true,
    tests_passed: false,
    timed_out: false,
    duration_seconds: 95,
    completed_at: "2025-01-15T10:35:00Z",
    created_at: "2025-01-15T10:00:00Z",
  },
  {
    id: "r3",
    git_repo_name: "nginx",
    git_repo_url: "https://example.com/nginx",
    target_chef_version: "18.4.2",
    commit_sha: "cccccccc33333333",
    platform_name: "ubuntu-22.04",
    suite_name: "default",
    converge_passed: null,
    tests_passed: null,
    timed_out: false,
    created_at: "2025-01-15T11:00:00Z",
  },
  {
    id: "r4",
    git_repo_name: "nginx",
    git_repo_url: "https://example.com/nginx",
    target_chef_version: "18.4.2",
    commit_sha: "dddddddd44444444",
    platform_name: "centos-9",
    suite_name: "default",
    converge_passed: false,
    tests_passed: false,
    timed_out: true,
    duration_seconds: 600,
    completed_at: "2025-01-15T11:15:00Z",
    created_at: "2025-01-15T11:00:00Z",
  },
  {
    id: "r5",
    git_repo_name: "mysql",
    git_repo_url: "https://example.com/mysql",
    target_chef_version: "18.4.2",
    commit_sha: "eeeeeeee55555555",
    platform_name: "ubuntu-22.04",
    suite_name: "default",
    converge_passed: false,
    tests_passed: false,
    timed_out: false,
    error_message: "VM failed to start",
    created_at: "2025-01-15T12:00:00Z",
  },
];

describe("GitKitchenResultsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listGitKitchenResults).mockResolvedValue(mockResults);
  });

  it("renders heading and description after load", async () => {
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });
    expect(
      screen.getByText("Per-instance Test Kitchen results across all batches."),
    ).toBeInTheDocument();
  });

  it("shows loading spinner initially", () => {
    vi.mocked(api.listGitKitchenResults).mockReturnValue(new Promise(() => {}));
    render(<GitKitchenResultsPage />);
    expect(
      screen.getByText("Loading git kitchen results…"),
    ).toBeInTheDocument();
  });

  it("shows error alert on API failure", async () => {
    vi.mocked(api.listGitKitchenResults).mockRejectedValue(
      new Error("Network error"),
    );
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByText("Network error")).toBeInTheDocument();
    expect(screen.getByText("Retry")).toBeInTheDocument();
  });

  it("retries loading when retry button is clicked", async () => {
    const user = userEvent.setup();
    vi.mocked(api.listGitKitchenResults)
      .mockRejectedValueOnce(new Error("Network error"))
      .mockResolvedValueOnce(mockResults);
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Retry"));

    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });
    expect(api.listGitKitchenResults).toHaveBeenCalledTimes(2);
  });

  it("renders summary cards with correct counts", async () => {
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    // Summary cards are uppercase labels in the grid
    // Total: 5, Passed: 1, Failed: 1, Pending: 1, Timed Out: 1, Errored: 1
    expect(screen.getByText("Total")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument(); // Total count
    // Each non-total status has count 1
    const ones = screen.getAllByText("1");
    expect(ones.length).toBeGreaterThanOrEqual(5);
  });

  it("table view renders results with correct columns", async () => {
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    // Check column headers within the table
    const table = screen.getByRole("table");
    expect(within(table).getByText("Cookbook")).toBeInTheDocument();
    expect(within(table).getByText("Platform")).toBeInTheDocument();
    expect(within(table).getByText("Suite")).toBeInTheDocument();
    expect(within(table).getByText("Chef Version")).toBeInTheDocument();
    expect(within(table).getByText("Commit")).toBeInTheDocument();
    expect(within(table).getByText("Status")).toBeInTheDocument();
    expect(within(table).getByText("Duration")).toBeInTheDocument();
    expect(within(table).getByText("Completed At")).toBeInTheDocument();

    // Check some data rows
    expect(within(table).getAllByText("apache2")).toHaveLength(2);
    expect(within(table).getAllByText("ubuntu-22.04")).toHaveLength(3);
    expect(within(table).getByText("mysql")).toBeInTheDocument();
  });

  it("commit SHA is truncated to 8 chars", async () => {
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    expect(screen.getByText("abcdef12")).toBeInTheDocument();
    expect(screen.queryByText("abcdef1234567890")).not.toBeInTheDocument();
  });

  it("filters by cookbook name", async () => {
    const user = userEvent.setup();
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    const cookbookInput = screen.getByPlaceholderText(
      "Filter by cookbook name",
    );
    await user.type(cookbookInput, "nginx");

    // Only nginx rows should remain
    expect(screen.getAllByText("nginx")).toHaveLength(2);
    expect(screen.queryByText("apache2")).not.toBeInTheDocument();
    expect(screen.queryByText("mysql")).not.toBeInTheDocument();
  });

  it("filters by status dropdown", async () => {
    const user = userEvent.setup();
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    const statusSelect = screen.getByDisplayValue("All");
    await user.selectOptions(statusSelect, "passed");

    // Only the passed result (apache2 on ubuntu) should remain
    const table = screen.getByRole("table");
    const rows = within(table).getAllByRole("row");
    // 1 header row + 1 data row
    expect(rows).toHaveLength(2);
    expect(within(table).getByText("apache2")).toBeInTheDocument();
  });

  it("shows 'No results' message when filter yields empty", async () => {
    const user = userEvent.setup();
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    const cookbookInput = screen.getByPlaceholderText(
      "Filter by cookbook name",
    );
    await user.type(cookbookInput, "nonexistent-cookbook");

    expect(
      screen.getByText("No results match the current filters."),
    ).toBeInTheDocument();
  });

  it("toggles to matrix view", async () => {
    const user = userEvent.setup();
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Platform Matrix"));

    // Matrix view should show cookbook names as row headers
    const table = screen.getByRole("table");
    expect(within(table).getByText("apache2")).toBeInTheDocument();
    expect(within(table).getByText("nginx")).toBeInTheDocument();
    expect(within(table).getByText("mysql")).toBeInTheDocument();

    // Column headers should include platforms
    expect(within(table).getByText("centos-9")).toBeInTheDocument();
    expect(within(table).getByText("ubuntu-22.04")).toBeInTheDocument();
  });

  it("platform matrix shows correct status symbols", async () => {
    const user = userEvent.setup();
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Platform Matrix"));

    const table = screen.getByRole("table");

    // apache2/ubuntu-22.04 = passed → ✓
    // apache2/centos-9 = failed → ✗
    // nginx/ubuntu-22.04 = pending → …
    // nginx/centos-9 = timed out → ⏱
    // mysql/ubuntu-22.04 = errored → !
    // mysql/centos-9 = untested → —
    expect(within(table).getByText("✓")).toBeInTheDocument();
    expect(within(table).getByText("✗")).toBeInTheDocument();
    expect(within(table).getByText("…")).toBeInTheDocument();
    expect(within(table).getByText("⏱")).toBeInTheDocument();
    expect(within(table).getByText("!")).toBeInTheDocument();
    expect(within(table).getByText("—")).toBeInTheDocument();
  });

  it("filters by platform name", async () => {
    const user = userEvent.setup();
    render(<GitKitchenResultsPage />);
    await waitFor(() => {
      expect(screen.getByText("Git Kitchen Results")).toBeInTheDocument();
    });

    const platformInput = screen.getByPlaceholderText("Filter by platform");
    await user.type(platformInput, "centos");

    const table = screen.getByRole("table");
    const rows = within(table).getAllByRole("row");
    // 1 header + 2 data rows (apache2/centos-9 and nginx/centos-9)
    expect(rows).toHaveLength(3);
    expect(screen.queryByText("mysql")).not.toBeInTheDocument();
  });
});
