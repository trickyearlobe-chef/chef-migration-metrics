// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AdminNotificationsPage } from "./AdminNotificationsPage";

vi.mock("../api");

const mockNotifications = {
  enabled: false,
  channels: [],
  readiness_milestones: [80, 90, 100],
  stale_node_alert_count: 0,
};

describe("AdminNotificationsPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchNotifications).mockResolvedValue(
      mockNotifications as never,
    );
  });

  it("renders page heading", async () => {
    render(<AdminNotificationsPage />);
    await waitFor(() =>
      expect(screen.getByText("Notifications")).toBeInTheDocument(),
    );
  });

  it("loads with notifications disabled (checkbox unchecked)", async () => {
    render(<AdminNotificationsPage />);
    await waitFor(() => screen.getByText("Notifications"));
    const checkbox = screen.getByRole("checkbox", { hidden: true });
    expect(checkbox).not.toBeChecked();
  });

  it("add channel button adds a new channel card", async () => {
    const user = userEvent.setup();
    render(<AdminNotificationsPage />);
    await waitFor(() => screen.getByText("Notifications"));

    await user.click(screen.getByRole("button", { name: /add channel/i }));

    expect(screen.getByText("New Channel")).toBeInTheDocument();
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminNotificationsPage />);
    await waitFor(() => screen.getByText("Notifications"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});
