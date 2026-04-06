// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";
import { AdminServerPage } from "./AdminServerPage";

vi.mock("../api");

const mockServerConfig = {
  listen_address: "0.0.0.0",
  port: 8080,
  tls: {
    mode: "off",
    cert_path: "",
    key_path: "",
    ca_path: "",
    min_version: "",
    http_redirect_port: 0,
    acme: {
      domains: [],
      email: "",
      ca_url: "",
      challenge: "",
      dns_provider: "",
      storage_path: "",
      renew_before_days: 0,
      agree_to_tos: false,
    },
  },
  websocket: {
    enabled: null,
    max_connections: 0,
    send_buffer_size: 0,
    write_timeout_seconds: 0,
    ping_interval_seconds: 0,
    pong_timeout_seconds: 0,
  },
  graceful_shutdown_seconds: 30,
};

describe("AdminServerPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(mockServerConfig as never);
  });

  it("renders page heading", async () => {
    render(<AdminServerPage />);
    await waitFor(() =>
      expect(screen.getByText("Server & TLS")).toBeInTheDocument(),
    );
  });

  it("loads and shows TLS mode off selected", async () => {
    render(<AdminServerPage />);
    await waitFor(() => {
      const modeSelect = screen.getAllByRole("combobox")[0];
      expect(modeSelect).toHaveValue("off");
    });
  });

  it("cert/key path fields are not shown when TLS mode is off", async () => {
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.queryByText("Certificate Path")).not.toBeInTheDocument();
    expect(screen.queryByText("Key Path")).not.toBeInTheDocument();
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});
