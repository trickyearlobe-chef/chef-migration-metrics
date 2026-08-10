// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";

vi.mock("../api");

import { ImportRejections } from "./ImportRejections";

// journeys/ownership-intake.md: "To have the rows it could not use handed back
// to me as a worklist — which row, and what was wrong with it — so I can get
// the source fixed rather than silently importing three quarters of it."
//
// The row number and the reason are the whole point: without them the list says
// an import went wrong without saying where, which is no more use than a count.

describe("ImportRejections", () => {
  beforeEach(() => {
    vi.mocked(api.fetchImportRejections).mockResolvedValue({
      data: [
        {
          import_label: "cmdb-nightly",
          run_at: "2026-08-10T09:00:00Z",
          source_row: 412,
          reason: "no owner",
          owner_raw: "  ",
          entity_type: "git_repo",
          entity_key: "web-app",
        },
      ],
      pagination: { page: 1, per_page: 100, total_items: 1, total_pages: 1 },
    });
  });

  it("says which row and what was wrong with it", async () => {
    render(<ImportRejections />);

    await waitFor(() => expect(screen.getByText("412")).toBeInTheDocument());
    expect(screen.getByText("no owner")).toBeInTheDocument();
  });

  it("names the import the row came from, so it can be taken to its owner", async () => {
    render(<ImportRejections />);
    await waitFor(() => expect(screen.getByText("cmdb-nightly")).toBeInTheDocument());
  });

  it("shows what the row was for, so a rejection can be recognised", async () => {
    render(<ImportRejections />);
    await waitFor(() => expect(screen.getByText(/web-app/)).toBeInTheDocument());
  });

  // Empty is the state worth reaching, so it is said rather than shown as a
  // table with no rows — which reads as "not loaded yet".
  it("says plainly when every row was used", async () => {
    vi.mocked(api.fetchImportRejections).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 100, total_items: 0, total_pages: 0 },
    });
    render(<ImportRejections />);
    await waitFor(() => expect(screen.getByText("Every row was used")).toBeInTheDocument());
  });

  // A worklist that fails to load must say so. Showing an empty table would
  // read as "nothing was rejected", which is the opposite of the truth.
  it("does not report an empty worklist when it failed to load", async () => {
    vi.mocked(api.fetchImportRejections).mockRejectedValue(new Error("database unavailable"));
    render(<ImportRejections />);

    await waitFor(() => expect(screen.getByText(/database unavailable/)).toBeInTheDocument());
    expect(screen.queryByText("Every row was used")).not.toBeInTheDocument();
  });
});
