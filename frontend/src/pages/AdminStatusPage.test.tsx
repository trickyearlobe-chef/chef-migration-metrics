// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import { AdminStatusPage } from "./AdminStatusPage";
import type { AdminStatus } from "../types";

vi.mock("../api");

const mockStatus: AdminStatus = {
  status: "healthy",
  version: "test-9.9.9",
  datastore: { status: "connected", pending_migrations: 0 },
  credential_storage: {
    encryption_key_configured: true,
    total_credentials: 3,
    credential_types: { chef_client_key: 3 },
    orphaned_credentials: 1,
  },
  collection: {
    next_run_at: "2026-06-15T13:00:00Z",
    last_run_at: "2026-06-15T12:00:00Z",
    last_run_status: "completed",
  },
  organisations: [
    {
      name: "org-a",
      credential_source: "database",
      last_collected_at: "2026-06-15T12:00:00Z",
      status: "completed",
      node_count: 2000,
    },
    {
      name: "org-b",
      credential_source: "file",
      last_collected_at: null,
      status: "never_collected",
      node_count: 0,
    },
  ],
};

describe("AdminStatusPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchAdminStatus).mockResolvedValue(mockStatus as never);
  });

  it("renders the heading and version", async () => {
    render(<AdminStatusPage />);
    await waitFor(() =>
      expect(screen.getByText("Operational Status")).toBeInTheDocument(),
    );
    expect(screen.getByText("test-9.9.9")).toBeInTheDocument();
  });

  it("shows datastore and credential-storage fields", async () => {
    render(<AdminStatusPage />);
    await waitFor(() => screen.getByText("Operational Status"));
    // pending migrations value
    expect(screen.getByText("Pending migrations")).toBeInTheDocument();
    // credential totals and orphan count
    expect(screen.getByText("Total credentials")).toBeInTheDocument();
    expect(screen.getByText("Orphaned")).toBeInTheDocument();
    // type breakdown row
    expect(screen.getByText("chef_client_key")).toBeInTheDocument();
  });

  it("lists organisations with their status and source", async () => {
    render(<AdminStatusPage />);
    await waitFor(() => screen.getByText("org-a"));
    expect(screen.getByText("org-b")).toBeInTheDocument();
    expect(screen.getByText("database")).toBeInTheDocument();
    expect(screen.getByText("never_collected")).toBeInTheDocument();
    expect(screen.getByText("2,000")).toBeInTheDocument();
  });

  it("shows an error with retry when the fetch fails", async () => {
    vi.mocked(api.fetchAdminStatus).mockRejectedValue(
      new Error("Connection failed"),
    );
    render(<AdminStatusPage />);
    await waitFor(() =>
      expect(screen.getByText(/Connection failed/)).toBeInTheDocument(),
    );
  });
});
