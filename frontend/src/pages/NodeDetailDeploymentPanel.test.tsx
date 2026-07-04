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

function nodeWithMigration(
  overrides: Partial<NodeDetailResponse["node"]> = {},
): NodeDetailResponse {
  return {
    ...baseNode,
    node: { ...baseNode.node, ...overrides },
  };
}

describe("NodeDetailPage — Deployment State Panel", () => {
  beforeEach(() => {
    vi.mocked(api.fetchFilterTargetChefVersions).mockResolvedValue({
      data: [],
    });
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

  it("does not render deployment panel when no migration data", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(baseNode);
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getAllByText("test-node").length).toBeGreaterThan(0),
    );
    expect(screen.queryByText("Deployment State")).not.toBeInTheDocument();
  });

  it("renders deployment panel with 'Staged' state", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "hab_dormant",
        active_chef_version: "17.10.0",
        dormant_installed: true,
        dormant_chef_version: "19.3.15",
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Deployment State")).toBeInTheDocument(),
    );
    const panel = screen.getByText("Deployment State").closest(".card")!;
    expect(screen.getByText("Staged")).toBeInTheDocument();
    expect(within(panel).getByText("17.10.0")).toBeInTheDocument();
    expect(within(panel).getByText("19.3.15")).toBeInTheDocument();
  });

  it("renders deployment panel with 'Activated' state", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "hab_active",
        active_chef_version: "19.3.15",
        dormant_installed: false,
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Deployment State")).toBeInTheDocument(),
    );
    expect(screen.getByText("Activated")).toBeInTheDocument();
  });

  it("renders deployment panel for retired omnibus_only state without 'Current only'", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "omnibus_only",
        active_chef_version: "17.10.0",
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Deployment State")).toBeInTheDocument(),
    );
    const panel = screen.getByText("Deployment State").closest(".card")!;
    // "Current only" is retired; omnibus_only now shows the neutral "—".
    expect(screen.queryByText("Current only")).not.toBeInTheDocument();
    expect(within(panel).getByText("17.10.0")).toBeInTheDocument();
  });

  it("renders speculative converge section when data present", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "hab_dormant",
        active_chef_version: "17.10.0",
        dormant_chef_version: "19.3.15",
        target_version: "19.3.15",
        target_converge_status: "success",
        target_execution_time: "2025-06-01T22:00:00Z",
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Speculative Converge")).toBeInTheDocument(),
    );
    expect(screen.getByText("Success")).toBeInTheDocument();
  });

  it("renders speculative converge failure", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "hab_dormant",
        active_chef_version: "17.10.0",
        dormant_chef_version: "19.3.15",
        target_version: "19.3.15",
        target_converge_status: "failed",
        target_execution_time: "2025-06-01T22:00:00Z",
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Speculative Converge")).toBeInTheDocument(),
    );
    expect(screen.getByText("Failed")).toBeInTheDocument();
  });

  it("shows 'Ready to Activate' callout when staged + converge success", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "hab_dormant",
        active_chef_version: "17.10.0",
        dormant_chef_version: "19.3.15",
        target_version: "19.3.15",
        target_converge_status: "success",
        target_execution_time: "2025-06-01T22:00:00Z",
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Ready to Activate")).toBeInTheDocument(),
    );
  });

  it("does not show 'Ready to Activate' when state is not hab_dormant", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "hab_active",
        active_chef_version: "19.3.15",
        target_version: "19.3.15",
        target_converge_status: "success",
        target_execution_time: "2025-06-01T22:00:00Z",
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Deployment State")).toBeInTheDocument(),
    );
    expect(screen.queryByText("Ready to Activate")).not.toBeInTheDocument();
  });

  it("does not show 'Ready to Activate' when converge status is fail", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "hab_dormant",
        active_chef_version: "17.10.0",
        dormant_chef_version: "19.3.15",
        target_version: "19.3.15",
        target_converge_status: "failed",
        target_execution_time: "2025-06-01T22:00:00Z",
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Deployment State")).toBeInTheDocument(),
    );
    expect(screen.queryByText("Ready to Activate")).not.toBeInTheDocument();
  });

  it("hides speculative converge section when no converge data", async () => {
    vi.mocked(api.fetchNodeDetail).mockResolvedValue(
      nodeWithMigration({
        migration_state: "omnibus_only",
        active_chef_version: "17.10.0",
      }),
    );
    render(<NodeDetailPage />);
    await waitFor(() =>
      expect(screen.getByText("Deployment State")).toBeInTheDocument(),
    );
    expect(screen.queryByText("Speculative Converge")).not.toBeInTheDocument();
  });
});
