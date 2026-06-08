// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, within, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import KitchenBatchesPage from "./KitchenBatchesPage";
import type {
  KitchenBatch,
  KitchenBatchDetail,
  BatchProgress,
  GitRepoListItem,
  KitchenBatchInstance,
} from "../types";

vi.mock("../api");

// WebSocket mock: capture onEvent listeners so tests can fire live events.
const { wsListeners, emitWsEvent } = vi.hoisted(() => {
  const listeners: Record<string, Set<(data: unknown) => void>> = {};
  return {
    wsListeners: listeners,
    emitWsEvent: (event: string, data: unknown) => {
      listeners[event]?.forEach((cb) => cb(data));
    },
  };
});

vi.mock("../hooks/useWebSocket", () => ({
  useWebSocket: () => ({
    status: "connected",
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
    onEvent: (event: string, cb: (data: unknown) => void) => {
      if (!wsListeners[event]) wsListeners[event] = new Set();
      wsListeners[event].add(cb);
      return () => wsListeners[event].delete(cb);
    },
    disconnect: vi.fn(),
    reconnect: vi.fn(),
  }),
}));

const mockDraftBatch: KitchenBatch = {
  id: "batch-1",
  name: "Draft Batch",
  filters: { cookbook_names: ["nginx", "apache"] },
  max_count: 10,
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

const mockInstances: KitchenBatchInstance[] = [
  {
    id: "inst-1",
    batch_id: "batch-2",
    git_repo_name: "nginx",
    git_repo_url: "https://example.com/nginx.git",
    instance_name: "default-ubuntu-2204",
    platform_name: "ubuntu-22.04",
    suite_name: "default",
    target_chef_version: "18",
    status: "passed",
    created_at: "2025-01-02T00:05:00Z",
  },
  {
    id: "inst-2",
    batch_id: "batch-2",
    git_repo_name: "nginx",
    git_repo_url: "https://example.com/nginx.git",
    instance_name: "default-centos-8",
    platform_name: "centos-8",
    suite_name: "default",
    target_chef_version: "18",
    status: "failed",
    error_message: "converge failed",
    created_at: "2025-01-02T00:05:00Z",
  },
  {
    id: "inst-3",
    batch_id: "batch-2",
    git_repo_name: "apache",
    git_repo_url: "https://example.com/apache.git",
    instance_name: "default-ubuntu-2204",
    platform_name: "ubuntu-22.04",
    suite_name: "default",
    target_chef_version: "18",
    status: "running",
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
  vi.mocked(api.fetchBatchProgress).mockResolvedValue(mockProgress);
  vi.mocked(api.fetchBatchInstances).mockResolvedValue([]);
  vi.mocked(api.createKitchenBatch).mockResolvedValue(mockDraftBatch);
  vi.mocked(api.getKitchenBatch).mockResolvedValue(mockDraftDetail);
  vi.mocked(api.runKitchenBatch).mockResolvedValue(mockRunningDetail);
  vi.mocked(api.cancelKitchenBatch).mockResolvedValue(mockCompletedBatch);
  vi.mocked(api.deleteKitchenBatch).mockResolvedValue(undefined);
  vi.mocked(api.excludeGitRepo).mockResolvedValue(undefined);
  vi.mocked(api.clearGitRepoExclusion).mockResolvedValue(undefined);
  vi.mocked(api.fetchTestKitchenConfig).mockResolvedValue({
    config: {
      enabled: false,
      driver: "",
      timeout_minutes: 30,
      driver_settings: {},
      driver_secrets: {},
      image_field_name: "",
      chef_license_key_credential: "",
      images: [],
      platform_map: [],
    },
    source: "database",
  });
}

describe("KitchenBatchesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    for (const key of Object.keys(wsListeners)) delete wsListeners[key];
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

  // 12a. Save lands on the runnable detail (not the list)
  it("Save lands on the batch detail rather than the list", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("+ New Batch")).toBeInTheDocument();
    });

    await user.click(screen.getByText("+ New Batch"));
    await user.type(
      screen.getByPlaceholderText("e.g. Phase 1 — Linux cookbooks"),
      "My Batch",
    );
    await user.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(api.createKitchenBatch).toHaveBeenCalled();
    });
    // Lands on the detail (draft) — Run Batch action present, not back on list
    await waitFor(() => {
      expect(api.getKitchenBatch).toHaveBeenCalledWith(mockDraftBatch.id);
    });
    expect(
      await screen.findByRole("heading", { level: 3, name: "Draft Batch" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Run Batch")).toBeInTheDocument();
    expect(api.runKitchenBatch).not.toHaveBeenCalled();
  });

  // 12b. Create & Run goes from form straight to a running batch
  it("Create & Run creates then runs the batch in one action", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.fetchTestKitchenConfig).mockResolvedValue({
      enabled: true,
      driver: "proxmox",
      timeout_minutes: 30,
      driver_settings: {},
      driver_secrets: {},
      image_field_name: "",
      chef_license_key_credential: "",
      images: [],
      platform_map: [],
      start_rate_window_minutes: 0,
      start_rate_max_per_window: 0,
    });
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("+ New Batch")).toBeInTheDocument();
    });

    await user.click(screen.getByText("+ New Batch"));
    await user.type(
      screen.getByPlaceholderText("e.g. Phase 1 — Linux cookbooks"),
      "My Batch",
    );
    // Wait for the TK-enabled config to propagate so the button is active
    await waitFor(() => {
      expect(screen.getByText("Create & Run")).toBeEnabled();
    });
    await user.click(screen.getByText("Create & Run"));

    await waitFor(() => {
      expect(api.createKitchenBatch).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(api.runKitchenBatch).toHaveBeenCalledWith(mockDraftBatch.id);
    });
    // Lands on the running detail — Cancel Batch action present
    expect(
      await screen.findByRole("heading", { level: 3, name: "Running Batch" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Cancel Batch")).toBeInTheDocument();
    expect(api.getKitchenBatch).not.toHaveBeenCalled();
  });

  // 12c. Create & Run is disabled until Test Kitchen is enabled
  it("Create & Run is disabled when Test Kitchen is not enabled", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("+ New Batch")).toBeInTheDocument();
    });

    await user.click(screen.getByText("+ New Batch"));
    await user.type(
      screen.getByPlaceholderText("e.g. Phase 1 — Linux cookbooks"),
      "My Batch",
    );
    // Default config mock has enabled: false
    expect(screen.getByText("Create & Run")).toBeDisabled();
    expect(screen.getByText("Save")).toBeEnabled();
  });

  // 12d. Detail view lists per-instance results grouped by cookbook
  it("detail view shows per-instance results grouped by cookbook, expandable", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockRunningDetail);
    vi.mocked(api.fetchBatchInstances).mockResolvedValue(mockInstances);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Running Batch")).toBeInTheDocument();
    });

    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[1]);

    await waitFor(() => {
      expect(screen.getByText("Cancel Batch")).toBeInTheDocument();
    });

    // Group headers (cookbooks) render with instance counts
    expect(await screen.findByText("Instance Results (3)")).toBeInTheDocument();
    expect(screen.getByText("apache")).toBeInTheDocument();
    expect(screen.getByText("nginx")).toBeInTheDocument();

    // Collapsed by default — instance rows not yet shown
    expect(screen.queryByText("default-centos-8")).not.toBeInTheDocument();

    // Expand the nginx group → its instance rows appear
    await user.click(screen.getByText("nginx"));
    expect(await screen.findByText("default-centos-8")).toBeInTheDocument();
    expect(screen.getByText("converge failed")).toBeInTheDocument();
    // Status badges for the nginx instances
    expect(screen.getByText("passed")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();

    expect(api.fetchBatchInstances).toHaveBeenCalledWith("batch-2");
  });

  // 12e. Instance table refreshes on the poll tick while running
  it("refreshes instances on the poll tick while running", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockRunningDetail);
    vi.mocked(api.fetchBatchInstances).mockResolvedValue(mockInstances);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Running Batch")).toBeInTheDocument();
    });

    await user.click(screen.getAllByText("View")[1]);
    await waitFor(() => {
      expect(screen.getByText("Instance Results (3)")).toBeInTheDocument();
    });

    const callsAfterMount = vi.mocked(api.fetchBatchInstances).mock.calls.length;
    // Advance past the 5s poll interval
    await vi.advanceTimersByTimeAsync(5000);
    expect(
      vi.mocked(api.fetchBatchInstances).mock.calls.length,
    ).toBeGreaterThan(callsAfterMount);
  });

  // 12f. A batch_progress event refreshes the detail without the poll firing
  it("refreshes progress on a batch_progress event without waiting for the poll", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockRunningDetail);
    vi.mocked(api.fetchBatchInstances).mockResolvedValue(mockInstances);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Running Batch")).toBeInTheDocument();
    });

    await user.click(screen.getAllByText("View")[1]);
    await waitFor(() => {
      expect(screen.getByText("Cancel Batch")).toBeInTheDocument();
    });

    const progressCalls = vi.mocked(api.fetchBatchProgress).mock.calls.length;
    const instanceCalls = vi.mocked(api.fetchBatchInstances).mock.calls.length;

    // Fire a live event for this batch — no timer advance.
    act(() => {
      emitWsEvent("batch_progress", {
        batch_id: "batch-2",
        instance_name: "default-ubuntu-2204",
        git_repo_name: "nginx",
        passed: true,
      });
    });

    await waitFor(() => {
      expect(
        vi.mocked(api.fetchBatchProgress).mock.calls.length,
      ).toBeGreaterThan(progressCalls);
    });
    expect(
      vi.mocked(api.fetchBatchInstances).mock.calls.length,
    ).toBeGreaterThan(instanceCalls);
  });

  // 12g. A batch_progress event for a different batch is ignored
  it("ignores a batch_progress event for a different batch", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockRunningDetail);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Running Batch")).toBeInTheDocument();
    });

    await user.click(screen.getAllByText("View")[1]);
    await waitFor(() => {
      expect(screen.getByText("Cancel Batch")).toBeInTheDocument();
    });

    const progressCalls = vi.mocked(api.fetchBatchProgress).mock.calls.length;

    act(() => {
      emitWsEvent("batch_progress", { batch_id: "some-other-batch", passed: true });
    });

    // No refresh should be triggered for an unrelated batch.
    await Promise.resolve();
    expect(vi.mocked(api.fetchBatchProgress).mock.calls.length).toBe(progressCalls);
  });

  // 12h. A batch_complete event refetches the batch detail (status flips)
  it("refetches the batch detail on a batch_complete event", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockRunningDetail);
    render(<KitchenBatchesPage />);
    await waitFor(() => {
      expect(screen.getByText("Running Batch")).toBeInTheDocument();
    });

    await user.click(screen.getAllByText("View")[1]);
    await waitFor(() => {
      expect(screen.getByText("Cancel Batch")).toBeInTheDocument();
    });

    // After running, the detail comes back completed.
    vi.mocked(api.getKitchenBatch).mockResolvedValue(mockCompletedDetail);
    const detailCalls = vi.mocked(api.getKitchenBatch).mock.calls.length;

    act(() => {
      emitWsEvent("batch_complete", {
        batch_id: "batch-2",
        status: "completed",
        total: 7,
        passed: 6,
        failed: 1,
        errored: 0,
      });
    });

    await waitFor(() => {
      expect(
        vi.mocked(api.getKitchenBatch).mock.calls.length,
      ).toBeGreaterThan(detailCalls);
    });
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
