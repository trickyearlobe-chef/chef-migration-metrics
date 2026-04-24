// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { NodeDetailPage } from "./NodeDetailPage";
import type { NodeDetailResponse, NodeKitchenRun } from "../types";

vi.mock("../api");
vi.mock("react-router-dom", () => ({
  useParams: () => ({ org: "test-org", name: "test-node" }),
  Link: ({
    children,
    to,
    ...rest
  }: {
    children: React.ReactNode;
    to: string;
    [k: string]: unknown;
  }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}));

const baseNode: NodeDetailResponse = {
  organisation_name: "test-org",
  node: {
    id: "n1",
    collection_run_id: "cr1",
    organisation_id: "org1",
    node_name: "test-node",
    chef_environment: "production",
    chef_version: "17.10.0",
    platform: "ubuntu",
    platform_version: "22.04",
    platform_family: "debian",
    filesystem: null,
    cookbooks: null,
    run_list: null,
    roles: null,
    policy_name: "",
    policy_group: "",
    ohai_time: 1700000000,
    is_stale: false,
    collected_at: "2025-01-01T00:00:00Z",
    created_at: "2025-01-01T00:00:00Z",
  },
  readiness: null,
};

function makeRun(overrides: Partial<NodeKitchenRun> = {}): NodeKitchenRun {
  return {
    id: "kr1",
    node_name: "test-node",
    organisation_name: "test-org",
    target_chef_version: "18.5.0",
    cookbook_source: "server",
    platform_name: "ubuntu-22.04",
    run_list: ["recipe[apache2]"],
    cookbook_versions: { apache2: "5.0.0" },
    converge_passed: true,
    verify_passed: true,
    converge_output: "converge ok",
    verify_output: "verify ok",
    duration_seconds: 120,
    started_at: "2025-01-15T10:00:00Z",
    completed_at: "2025-01-15T10:02:00Z",
    created_at: "2025-01-15T10:00:00Z",
    ...overrides,
  };
}

/** Return the kitchen section card container. */
function getKitchenSection(): HTMLElement {
  const heading = screen.getByText("Node Kitchen Testing");
  return heading.closest(".card")!;
}

describe("NodeKitchenSection", () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(baseNode);
    vi.mocked(api.fetchFilterTargetChefVersions).mockResolvedValue({
      data: ["18.5.0", "19.1.0"],
    });
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([]);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("shows loading text then 'No kitchen runs yet.' when empty", async () => {
    vi.mocked(api.fetchNodeKitchenRuns).mockImplementation(
      () => new Promise((resolve) => setTimeout(() => resolve([]), 50)),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Loading runs…")).toBeInTheDocument(),
    );
    await vi.advanceTimersByTimeAsync(100);
    await waitFor(() =>
      expect(screen.getByText("No kitchen runs yet.")).toBeInTheDocument(),
    );
  });

  it("renders run table with correct column headers", async () => {
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([makeRun()]);
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Node Kitchen Testing")).toBeInTheDocument(),
    );
    const section = getKitchenSection();
    for (const col of [
      "Target",
      "Source",
      "Platform",
      "Converge",
      "Verify",
      "Duration",
      "Started",
      "Error",
    ]) {
      expect(
        within(section).getByRole("columnheader", { name: col }),
      ).toBeInTheDocument();
    }
  });

  it("shows ✅ for passed converge", async () => {
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([
      makeRun({ converge_passed: true, verify_passed: false }),
    ]);
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Node Kitchen Testing")).toBeInTheDocument(),
    );
    const section = getKitchenSection();
    await waitFor(() => {
      const cells = within(section).getAllByRole("cell");
      // Converge is the 4th column (index 3)
      expect(cells[3].textContent).toBe("✅");
    });
  });

  it("shows ❌ for failed converge", async () => {
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([
      makeRun({ converge_passed: false, verify_passed: false }),
    ]);
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Node Kitchen Testing")).toBeInTheDocument(),
    );
    const section = getKitchenSection();
    await waitFor(() => {
      const cells = within(section).getAllByRole("cell");
      expect(cells[3].textContent).toBe("❌");
    });
  });

  it("shows ⏳ for pending converge (null)", async () => {
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([
      makeRun({
        converge_passed: null,
        verify_passed: null,
        completed_at: undefined,
      }),
    ]);
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Node Kitchen Testing")).toBeInTheDocument(),
    );
    const section = getKitchenSection();
    await waitFor(() => {
      const cells = within(section).getAllByRole("cell");
      expect(cells[3].textContent).toBe("⏳");
    });
  });

  it("toggles 'New Test Run…' to show the form", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("▸ New Test Run…")).toBeInTheDocument(),
    );
    expect(screen.queryByText("Cookbook Source")).not.toBeInTheDocument();

    await user.click(screen.getByText("▸ New Test Run…"));
    expect(screen.getByText("Cookbook Source")).toBeInTheDocument();
    expect(screen.getByText("Target Chef Version")).toBeInTheDocument();
    expect(screen.getByText("Run Test")).toBeInTheDocument();
  });

  it("form shows target version select and cookbook source radios", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<NodeDetailPage />);
    await waitFor(() => screen.getByText("▸ New Test Run…"));
    await user.click(screen.getByText("▸ New Test Run…"));

    const select = screen.getByRole("combobox");
    expect(select).toBeInTheDocument();
    const options = screen.getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["18.5.0", "19.1.0"]);

    const radios = screen.getAllByRole("radio");
    expect(radios).toHaveLength(3);
    expect(screen.getByLabelText("server")).toBeChecked();
  });

  it("clicking 'Run Test' calls triggerNodeKitchenRun", async () => {
    vi.mocked(api.triggerNodeKitchenRun).mockResolvedValue({
      status: "ok",
      message: "run queued",
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<NodeDetailPage />);
    await waitFor(() => screen.getByText("▸ New Test Run…"));
    await user.click(screen.getByText("▸ New Test Run…"));
    await user.click(screen.getByText("Run Test"));

    await waitFor(() =>
      expect(api.triggerNodeKitchenRun).toHaveBeenCalledWith({
        node_name: "test-node",
        organisation_name: "test-org",
        target_chef_version: "18.5.0",
        cookbook_source: "server",
      }),
    );
    await waitFor(() =>
      expect(screen.getByText("Started: run queued")).toBeInTheDocument(),
    );
  });

  it("formats duration as seconds when < 60", async () => {
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([
      makeRun({ duration_seconds: 45 }),
    ]);
    render(<NodeDetailPage />);
    const section = await waitFor(() => getKitchenSection());
    await waitFor(() =>
      expect(within(section).getByText("45s")).toBeInTheDocument(),
    );
  });

  it("formats duration as minutes and seconds when >= 60", async () => {
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([
      makeRun({ duration_seconds: 125 }),
    ]);
    render(<NodeDetailPage />);
    const section = await waitFor(() => getKitchenSection());
    await waitFor(() =>
      expect(within(section).getByText("2m 5s")).toBeInTheDocument(),
    );
  });

  it("clicking a row toggles expanded view with converge/verify output", async () => {
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([
      makeRun({
        converge_output: "CONVERGE_LOG_DATA",
        verify_output: "VERIFY_LOG_DATA",
      }),
    ]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<NodeDetailPage />);
    const section = await waitFor(() => getKitchenSection());
    await waitFor(() =>
      expect(within(section).getByText("18.5.0")).toBeInTheDocument(),
    );
    expect(screen.queryByText("CONVERGE_LOG_DATA")).not.toBeInTheDocument();

    await user.click(within(section).getByText("18.5.0"));
    expect(screen.getByText("CONVERGE_LOG_DATA")).toBeInTheDocument();
    expect(screen.getByText("VERIFY_LOG_DATA")).toBeInTheDocument();
    expect(screen.getByText("Converge Output")).toBeInTheDocument();
    expect(screen.getByText("Verify Output")).toBeInTheDocument();

    await user.click(within(section).getByText("18.5.0"));
    expect(screen.queryByText("CONVERGE_LOG_DATA")).not.toBeInTheDocument();
  });

  it("shows truncated error message and expands on click", async () => {
    const longError =
      "This is a very long error message that exceeds forty characters for sure";
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([
      makeRun({ error_message: longError }),
    ]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<NodeDetailPage />);
    const section = await waitFor(() => getKitchenSection());
    const truncated = longError.slice(0, 40) + "…";
    await waitFor(() =>
      expect(within(section).getByText(truncated)).toBeInTheDocument(),
    );

    await user.click(within(section).getByText(truncated));
    expect(within(section).getByText(longError)).toBeInTheDocument();
  });
});
