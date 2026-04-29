// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { GitKitchenSection } from "./GitKitchenSection";
import type { GitKitchenPlanResult, GitKitchenResult } from "../types";

vi.mock("../api");

vi.mock("../context/GlobalFilterContext", () => ({
  useGlobalFilters: () => ({
    targetChefVersion: "19.1.164",
    targetVersions: ["19.1.164"],
    setTargetChefVersion: vi.fn(),
    staleTiers: [],
    setStaleTiers: vi.fn(),
    versionsLoading: false,
  }),
}));

vi.mock("../hooks/useWebSocket", () => ({
  useWebSocket: () => ({
    status: "connected",
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
    onEvent: vi.fn(() => vi.fn()),
    disconnect: vi.fn(),
    reconnect: vi.fn(),
  }),
}));

const basePlan: GitKitchenPlanResult = {
  git_repo_name: "example-cookbook",
  git_repo_url: "https://git.example.com/example-cookbook",
  commit_sha: "abc123",
  instances: [
    {
      instance_name: "default-ubuntu-2204",
      suite_name: "default",
      platform_name: "ubuntu-22.04",
      status: "mapped",
      status_reason: "Image found",
      image_name: "ubuntu:22.04",
    },
    {
      instance_name: "default-centos-8",
      suite_name: "default",
      platform_name: "centos-8",
      status: "unmapped",
      status_reason: "No image mapping found",
    },
    {
      instance_name: "default-windows-2022",
      suite_name: "default",
      platform_name: "windows-2022",
      status: "skipped",
      status_reason: "Platform skipped",
    },
    {
      instance_name: "default-legacy",
      suite_name: "default",
      platform_name: "legacy-os",
      status: "excluded",
      status_reason: "Excluded by config",
    },
  ],
  total: 4,
  mapped: 1,
  unmapped: 1,
  skipped: 1,
  excluded: 1,
  user_excluded: 0,
};

const baseResult: GitKitchenResult = {
  id: "r1",
  git_repo_name: "example-cookbook",
  git_repo_url: "https://git.example.com/example-cookbook",
  target_chef_version: "18.5.0",
  commit_sha: "abc123",
  platform_name: "ubuntu-22.04",
  suite_name: "default",
  instance_name: "default-ubuntu-2204",
  passed: true,
  timed_out: false,
  duration_seconds: 90,
  created_at: "2025-01-15T10:00:00Z",
};

describe("GitKitchenSection", () => {
  beforeEach(() => {
    vi.mocked(api.fetchGitKitchenInstances).mockResolvedValue(basePlan);
    vi.mocked(api.fetchGitKitchenResults).mockResolvedValue([baseResult]);
    vi.mocked(api.fetchKitchenExclusions).mockResolvedValue([]);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders instance table with mapped instances", async () => {
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("Test Kitchen Instances")).toBeInTheDocument(),
    );
    expect(screen.getByText("default-ubuntu-2204")).toBeInTheDocument();
    expect(screen.getByText("default-centos-8")).toBeInTheDocument();
    expect(screen.getByText("default-windows-2022")).toBeInTheDocument();
    expect(screen.getByText("default-legacy")).toBeInTheDocument();
  });

  it("shows run button only for mapped instances", async () => {
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("Test Kitchen Instances")).toBeInTheDocument(),
    );
    const runButtons = screen.getAllByRole("button", { name: "Run" });
    expect(runButtons).toHaveLength(1);
  });

  it("handles loading state", async () => {
    vi.mocked(api.fetchGitKitchenInstances).mockImplementation(
      () => new Promise(() => {}),
    );
    render(<GitKitchenSection repoName="example-cookbook" />);
    expect(
      screen.getByText("Loading kitchen instances…"),
    ).toBeInTheDocument();
  });

  it("handles error state", async () => {
    vi.mocked(api.fetchGitKitchenInstances).mockRejectedValue(
      new Error("Network error"),
    );
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("Network error")).toBeInTheDocument(),
    );
  });

  it("shows passed result badge for passed instance", async () => {
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("✓ Passed")).toBeInTheDocument(),
    );
  });

  it("shows failed result badge when result has passed=false", async () => {
    vi.mocked(api.fetchGitKitchenResults).mockResolvedValue([
      { ...baseResult, passed: false },
    ]);
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("✗ Failed")).toBeInTheDocument(),
    );
  });

  it("shows running badge when result has passed=null", async () => {
    vi.mocked(api.fetchGitKitchenResults).mockResolvedValue([
      { ...baseResult, passed: null },
    ]);
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("Running…")).toBeInTheDocument(),
    );
  });

  it("calls triggerGitKitchenRun when Run button is clicked", async () => {
    vi.mocked(api.triggerGitKitchenRun).mockResolvedValue({
      message: "Run queued",
    });
    const user = userEvent.setup();
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Run" })).toBeInTheDocument(),
    );
    await user.click(screen.getByRole("button", { name: "Run" }));
    await waitFor(() =>
      expect(api.triggerGitKitchenRun).toHaveBeenCalledWith({
        git_repo_name: "example-cookbook",
        instance_name: "default-ubuntu-2204",
        target_chef_version: "19.1.164",
      }),
    );
    await waitFor(() =>
      expect(screen.getByText("Run queued")).toBeInTheDocument(),
    );
  });

  it("expands output when clicking a result badge", async () => {
    const resultWithOutput: GitKitchenResult = {
      ...baseResult,
      passed: false,
      output: "Converging recipe[default]...\nERROR: package[nginx] failed",
      error_message: "converge failed",
      duration_seconds: 45,
    };
    vi.mocked(api.fetchGitKitchenResults).mockResolvedValue([resultWithOutput]);
    const user = userEvent.setup();
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("✗ Failed")).toBeInTheDocument(),
    );
    expect(screen.queryByText(/Converging recipe/)).not.toBeInTheDocument();
    await user.click(screen.getByText("✗ Failed"));
    expect(screen.getByText(/Converging recipe/)).toBeInTheDocument();
    expect(screen.getByText(/ERROR: package\[nginx\] failed/)).toBeInTheDocument();
    expect(screen.getByText(/Duration: 45s/)).toBeInTheDocument();
    expect(screen.getByText(/Error: converge failed/)).toBeInTheDocument();
  });

  it("collapses output when clicking the badge again", async () => {
    vi.mocked(api.fetchGitKitchenResults).mockResolvedValue([
      { ...baseResult, passed: false, output: "some output" },
    ]);
    const user = userEvent.setup();
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("✗ Failed")).toBeInTheDocument(),
    );
    await user.click(screen.getByText("✗ Failed"));
    expect(screen.getByText("some output")).toBeInTheDocument();
    await user.click(screen.getByText("✗ Failed"));
    expect(screen.queryByText("some output")).not.toBeInTheDocument();
  });

  it("shows placeholder when result has no output", async () => {
    vi.mocked(api.fetchGitKitchenResults).mockResolvedValue([
      { ...baseResult, passed: true, output: undefined },
    ]);
    const user = userEvent.setup();
    render(<GitKitchenSection repoName="example-cookbook" />);
    await waitFor(() =>
      expect(screen.getByText("✓ Passed")).toBeInTheDocument(),
    );
    await user.click(screen.getByText("✓ Passed"));
    expect(screen.getByText("(no output captured)")).toBeInTheDocument();
  });
});
