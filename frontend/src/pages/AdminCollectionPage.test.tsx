// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AdminCollectionPage } from "./AdminCollectionPage";

vi.mock("../api");

const mockCollection = {
  schedule: "0 * * * *",
  stale_node_threshold_days: 30,
  stale_cookbook_threshold_days: 30,
  skip_server_cookbook_download: false,
  delete_server_cookbooks_after_scan: null,
};

describe("AdminCollectionPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchCollection).mockResolvedValue(mockCollection as never);
  });

  it("renders page heading", async () => {
    render(<AdminCollectionPage />);
    await waitFor(() =>
      expect(screen.getByText("Collection Settings")).toBeInTheDocument(),
    );
  });

  it("shows loading spinner initially", () => {
    vi.mocked(api.fetchCollection).mockImplementation(() => new Promise(() => {}));
    render(<AdminCollectionPage />);
    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it("loads and displays current schedule value after fetch resolves", async () => {
    render(<AdminCollectionPage />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("0 * * * *")).toBeInTheDocument(),
    );
  });

  it("save button is disabled when nothing changed", async () => {
    render(<AdminCollectionPage />);
    await waitFor(() =>
      expect(screen.getByText("Collection Settings")).toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("shows save error when saveCollection rejects", async () => {
    vi.mocked(api.saveCollection).mockRejectedValue(new Error("Network error"));
    render(<AdminCollectionPage />);
    await waitFor(() => screen.getByText("Collection Settings"));

    const user = userEvent.setup();
    await user.type(screen.getByDisplayValue("0 * * * *"), "x");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(screen.getByText(/network error/i)).toBeInTheDocument(),
    );
  });
});
