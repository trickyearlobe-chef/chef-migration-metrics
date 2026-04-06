// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import { AdminLoggingPage } from "./AdminLoggingPage";

vi.mock("../api");

const mockLogging = { level: "INFO", retention_days: 30 };

describe("AdminLoggingPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchLogging).mockResolvedValue(mockLogging as never);
  });

  it("renders page heading", async () => {
    render(<AdminLoggingPage />);
    await waitFor(() =>
      expect(screen.getByText("Logging Settings")).toBeInTheDocument(),
    );
  });

  it("loads and displays INFO as selected log level", async () => {
    render(<AdminLoggingPage />);
    await waitFor(() =>
      expect(screen.getByRole("combobox")).toHaveValue("INFO"),
    );
  });

  it("save button is disabled initially (no dirty state)", async () => {
    render(<AdminLoggingPage />);
    await waitFor(() => screen.getByText("Logging Settings"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("shows error when fetch fails", async () => {
    vi.mocked(api.fetchLogging).mockRejectedValue(new Error("Network error"));
    render(<AdminLoggingPage />);
    await waitFor(() =>
      expect(screen.getByText(/failed to load logging settings/i)).toBeInTheDocument(),
    );
  });
});
