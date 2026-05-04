// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import { AdminConcurrencyPage } from "./AdminConcurrencyPage";

vi.mock("../api");

const mockConcurrency = {
  organisation_collection: 5,
  node_page_fetching: 10,
  git_pull: 4,
  cookbook_download: 8,
  cookstyle_scan: 4,
  readiness_evaluation: 4,
};

describe("AdminConcurrencyPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchConcurrency).mockResolvedValue(mockConcurrency as never);
  });

  it("renders page heading", async () => {
    render(<AdminConcurrencyPage />);
    await waitFor(() =>
      expect(screen.getByText("Concurrency Settings")).toBeInTheDocument(),
    );
  });

  it("loads and displays organisation_collection value of 5", async () => {
    render(<AdminConcurrencyPage />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("5")).toBeInTheDocument(),
    );
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminConcurrencyPage />);
    await waitFor(() => screen.getByText("Concurrency Settings"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});
