// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AdminServerPage } from "./AdminServerPage";

vi.mock("../api");

const mockServerConfig = {
  listen_address: "0.0.0.0",
  port: 8080,
  tls: {
    mode: "off",
    cert_source: "file",
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
      dns_provider_config: {},
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

  it("CA Path field label states it enables mutual TLS", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...mockServerConfig,
      tls: { ...mockServerConfig.tls, mode: "static", cert_path: "/c", key_path: "/k", ca_path: "" },
    } as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByText(/enables mutual TLS/i)).toBeInTheDocument();
  });

  it("warns about mTLS lockout only when CA Path is set", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...mockServerConfig,
      tls: { ...mockServerConfig.tls, mode: "static", cert_path: "/c", key_path: "/k", ca_path: "" },
    } as never);
    const { unmount } = render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.queryByText(/enforces mutual TLS/i)).not.toBeInTheDocument();
    unmount();

    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...mockServerConfig,
      tls: { ...mockServerConfig.tls, mode: "static", cert_path: "/c", key_path: "/k", ca_path: "/etc/ssl/client-ca.crt" },
    } as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByText(/enforces mutual TLS/i)).toBeInTheDocument();
    expect(screen.getByText(/locks everyone out/i)).toBeInTheDocument();
    expect(screen.getByText(/ERR_BAD_SSL_CLIENT_AUTH_CERT/)).toBeInTheDocument();
  });

  it("save button is disabled when no changes", async () => {
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("renders listen address and port loaded from config", async () => {
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("HTTP Listener"));
    expect(screen.getByDisplayValue("0.0.0.0")).toBeInTheDocument();
    expect(screen.getByDisplayValue("8080")).toBeInTheDocument();
  });

  it("saves an edited listen port", async () => {
    vi.mocked(api.saveServerConfig).mockResolvedValue({
      value: { ...mockServerConfig, port: 9090 },
      restart_required: true,
    } as never);
    const user = userEvent.setup();

    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("HTTP Listener"));

    const portInput = screen.getByDisplayValue("8080");
    await user.clear(portInput);
    await user.type(portInput, "9090");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveServerConfig).toHaveBeenCalled());
    const arg = vi.mocked(api.saveServerConfig).mock.calls[0][0];
    expect(arg.port).toBe(9090);
  });

  it("does not show Apply & Restart before a save", async () => {
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("HTTP Listener"));
    expect(
      screen.queryByRole("button", { name: /apply & restart/i }),
    ).not.toBeInTheDocument();
  });

  async function saveAChange() {
    vi.mocked(api.saveServerConfig).mockResolvedValue({
      value: { ...mockServerConfig, port: 9090 },
      restartRequired: true,
    } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("HTTP Listener"));
    const portInput = screen.getByDisplayValue("8080");
    await user.clear(portInput);
    await user.type(portInput, "9090");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(api.saveServerConfig).toHaveBeenCalled());
    return user;
  }

  it("shows an enabled Apply & Restart button after a successful save", async () => {
    await saveAChange();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /apply & restart/i }),
      ).toBeEnabled(),
    );
  });

  it("triggers a restart and waits for the server to come back", async () => {
    vi.mocked(api.restartServer).mockResolvedValue({
      status: "restarting",
      message: "ok",
    });
    vi.mocked(api.waitForServerHealthy).mockResolvedValue(undefined);

    const user = await saveAChange();
    await user.click(
      await screen.findByRole("button", { name: /apply & restart/i }),
    );

    await waitFor(() => expect(api.restartServer).toHaveBeenCalled());
    await waitFor(() => expect(api.waitForServerHealthy).toHaveBeenCalled());
  });

  it("shows an error when the restart request fails", async () => {
    vi.mocked(api.restartServer).mockRejectedValue(new Error("boom"));

    const user = await saveAChange();
    await user.click(
      await screen.findByRole("button", { name: /apply & restart/i }),
    );

    await waitFor(() =>
      expect(screen.getByText(/boom/i)).toBeInTheDocument(),
    );
  });
});
