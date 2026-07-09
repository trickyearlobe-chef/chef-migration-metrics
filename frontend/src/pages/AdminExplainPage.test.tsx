// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AdminExplainPage, PREFILL_KEY } from "./AdminExplainPage";

vi.mock("../api");

const catalog = {
  entries: [
    { key: "roles_list", label: "Roles list", description: "Roles read path.", analyzable: true },
    { key: "node_list_heavy", label: "Node list (heavy JSON)", description: "Heavy path.", analyzable: true },
  ],
};

const sampleResult = {
  label: "Roles list",
  param_summary: "orgs=3, limit=50",
  analyze: true,
  statement_timeout_ms: 15000,
  captured_at: "2026-07-09T13:20:00Z",
  app_version: "2.15.1",
  run1: { plan_text: "Seq Scan on role_summary", duration_ms: 12.3, truncated: false },
};

describe("AdminExplainPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchExplainCatalog).mockResolvedValue(catalog as never);
    vi.mocked(api.runExplain).mockResolvedValue(sampleResult as never);
    sessionStorage.clear();
  });

  it("renders the catalog entries", async () => {
    render(<AdminExplainPage />);
    await waitFor(() =>
      expect(screen.getByRole("option", { name: "Roles list" })).toBeInTheDocument(),
    );
    expect(screen.getByRole("option", { name: "Node list (heavy JSON)" })).toBeInTheDocument();
  });

  it("runs a canned explain and shows the plan", async () => {
    render(<AdminExplainPage />);
    await waitFor(() => screen.getByRole("option", { name: "Roles list" }));

    await userEvent.click(screen.getByRole("button", { name: /run canned explain/i }));

    await waitFor(() =>
      expect(screen.getByText(/Seq Scan on role_summary/)).toBeInTheDocument(),
    );
    expect(vi.mocked(api.runExplain)).toHaveBeenCalledWith(
      expect.objectContaining({ catalog_key: "roles_list" }),
    );
  });

  it("runs a free-text query", async () => {
    render(<AdminExplainPage />);
    await waitFor(() => screen.getByRole("option", { name: "Roles list" }));

    await userEvent.type(screen.getByLabelText("Custom SQL"), "SELECT 1");
    await userEvent.click(screen.getByRole("button", { name: /run custom explain/i }));

    await waitFor(() =>
      expect(vi.mocked(api.runExplain)).toHaveBeenCalledWith(
        expect.objectContaining({ sql: "SELECT 1" }),
      ),
    );
  });

  it("pre-fills the free-text box from sessionStorage", async () => {
    sessionStorage.setItem(PREFILL_KEY, "SELECT * FROM node_snapshots WHERE x = $1");
    render(<AdminExplainPage />);
    await waitFor(() =>
      expect(screen.getByLabelText("Custom SQL")).toHaveValue(
        "SELECT * FROM node_snapshots WHERE x = $1",
      ),
    );
    // Consumed once.
    expect(sessionStorage.getItem(PREFILL_KEY)).toBeNull();
  });

  it("downloads the plan as a .txt", async () => {
    const createObjectURL = vi.fn(() => "blob:mock");
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, "createObjectURL", { value: createObjectURL, configurable: true });
    Object.defineProperty(URL, "revokeObjectURL", { value: revokeObjectURL, configurable: true });

    render(<AdminExplainPage />);
    await waitFor(() => screen.getByRole("option", { name: "Roles list" }));
    await userEvent.click(screen.getByRole("button", { name: /run canned explain/i }));
    await waitFor(() => screen.getByText(/Seq Scan on role_summary/));

    await userEvent.click(screen.getByRole("button", { name: /download \.txt/i }));
    expect(createObjectURL).toHaveBeenCalled();
  });
});
