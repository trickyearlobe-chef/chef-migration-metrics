// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import KitchenQueuePage from "./KitchenQueuePage";
import * as api from "../api";
import type { KitchenQueueItem } from "../types";

vi.mock("../api");
vi.mock("../hooks/useWebSocket", () => ({
  useWebSocket: () => ({ onEvent: () => () => {} }),
}));

// List responses omit `output` (the backend list query does not select it),
// so the queue page must lazy-fetch the per-item detail to show output.
const completedItem: KitchenQueueItem = {
  id: "item-1",
  run_type: "git",
  git_repo_name: "example-cookbook",
  suite_name: "default",
  platform_name: "almalinux-9",
  instance_name: "default-almalinux-9",
  target_chef_version: "18.4.2",
  priority: 0,
  status: "completed",
  enqueued_at: "2026-06-08T10:00:00Z",
  started_at: "2026-06-08T10:00:05Z",
  completed_at: "2026-06-08T10:05:00Z",
  // No output here — exactly as the list endpoint returns it.
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(api.fetchKitchenQueue).mockResolvedValue({
    items: [completedItem],
    stats: { queued: 0, running: 0, workers_active: 0 },
  });
});

describe("KitchenQueuePage — completed-job output", () => {
  it("lazy-fetches the item detail on expand and shows its output", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchKitchenQueueItem).mockResolvedValue({
      ...completedItem,
      output: "Recipe: example::default\n       Converge complete.",
    });

    render(<KitchenQueuePage />);

    const repoCell = await screen.findByText("example-cookbook");
    await user.click(repoCell);

    await waitFor(() => {
      expect(api.fetchKitchenQueueItem).toHaveBeenCalledWith("item-1");
    });
    expect(
      await screen.findByText(/Converge complete\./),
    ).toBeInTheDocument();
  });

  it("shows 'No output available' when the detail has no output", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchKitchenQueueItem).mockResolvedValue({
      ...completedItem,
      output: "",
    });

    render(<KitchenQueuePage />);

    await user.click(await screen.findByText("example-cookbook"));

    await waitFor(() => {
      expect(api.fetchKitchenQueueItem).toHaveBeenCalledWith("item-1");
    });
    expect(
      await screen.findByText(/No output available/),
    ).toBeInTheDocument();
  });
});
