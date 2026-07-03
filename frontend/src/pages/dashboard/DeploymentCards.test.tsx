// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../../api";
import { DeploymentStatusCard } from "./DeploymentCards";
import type { DeploymentStatusResponse } from "../../types";

vi.mock("../../api");

const emptyResponse: DeploymentStatusResponse = { data: [], total_nodes: 0 };

const statusResponse: DeploymentStatusResponse = {
  data: [
    {
      version: "19.3.15",
      staged: 5,
      activated: 2,
      converge_passing: 4,
      converge_failing: 1,
      total: 7,
    },
    {
      version: "19.3.5",
      staged: 0,
      activated: 8,
      converge_passing: 8,
      converge_failing: 0,
      total: 8,
    },
  ],
  total_nodes: 100,
};

function renderWithRouter(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("DeploymentStatusCard", () => {
  beforeEach(() => {
    vi.mocked(api.fetchDeploymentStatus).mockResolvedValue(emptyResponse);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("shows loading state then renders card title", async () => {
    vi.mocked(api.fetchDeploymentStatus).mockResolvedValue(statusResponse);
    renderWithRouter(<DeploymentStatusCard />);
    expect(
      screen.getByText("Loading deployment status…"),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(
        screen.getByText("Deployment Status — Per Version"),
      ).toBeInTheDocument(),
    );
  });

  it("shows error state on API failure", async () => {
    vi.mocked(api.fetchDeploymentStatus).mockRejectedValue(
      new Error("server error"),
    );
    renderWithRouter(<DeploymentStatusCard />);
    await waitFor(() =>
      expect(screen.getByText("server error")).toBeInTheDocument(),
    );
  });

  it("shows empty state when no versions deployed", async () => {
    vi.mocked(api.fetchDeploymentStatus).mockResolvedValue(emptyResponse);
    renderWithRouter(<DeploymentStatusCard />);
    await waitFor(() =>
      expect(
        screen.getByText(/no deployment data/i),
      ).toBeInTheDocument(),
    );
  });

  it("renders version entries as battery bars", async () => {
    vi.mocked(api.fetchDeploymentStatus).mockResolvedValue(statusResponse);
    renderWithRouter(<DeploymentStatusCard />);
    await waitFor(() => {
      expect(screen.getByText("19.3.15")).toBeInTheDocument();
      expect(screen.getByText("19.3.5")).toBeInTheDocument();
    });
  });

  it("passes organisation to API", async () => {
    vi.mocked(api.fetchDeploymentStatus).mockResolvedValue(emptyResponse);
    renderWithRouter(<DeploymentStatusCard organisation="staging" />);
    await waitFor(() =>
      expect(api.fetchDeploymentStatus).toHaveBeenCalledWith("staging"),
    );
  });

  it("links staged/activated to the RAW migration_state values, not the display labels", async () => {
    // Regression: the drill-down must send migration_state=hab_dormant (the stored
    // value), not "Staged" (the label) — otherwise the Nodes list matches nothing
    // while the dashboard counts thousands.
    vi.mocked(api.fetchDeploymentStatus).mockResolvedValue(statusResponse);
    renderWithRouter(<DeploymentStatusCard />);

    const staged = await screen.findByRole("link", { name: /5 staged/i });
    expect(staged.getAttribute("href")).toContain("migration_state=hab_dormant");
    expect(staged.getAttribute("href")).not.toContain("migration_state=Staged");

    const activated = screen.getByRole("link", { name: /2 activated/i });
    expect(activated.getAttribute("href")).toContain("migration_state=hab_active");
  });
});
