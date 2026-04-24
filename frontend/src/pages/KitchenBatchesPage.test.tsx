// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import KitchenBatchesPage from "./KitchenBatchesPage";
import type {
  KitchenBatch,
  KitchenBatchDetail,
  GitKitchenResult,
  BatchProgress,
  GitRepoListItem,
} from "../types";

vi.mock("../api");

const mockDraftBatch: KitchenBatch = {
  id: "batch-1",
  name: "Draft Batch",
  filters: { cookbook_names: ["nginx", "apache"] },
  max_count: 10,
  max_concurrent_vms: 3,
  dry_run: false,
  status: "draft",
  created_by: "admin",
  created_at: "2025-01-01T00:00:00Z",
};

const mockRunningBatch: KitchenBatch = {
  id: "batch-2",
  name: "Running Batch",
  filters: { platforms: ["ubuntu-22.04"] },
  max_count: null,
  max_concurrent_vms: 5,
  dry_run: true,
  status: "running",
  created_by: "tester",
  created_at: "2025-01-02T00:00:00Z",
  started_at: "2025-01-02T00:05:00Z",
};

const mockCompletedBatch: KitchenBatch = {
  id: "batch-3",
  name: "Completed Batch",
  filters: { has_test_suite: true, previous_status: "failed" },
  max_count: 5,
  max_concurrent_vms: null,
  dry_run: false,
  status: "completed",
  created_by: "admin",
  created_at: "2025-01-03T00:00:00Z",
  started_at: "2025-01-03T00:05:00Z",
  completed_at: "2025-01-03T01:00:00Z",
};

const mockBatches: KitchenBatch[] = [
  mockDraftBatch,
  mockRunningBatch,
  mockCompletedBatch,
];

const mockDraftDetail: KitchenBatchDetail = {
  ...mockDraftBatch,
  estimate: {
    total_cookbooks: 2,
    total_estimated_vms: 6,
    per_platform: { "ubuntu-22.04": 3, "centos-8": 3 },
    cookbooks: [
      {
        name: "nginx",
        git_repo_url: "https://example.com/nginx.git",
        estimated_vms: 3,
      },
      {
        name: "apache",
        git_repo_url: "https://example.com/apache.git",
        estimated_vms: 3,
      },
    ],
  },
};

const mockRunningDetail: KitchenBatchDetail = {
  ...mockRunningBatch,
  estimate: {
    total_cookbooks: 1,
    total_estimated_vms: 4,
    per_platform: { "ubuntu-22.04": 4 },
    cookbooks: [
      {
        name: "myapp",
        git_repo_url: "https://example.com/myapp.git",
        estimated_vms: 4,
      },
    ],
  },
};

const mockCompletedDetail: KitchenBatchDetail = {
  ...mockCompletedBatch,
  estimate: {
    total_cookbooks: 2,
    total_estimated_vms: 6,
    per_platform: { "ubuntu-22.04": 3, "centos-8": 3 },
    cookbooks: [
      {
        name: "nginx",
        git_repo_url: "https://example.com/nginx.git",
        estimated_vms: 3,
      },
      {
        name: "apache",
        git_repo_url: "https://example.com/apache.git",
        estimated_vms: 3,
      },
    ],
  },
};

const mockProgress: BatchProgress = {
  passed: 3,
  failed: 1,
  pending: 2,
  timed_out: 1,
  errored: 0,
  total: 7,
};

const mockResults: GitKitchenResult[] = [
  {
    id: "r1",
    batch_id: "batch-2",
    git_repo_name: "myapp",
    git_repo_url: "https://example.com/myapp.git",
    target_chef_version: "18.4.2",
    commit_sha: "abcdef1234567890",
    platform_name: "ubuntu-22.04",
    suite_name: "default",
    converge_passed: true,
    tests_passed: true,
    timed_out: false,
    duration_seconds: 120,
    started_at: "2025-01-02T00:06:00Z",
    completed_at: "2025-01-02T00:08:00Z",
    created_at: "2025-01-02T00:05:00Z",
  },
  {
    id: "r2",
    batch_id: "batch-2",
    git_repo_name: "myapp",
    git_repo_url: "https://example.com/myapp.git",
    target_chef_version: "18.4.2",
    commit_sha: "bbbbbbbb22222222",
    platform_name: "centos-9",
    suite_name: "default",
    converge_passed: true,
    tests_passed: false,
    timed_out: false,
    duration_seconds: 95,
    started_at: "2025-01-02T00:06:00Z",
    completed_at: "2025-01-02T00:07:35Z",
    created_at: "2025-01-02T00:05:00Z",
  },
];

const mockExcludedRepos: GitRepoListItem[] = [
  {
    id: "repo-1",
    name: "legacy-cookbook",
    git_repo_url: "https://example.com/legacy-cookbook.git",
    has_test_suite: false,
    clone_status: "cloned",
    tk_status: "excluded",
  },
];

function setupDefaultMocks() {
  vi.mocked(api.listKitchenBatches).mockResolvedValue(mockBatches);
  vi.mocked(api.listExcludedGitRepos).mockResolvedValue(mockExcludedRepos);
  vi.mocked(api.fetchBatchResults).mockResolvedValue(mockResults);
  vi.mocked(api.fetchBatchProgress).mockResolvedValue(mockProgress);
  vi.mocked(api.createKitchenBatch).mockResolvedValue(mockDraftBatch);
  vi.mocked(api.getKitchenBatch).mockResolvedValue(mockDraftDetail);
  vi.mocked(api.runKitchenBatch).mockResolvedValue(mockRunningDetail);
  vi.mocked(api.cancelKitchenBatch).mockResolvedValue(mockCompletedBatch);
  vi.mocked(api.deleteKitchenBatch).mockResolvedValue(undefined);
  vi.mocked(api.excludeGitRepo).mockResolvedValue(undefined);
  vi.mocked(api.clearGitRepoExclusion).mockResolvedValue(undefined);
}

describe("KitchenBatchesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers({ shouldAdvanceTime: true });
    setupDefaultMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  // 1. Shows loading spinner on initial load
  it("shows loading spinner on initial load", () => {
    vi.mocked(api.listKitchenBatches).mockReturnValue(new Promise(() => {}));
    render(<KitchenBatchesPage />);
    expect(screen.getByText("Loading kitchen batches…")).toBeInTheDocument();
  });

  // 2. List view renders batch table after loading
  it("renders batch table after loading", async () => {
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Draft Batch")).toBeInTheDocument();
    });

    const table = screen.getByRole("table");
    expect(within(table).getByText("Draft Batch")).toBeInTheDocument();
    expect(within(table).getByText("Running Batch")).toBeInTheDocument();
    expect(within(table).getByText("Completed Batch")).toBeInTheDocument();

    // Check column headers
    expect(within(table).getByText("Name")).toBeInTheDocument();
    expect(within(table).getByText("Status")).toBeInTheDocument();
    expect(within(table).getByText("Dry Run")).toBeInTheDocument();
    expect(within(table).getByText("Max Count")).toBeInTheDocument();
    expect(within(table).getByText("Created")).toBeInTheDocument();
    expect(within(table).getByText("Actions")).toBeInTheDocument();

    // Check status badges
    expect(within(table).getByText("draft")).toBeInTheDocument();
    expect(within(table).getByText("running")).toBeInTheDocument();
    expect(within(table).getByText("completed")).toBeInTheDocument();
  });

  // 3. Shows "No batches yet" when list is empty
  it("shows 'No batches yet' when list is empty", async () => {
    vi.mocked(api.listKitchenBatches).mockResolvedValue([]);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(
        screen.getByText("No batches yet. Create one to get started."),
      ).toBeInTheDocument();
    });
  });

  // 4. Shows error alert on API failure
  it("shows error alert on API failure", async () => {
    vi.mocked(api.listKitchenBatches).mockRejectedValue(
      new Error("Network error"),
    );
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByText("Network error")).toBeInTheDocument();
  });

  // 5. Clicking "+ New Batch" shows the create form
  it("clicking '+ New Batch' shows the create form", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("+ New Batch")).toBeInTheDocument();
    });

    await user.click(screen.getByText("+ New Batch"));

    expect(screen.getByText("New Batch")).toBeInTheDocument();
    expect(screen.getByText("Batch Name")).toBeInTheDocument();
    expect(screen.getByText("Cookbook Names")).toBeInTheDocument();
    expect(screen.getByText("Exclude Cookbooks")).toBeInTheDocument();
    expect(screen.getByText("Has Test Suite")).toBeInTheDocument();
    expect(screen.getByText("Previous Status")).toBeInTheDocument();
    expect(screen.getByText("Max Count")).toBeInTheDocument();
    expect(screen.getByText("Max Concurrent VMs")).toBeInTheDocument();
    expect(screen.getByText("Dry Run")).toBeInTheDocument();
    expect(screen.getByText("Save")).toBeInTheDocument();
    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  // 6. Detail view shows batch name and status badge
  it("detail view shows batch name and status badge", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Draft Batch")).toBeInTheDocument();
    });

    // Click "View" on the first batch
    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[0]);

    await waitFor(() => {
      // The detail heading uses h3
      expect(
        screen.getByRole("heading", { level: 3, name: "Draft Batch" }),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("draft")).toBeInTheDocument();
    expect(screen.getByText("← Back")).toBeInTheDocument();
  });

  // 7. Detail view shows filter summary
  it("detail view shows filter summary", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Draft Batch")).toBeInTheDocument();
    });

    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[0]);

    await waitFor(() => {
      expect(screen.getByText("Filters")).toBeInTheDocument();
    });
    expect(screen.getByText("Cookbook Names")).toBeInTheDocument();
    expect(screen.getByText("nginx, apache")).toBeInTheDocument();
    // max_count = 10 for draft batch
    expect(screen.getByText("10")).toBeInTheDocument();
  });

  // 8. Detail view shows Run button for draft batches
  it("detail view shows Run button for draft batches", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Draft Batch")).toBeInTheDocument();
    });

    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[0]);

    await waitFor(() => {
      expect(screen.getByText("Run Batch")).toBeInTheDocument();
    });
    expect(screen.getByText("Delete")).toBeInTheDocument();
  });

  // 9. Detail view shows Cancel button for running batches
  it("detail view shows Cancel button for running batches", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockRunningDetail);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Running Batch")).toBeInTheDocument();
    });

    // Click "View" on the running batch (second row)
    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[1]);

    await waitFor(() => {
      expect(screen.getByText("Cancel Batch")).toBeInTheDocument();
    });
  });

  // 10. Detail view shows Delete button for completed batches
  it("detail view shows Delete button for completed batches", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockCompletedDetail);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Completed Batch")).toBeInTheDocument();
    });

    // Click "View" on the completed batch (third row)
    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[2]);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { level: 3, name: "Completed Batch" }),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("Delete")).toBeInTheDocument();
    // Running and previewing are the only statuses that show Cancel in detail
    expect(screen.queryByText("Cancel Batch")).not.toBeInTheDocument();
    expect(screen.queryByText("Run Batch")).not.toBeInTheDocument();
  });

  // 11. Detail view with running batch shows progress bar
  it("detail view with running batch shows progress bar", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockRunningDetail);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Running Batch")).toBeInTheDocument();
    });

    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[1]);

    await waitFor(() => {
      expect(screen.getByText("Cancel Batch")).toBeInTheDocument();
    });

    // Progress bar text items
    await waitFor(() => {
      expect(screen.getByText(/3 passed/)).toBeInTheDocument();
    });
    expect(screen.getByText(/1 failed/)).toBeInTheDocument();
    expect(screen.getByText(/2 pending/)).toBeInTheDocument();
    expect(screen.getByText(/1 timed out/)).toBeInTheDocument();
    expect(screen.getByText(/Total: 7/)).toBeInTheDocument();
  });

  // 12. Results tab renders results table
  it("results tab renders results table", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockRunningDetail);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Running Batch")).toBeInTheDocument();
    });

    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[1]);

    // Wait for the tabs to appear
    await waitFor(() => {
      expect(screen.getByText("Overview")).toBeInTheDocument();
    });

    // Wait for results to load so the tab shows the count
    await waitFor(() => {
      expect(screen.getByText(/Results \(2\)/)).toBeInTheDocument();
    });

    // Click the Results tab
    await user.click(screen.getByText(/Results \(2\)/));

    // The results table should show result rows
    const table = screen.getByRole("table");
    expect(within(table).getByText("Cookbook")).toBeInTheDocument();
    expect(within(table).getByText("Platform")).toBeInTheDocument();
    expect(within(table).getByText("Suite")).toBeInTheDocument();
    expect(within(table).getByText("Chef Version")).toBeInTheDocument();
    expect(within(table).getAllByText("myapp")).toHaveLength(2);
    expect(within(table).getByText("ubuntu-22.04")).toBeInTheDocument();
    expect(within(table).getByText("centos-9")).toBeInTheDocument();
    expect(within(table).getByText("120s")).toBeInTheDocument();
    expect(within(table).getByText("95s")).toBeInTheDocument();
  });

  // 13. ExcludedCookbooksSection renders when expanded
  it("ExcludedCookbooksSection renders when expanded", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Kitchen Batches")).toBeInTheDocument();
    });

    // The section header should be visible
    expect(screen.getByText("Excluded Cookbooks")).toBeInTheDocument();

    // Click to expand
    await user.click(screen.getByText("Excluded Cookbooks"));

    // Wait for the excluded repos to load and render
    await waitFor(() => {
      expect(screen.getByText("legacy-cookbook")).toBeInTheDocument();
    });

    // The "Clear" button and "+ Exclude Cookbook" button should be visible
    expect(screen.getByText("Clear")).toBeInTheDocument();
    expect(screen.getByText("+ Exclude Cookbook")).toBeInTheDocument();
    expect(api.listExcludedGitRepos).toHaveBeenCalled();
  });
});
