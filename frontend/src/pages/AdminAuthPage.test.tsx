// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import { AdminAuthPage } from "./AdminAuthPage";

vi.mock("../api");

const mockAuthConfig = {
  providers: [{ type: "local" }],
  session_expiry: "24h",
  min_password_length: 8,
  lockout_attempts: 5,
};

describe("AdminAuthPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockAuthConfig as never);
  });

  it("renders page heading", async () => {
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(screen.getByText("Authentication")).toBeInTheDocument(),
    );
  });

  it("loads and shows local provider card", async () => {
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(screen.getByText("Local Provider")).toBeInTheDocument(),
    );
  });

  it("shows session_expiry value", async () => {
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(screen.getByDisplayValue("24h")).toBeInTheDocument(),
    );
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminAuthPage />);
    await waitFor(() => screen.getByText("Authentication"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});
