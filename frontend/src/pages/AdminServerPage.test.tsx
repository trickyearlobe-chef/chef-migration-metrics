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
      register_hostname: false,
      hostname_ttl: 60,
      hostname_interface: "",
      hostname_ip: "",
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
  trusted_proxy: false,
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

  // --- cert_source: db (Chunk 4 — DB cert/key UI) ---

  const staticFileConfig = {
    ...mockServerConfig,
    tls: { ...mockServerConfig.tls, mode: "static", cert_source: "file", cert_path: "/c", key_path: "/k" },
  };
  const staticDbConfig = {
    ...mockServerConfig,
    tls: { ...mockServerConfig.tls, mode: "static", cert_source: "db", cert_path: "", key_path: "" },
  };

  it("shows a Certificate Source selector in static mode", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticFileConfig as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByLabelText("Certificate source")).toHaveValue("file");
  });

  it("shows path fields for file source and PEM textareas for db source", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticFileConfig as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    // File source: paths shown, PEM textareas hidden.
    expect(screen.getByText("Certificate Path")).toBeInTheDocument();
    expect(screen.queryByLabelText("Private key (PEM)")).not.toBeInTheDocument();

    // Toggle to db: paths hidden, PEM textareas shown.
    await user.selectOptions(screen.getByLabelText("Certificate source"), "db");
    expect(screen.queryByText("Certificate Path")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Certificate (PEM)")).toBeInTheDocument();
    expect(screen.getByLabelText("Private key (PEM)")).toBeInTheDocument();
  });

  it("never pre-fills the private key textarea for db source", async () => {
    // GET never returns key material; the textarea must start empty even when a
    // certificate is already installed.
    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...staticDbConfig,
      tls_certificate_info: {
        subject: "CN=example.com",
        issuer: "CN=Example CA",
        dns_names: ["example.com"],
        not_before: "2026-01-01T00:00:00Z",
        not_after: "2027-01-01T00:00:00Z",
      },
    } as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByLabelText("Certificate (PEM)")).toHaveValue("");
    expect(screen.getByLabelText("Private key (PEM)")).toHaveValue("");
  });

  it("renders the installed certificate metadata panel for db source", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...staticDbConfig,
      tls_certificate_info: {
        subject: "CN=example.com",
        issuer: "CN=Example CA",
        dns_names: ["example.com", "www.example.com"],
        not_before: "2026-01-01T00:00:00Z",
        not_after: "2027-01-01T00:00:00Z",
      },
    } as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByText("CN=example.com")).toBeInTheDocument();
    expect(screen.getByText("CN=Example CA")).toBeInTheDocument();
    expect(screen.getByText(/www\.example\.com/)).toBeInTheDocument();
  });

  it("prompts to paste a certificate when none is installed for db source", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByText(/no certificate.*stored/i)).toBeInTheDocument();
  });

  it("sends pasted certificate and private key on save for db source", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    vi.mocked(api.saveServerConfig).mockResolvedValue({
      value: staticDbConfig,
      restartRequired: true,
    } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.type(screen.getByLabelText("Certificate (PEM)"), "CERTPEM");
    await user.type(screen.getByLabelText("Private key (PEM)"), "KEYPEM");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveServerConfig).toHaveBeenCalled());
    const arg = vi.mocked(api.saveServerConfig).mock.lastCall![0];
    expect(arg.tls.certificate).toBe("CERTPEM");
    expect(arg.tls.private_key).toBe("KEYPEM");
  });

  it("clears the PEM textareas after a successful db save", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    // PUT response omits cert/key (write-only), as the real API does.
    vi.mocked(api.saveServerConfig).mockResolvedValue({
      value: staticDbConfig,
      restartRequired: true,
    } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.type(screen.getByLabelText("Private key (PEM)"), "KEYPEM");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveServerConfig).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.getByLabelText("Private key (PEM)")).toHaveValue(""),
    );
  });

  // --- CSR generation (Chunk 6 — CSR UI) ---

  const CSR_PEM = "-----BEGIN CERTIFICATE REQUEST-----\nMIIB\n-----END CERTIFICATE REQUEST-----\n";

  it("shows the CSR generation panel only for db source", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticFileConfig as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    // File source: no CSR panel.
    expect(
      screen.queryByRole("button", { name: /generate csr/i }),
    ).not.toBeInTheDocument();

    // Toggle to db: CSR panel appears.
    await user.selectOptions(screen.getByLabelText("Certificate source"), "db");
    expect(
      screen.getByRole("button", { name: /generate csr/i }),
    ).toBeInTheDocument();
  });

  it("defaults the key algorithm to ecdsa-p256 and offers all algorithms", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    const algo = screen.getByLabelText("Key algorithm") as HTMLSelectElement;
    expect(algo).toHaveValue("ecdsa-p256");
    const values = Array.from(algo.options).map((o) => o.value);
    expect(values).toEqual([
      "ecdsa-p256",
      "ecdsa-p384",
      "rsa-2048",
      "rsa-3072",
      "rsa-4096",
    ]);
  });

  it("disables Generate CSR until an identifier (CN or SAN) is present", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    expect(screen.getByRole("button", { name: /generate csr/i })).toBeDisabled();
    await user.type(screen.getByLabelText("Common Name"), "example.com");
    expect(screen.getByRole("button", { name: /generate csr/i })).toBeEnabled();
  });

  it("adds and removes DNS SANs", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    const sanInput = screen.getByPlaceholderText("dns.example.com");
    await user.type(sanInput, "www.example.com{enter}");
    expect(screen.getByText("www.example.com")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /remove www\.example\.com/i }));
    expect(screen.queryByText("www.example.com")).not.toBeInTheDocument();
  });

  it("sends the subject, SANs and algorithm to generateCSR", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    vi.mocked(api.generateCSR).mockResolvedValue({
      csr_pem: CSR_PEM,
      key_algorithm: "rsa-2048",
    } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.type(screen.getByLabelText("Common Name"), "example.com");
    await user.type(screen.getByLabelText("Organization"), "Example Corp");
    await user.type(screen.getByPlaceholderText("dns.example.com"), "www.example.com{enter}");
    await user.type(screen.getByPlaceholderText("10.0.0.1"), "10.0.0.1{enter}");
    await user.selectOptions(screen.getByLabelText("Key algorithm"), "rsa-2048");
    await user.click(screen.getByRole("button", { name: /generate csr/i }));

    await waitFor(() => expect(api.generateCSR).toHaveBeenCalled());
    const arg = vi.mocked(api.generateCSR).mock.lastCall![0];
    expect(arg.common_name).toBe("example.com");
    expect(arg.organization).toBe("Example Corp");
    expect(arg.dns_sans).toEqual(["www.example.com"]);
    expect(arg.ip_sans).toEqual(["10.0.0.1"]);
    expect(arg.key_algorithm).toBe("rsa-2048");
  });

  it("shows the returned CSR PEM and guidance after generation", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    vi.mocked(api.generateCSR).mockResolvedValue({
      csr_pem: CSR_PEM,
      key_algorithm: "ecdsa-p256",
    } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.type(screen.getByLabelText("Common Name"), "example.com");
    await user.click(screen.getByRole("button", { name: /generate csr/i }));

    await waitFor(() =>
      expect(screen.getByLabelText("Generated CSR (PEM)")).toHaveValue(CSR_PEM),
    );
    // Guidance to submit the CSR and paste the signed cert back above.
    expect(screen.getByText(/paste the signed certificate/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /download csr/i }),
    ).toBeInTheDocument();
  });

  // --- Behind-proxy plain-HTTP toggle (Chunk 8d — tls.md § 9.1) ---

  it("shows the behind-proxy plain-HTTP toggle", async () => {
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(
      screen.getByRole("checkbox", { name: /terminate tls at a proxy/i }),
    ).toBeInTheDocument();
  });

  it("reflects mode off + trusted_proxy as the toggle being on", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...mockServerConfig,
      trusted_proxy: true,
    } as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(
      screen.getByRole("checkbox", { name: /terminate tls at a proxy/i }),
    ).toBeChecked();
  });

  it("enabling behind-proxy sets mode off and trusted_proxy in the save payload", async () => {
    // Start from a static config so the toggle has to flip mode to off too.
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticFileConfig as never);
    vi.mocked(api.saveServerConfig).mockResolvedValue({
      value: { ...staticFileConfig, tls: { ...staticFileConfig.tls, mode: "off" }, trusted_proxy: true },
      restartRequired: true,
    } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.click(
      screen.getByRole("checkbox", { name: /terminate tls at a proxy/i }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveServerConfig).toHaveBeenCalled());
    const arg = vi.mocked(api.saveServerConfig).mock.lastCall![0];
    expect(arg.tls.mode).toBe("off");
    expect(arg.trusted_proxy).toBe(true);
  });

  it("disabling behind-proxy clears trusted_proxy in the save payload", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...mockServerConfig,
      trusted_proxy: true,
    } as never);
    vi.mocked(api.saveServerConfig).mockResolvedValue({
      value: { ...mockServerConfig, trusted_proxy: false },
      restartRequired: true,
    } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.click(
      screen.getByRole("checkbox", { name: /terminate tls at a proxy/i }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveServerConfig).toHaveBeenCalled());
    const arg = vi.mocked(api.saveServerConfig).mock.lastCall![0];
    expect(arg.trusted_proxy).toBe(false);
  });

  it("warns about X-Forwarded-Proto trust only when behind-proxy is enabled", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(mockServerConfig as never);
    const { unmount } = render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.queryByText(/spoof/i)).not.toBeInTheDocument();
    unmount();

    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...mockServerConfig,
      trusted_proxy: true,
    } as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getAllByText(/X-Forwarded-Proto/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/spoof/i)).toBeInTheDocument();
  });

  it("shows an error when CSR generation fails", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(staticDbConfig as never);
    vi.mocked(api.generateCSR).mockRejectedValue(new Error("bad subject"));
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.type(screen.getByLabelText("Common Name"), "example.com");
    await user.click(screen.getByRole("button", { name: /generate csr/i }));

    await waitFor(() =>
      expect(screen.getByText(/bad subject/i)).toBeInTheDocument(),
    );
  });

  // --- ACME UI (Chunk 10 — Feature 3 frontend) ---

  const acmeBase = {
    ...mockServerConfig,
    tls: {
      ...mockServerConfig.tls,
      mode: "acme",
      acme: {
        ...mockServerConfig.tls.acme,
        domains: ["app.example.com"],
        email: "admin@example.com",
        ca_url: "https://acme-v02.api.letsencrypt.org/directory",
        challenge: "dns-01",
        dns_provider: "route53",
        dns_provider_config: { region: "us-east-1", hosted_zone_id: "Z123" },
        agree_to_tos: true,
      },
    },
  };
  const withAcme = (overrides: Record<string, unknown>) => ({
    ...acmeBase,
    tls: { ...acmeBase.tls, acme: { ...acmeBase.tls.acme, ...overrides } },
  });

  it("warns when the ACME CA URL is a staging endpoint", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(
      withAcme({ ca_url: "https://acme-staging-v02.api.letsencrypt.org/directory" }) as never,
    );
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByText(/staging/i)).toBeInTheDocument();
  });

  it("does not warn for a production CA URL", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(acmeBase as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.queryByText(/staging/i)).not.toBeInTheDocument();
  });

  it("shows Route 53 region, hosted zone and AWS cred inputs for dns-01 route53", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(acmeBase as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByLabelText("Route 53 region")).toHaveValue("us-east-1");
    expect(screen.getByLabelText("Route 53 hosted zone ID")).toHaveValue("Z123");
    expect(screen.getByLabelText("AWS access key ID")).toBeInTheDocument();
    expect(screen.getByLabelText("AWS secret access key")).toBeInTheDocument();
  });

  it("hides DNS-01 provider fields for http-01", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(withAcme({ challenge: "http-01" }) as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.queryByLabelText("Route 53 region")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("AWS secret access key")).not.toBeInTheDocument();
  });

  it("renders the AWS secret access key as a write-only password input", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(acmeBase as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    const secret = screen.getByLabelText("AWS secret access key") as HTMLInputElement;
    expect(secret).toHaveValue("");
    expect(secret.type).toBe("password");
  });

  it("sends Route 53 region/zone and creds in the save payload", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(acmeBase as never);
    vi.mocked(api.saveServerConfig).mockResolvedValue({ value: acmeBase, restartRequired: true } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.type(screen.getByLabelText("AWS access key ID"), "AKIAEXAMPLE");
    await user.type(screen.getByLabelText("AWS secret access key"), "shh");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveServerConfig).toHaveBeenCalled());
    const arg = vi.mocked(api.saveServerConfig).mock.lastCall![0];
    expect(arg.tls.acme.dns_provider_config.region).toBe("us-east-1");
    expect(arg.tls.acme.dns_provider_config.hosted_zone_id).toBe("Z123");
    expect(arg.tls.acme.route53?.access_key_id).toBe("AKIAEXAMPLE");
    expect(arg.tls.acme.route53?.secret_access_key).toBe("shh");
  });

  it("shows the register-hostname toggle only for route53 dns-01", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(acmeBase as never);
    const { unmount } = render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByRole("checkbox", { name: /register hostname/i })).toBeInTheDocument();
    unmount();

    vi.mocked(api.fetchServerConfig).mockResolvedValue(withAcme({ challenge: "http-01" }) as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.queryByRole("checkbox", { name: /register hostname/i })).not.toBeInTheDocument();
  });

  it("IP source selector reveals interface or manual IP inputs", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue(withAcme({ register_hostname: true }) as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    const sel = screen.getByLabelText("IP source");
    expect(sel).toHaveValue("auto");
    expect(screen.queryByLabelText("Hostname IP address")).not.toBeInTheDocument();

    await user.selectOptions(sel, "manual");
    expect(screen.getByLabelText("Hostname IP address")).toBeInTheDocument();

    await user.selectOptions(sel, "interface");
    expect(screen.getByLabelText("Hostname network interface")).toBeInTheDocument();
    expect(screen.queryByLabelText("Hostname IP address")).not.toBeInTheDocument();
  });

  it("sends hostname self-registration fields on save", async () => {
    const cfg = withAcme({ register_hostname: true });
    vi.mocked(api.fetchServerConfig).mockResolvedValue(cfg as never);
    vi.mocked(api.saveServerConfig).mockResolvedValue({ value: cfg, restartRequired: true } as never);
    const user = userEvent.setup();
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));

    await user.selectOptions(screen.getByLabelText("IP source"), "manual");
    await user.type(screen.getByLabelText("Hostname IP address"), "203.0.113.5");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveServerConfig).toHaveBeenCalled());
    const arg = vi.mocked(api.saveServerConfig).mock.lastCall![0];
    expect(arg.tls.acme.register_hostname).toBe(true);
    expect(arg.tls.acme.hostname_ip).toBe("203.0.113.5");
  });

  it("renders the ACME status panel with cert metadata and hostname error", async () => {
    vi.mocked(api.fetchServerConfig).mockResolvedValue({
      ...acmeBase,
      tls_certificate_info: {
        subject: "CN=app.example.com",
        issuer: "CN=Let's Encrypt",
        dns_names: ["app.example.com"],
        not_before: "2026-06-01T00:00:00Z",
        not_after: "2026-08-30T00:00:00Z",
      },
      acme_status: {
        last_renewal: "2026-06-01T00:00:00Z",
        last_error: "",
        hostname_error: "no IPv4 detectable",
      },
    } as never);
    render(<AdminServerPage />);
    await waitFor(() => screen.getByText("Server & TLS"));
    expect(screen.getByText("CN=app.example.com")).toBeInTheDocument();
    expect(screen.getByText(/last renewal/i)).toBeInTheDocument();
    expect(screen.getByText(/no IPv4 detectable/)).toBeInTheDocument();
  });
});
