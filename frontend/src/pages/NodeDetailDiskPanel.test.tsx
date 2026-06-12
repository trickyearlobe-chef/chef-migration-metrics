// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import * as api from "../api";
import { NodeDetailPage } from "./NodeDetailPage";
import type { NodeDetailResponse } from "../types";

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

// readiness is null throughout — the disk card must still render, because the
// verdict is a version-invariant node-level value (migration 0037), not a
// per-target readiness row. This is the regression the decouple fixes.
function detail(
  nodeOverrides: Partial<NodeDetailResponse["node"]> = {},
): NodeDetailResponse {
  return {
    organisation_name: "test-org",
    install_path: "/hab",
    min_remaining_free_percent: 10,
    readiness: null,
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
      ...nodeOverrides,
    },
  };
}

describe("NodeDetailPage — Disk Space panel (node-level, no readiness rows)", () => {
  beforeEach(() => {
    vi.mocked(api.fetchFilterTargetChefVersions).mockResolvedValue({ data: [] });
    vi.mocked(api.fetchNodeKitchenRuns).mockResolvedValue([]);
    vi.mocked(api.fetchNodeDependencyGraph).mockResolvedValue({
      nodes: [],
      edges: [],
      metadata: {
        total_roles: 0,
        total_cookbooks: 0,
        incompatible_cookbooks: 0,
        tk_failed_cookbooks: 0,
      },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  async function renderDetail(d: NodeDetailResponse): Promise<HTMLElement> {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(d);
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Disk Space")).toBeInTheDocument(),
    );
    // The panel wraps the "Disk Space" label, the status badge, and details.
    return screen.getByText("Disk Space").closest("div") as HTMLElement;
  }

  it("shows Sufficient from the node verdict", async () => {
    const panel = await renderDetail(
      detail({
        sufficient_disk_space: true,
        available_disk_mb: 8192,
        required_disk_mb: 2048,
      }),
    );
    expect(within(panel).getByText("Sufficient")).toBeInTheDocument();
  });

  it("shows Insufficient from the node verdict", async () => {
    const panel = await renderDetail(
      detail({
        sufficient_disk_space: false,
        available_disk_mb: 512,
        required_disk_mb: 2048,
      }),
    );
    expect(within(panel).getByText("Insufficient")).toBeInTheDocument();
  });

  it("shows Unknown (panel still present) when the verdict is absent", async () => {
    const panel = await renderDetail(detail({}));
    expect(within(panel).getByText("Unknown")).toBeInTheDocument();
  });
});
