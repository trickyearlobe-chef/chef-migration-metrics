// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import * as api from "../api";
import { AdminAuthPage } from "./AdminAuthPage";

vi.mock("../api");

const mockAuthConfig = {
  providers: [{ type: "local" }],
  session_expiry: "24h",
  min_password_length: 8,
  lockout_attempts: 5,
};

const mockSAMLAuthConfig = {
  providers: [{ type: "saml", sp_entity_id: "https://app.example.com/saml" }],
  session_expiry: "24h",
  min_password_length: 8,
  lockout_attempts: 5,
};

const mockCertResponse = {
  certificate_pem: "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n",
  fingerprint_sha256: "abcdef1234567890abcdef1234567890",
  not_after: "2036-01-01T00:00:00Z",
  subject: "https://app.example.com/saml",
};

describe("AdminAuthPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockAuthConfig as never);
    vi.mocked(api.fetchSAMLCertificate).mockResolvedValue(null);
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

  it("shows generate cert button when SAML provider exists", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Generate SP Certificate" })).toBeInTheDocument(),
    );
  });

  it("shows IdP metadata source dropdown defaulting to URL", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("button", { name: "Generate SP Certificate" }),
    );
    expect(screen.getByRole("option", { name: "Paste XML" })).toBeInTheDocument();
    // Default source is URL → URL input visible, no paste textarea.
    expect(
      screen.getByPlaceholderText("https://idp.example.com/metadata.xml"),
    ).toBeInTheDocument();
  });

  it("switches IdP metadata source to paste XML", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("button", { name: "Generate SP Certificate" }),
    );
    fireEvent.change(screen.getByDisplayValue("Fetch from URL"), {
      target: { value: "xml" },
    });
    expect(screen.getByPlaceholderText(/EntityDescriptor/)).toBeInTheDocument();
    expect(
      screen.queryByPlaceholderText("https://idp.example.com/metadata.xml"),
    ).not.toBeInTheDocument();
  });

  it("defaults the SP Base URL field to the browser origin", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(
        screen.getByDisplayValue(window.location.origin),
      ).toBeInTheDocument(),
    );
  });

  it("does not show cert section when no SAML provider", async () => {
    render(<AdminAuthPage />);
    await waitFor(() => screen.getByText("Authentication"));
    expect(screen.queryByText("SAML SP Certificate")).not.toBeInTheDocument();
  });

  it("shows existing cert when fetched", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.fetchSAMLCertificate).mockResolvedValue(mockCertResponse);
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Regenerate SP Certificate" })).toBeInTheDocument(),
    );
    expect(screen.getByDisplayValue(/BEGIN CERTIFICATE/)).toBeInTheDocument();
  });

  it("calls generateSAMLKeypair on button click", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.generateSAMLKeypair).mockResolvedValue(mockCertResponse);
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Generate SP Certificate" })).toBeInTheDocument(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Generate SP Certificate" }));
    await waitFor(() =>
      expect(api.generateSAMLKeypair).toHaveBeenCalledTimes(1),
    );
    expect(screen.getByDisplayValue(/BEGIN CERTIFICATE/)).toBeInTheDocument();
  });

  it("shows Export SP Metadata button when SAML provider exists", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Export SP Metadata (XML)" }),
      ).toBeInTheDocument(),
    );
  });

  it("does not show Export SP Metadata button without a SAML provider", async () => {
    render(<AdminAuthPage />);
    await waitFor(() => screen.getByText("Authentication"));
    expect(
      screen.queryByRole("button", { name: "Export SP Metadata (XML)" }),
    ).not.toBeInTheDocument();
  });

  it("downloads metadata XML on Export click", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.fetchSAMLMetadata).mockResolvedValue(
      '<?xml version="1.0"?><EntityDescriptor/>',
    );
    const createObjectURL = vi.fn(() => "blob:metadata");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", { createObjectURL, revokeObjectURL });
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});

    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("button", { name: "Export SP Metadata (XML)" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Export SP Metadata (XML)" }));

    await waitFor(() => expect(api.fetchSAMLMetadata).toHaveBeenCalledTimes(1));
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(clickSpy).toHaveBeenCalledTimes(1);
    expect(revokeObjectURL).toHaveBeenCalledTimes(1);

    clickSpy.mockRestore();
    vi.unstubAllGlobals();
  });

  it("shows an error message when metadata export fails", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.fetchSAMLMetadata).mockRejectedValue(
      new Error("SAML provider not initialised"),
    );
    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("button", { name: "Export SP Metadata (XML)" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Export SP Metadata (XML)" }));

    await waitFor(() =>
      expect(screen.getByText(/SAML provider not initialised/)).toBeInTheDocument(),
    );
  });

  it("shows the metadata URL for IdPs that fetch it directly", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.samlMetadataUrl).mockReturnValue(
      "https://app.example.com/api/v1/auth/saml/metadata",
    );
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(
        screen.getByDisplayValue("https://app.example.com/api/v1/auth/saml/metadata"),
      ).toBeInTheDocument(),
    );
  });

  it("copies the metadata URL to the clipboard", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.samlMetadataUrl).mockReturnValue(
      "https://app.example.com/api/v1/auth/saml/metadata",
    );
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });

    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("button", { name: "Copy URL" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Copy URL" }));

    expect(writeText).toHaveBeenCalledWith(
      "https://app.example.com/api/v1/auth/saml/metadata",
    );
    vi.unstubAllGlobals();
  });

  it("surfaces the backend-computed ACS (callback) URL with a copy button", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.fetchSAMLEndpoints).mockResolvedValue({
      acs_url: "https://cmm.example.com/api/v1/auth/saml/acs",
      slo_url: "https://cmm.example.com/api/v1/auth/saml/slo",
      metadata_url: "https://cmm.example.com/api/v1/auth/saml/metadata",
      entity_id: "https://cmm.example.com",
    });
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });

    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(
        screen.getByDisplayValue("https://cmm.example.com/api/v1/auth/saml/acs"),
      ).toBeInTheDocument(),
    );
    // The callback URL field has its own copy control.
    fireEvent.click(screen.getAllByRole("button", { name: "Copy" })[0]);
    expect(writeText).toHaveBeenCalledWith(
      "https://cmm.example.com/api/v1/auth/saml/acs",
    );
    vi.unstubAllGlobals();
  });

  it("shows Sign AuthnRequests checkbox for a SAML provider", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(
        screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }),
      ).toBeInTheDocument(),
    );
    // Defaults to unchecked when sign_requests is absent.
    expect(
      screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }),
    ).not.toBeChecked();
  });

  it("does not show SAML option checkboxes for a local provider", async () => {
    render(<AdminAuthPage />);
    await waitFor(() => screen.getByText("Authentication"));
    expect(
      screen.queryByRole("checkbox", { name: /Sign AuthnRequests/ }),
    ).not.toBeInTheDocument();
  });

  it("reflects sign_requests=true as checked", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue({
      ...mockSAMLAuthConfig,
      providers: [{ type: "saml", sign_requests: true }],
    } as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(
        screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }),
      ).toBeChecked(),
    );
  });

  it("reflects debug_log_assertions=true as checked", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue({
      ...mockSAMLAuthConfig,
      providers: [{ type: "saml", debug_log_assertions: true }],
    } as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(
        screen.getByRole("checkbox", { name: /Log decrypted assertions/ }),
      ).toBeChecked(),
    );
  });

  it("toggling Log decrypted assertions enables Save", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("checkbox", { name: /Log decrypted assertions/ }),
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    fireEvent.click(
      screen.getByRole("checkbox", { name: /Log decrypted assertions/ }),
    );
    expect(
      screen.getByRole("checkbox", { name: /Log decrypted assertions/ }),
    ).toBeChecked();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("toggling Sign AuthnRequests enables Save", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }),
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    fireEvent.click(screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }));
    expect(screen.getByRole("checkbox", { name: /Sign AuthnRequests/ })).toBeChecked();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  // --- Issue 1: SP base URL save should not falsely demand a restart and must
  // refresh the backend-computed ACS/SLO/entity copy fields. ---

  it("page subtitle does not claim auth changes require a restart", async () => {
    render(<AdminAuthPage />);
    await waitFor(() => screen.getByText("Authentication"));
    expect(
      screen.queryByText(/require an application restart/i),
    ).not.toBeInTheDocument();
  });

  it("does not show a restart banner after saving when the backend applies changes live", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.fetchSAMLEndpoints).mockResolvedValue({
      acs_url: "https://cmm.example.com/api/v1/auth/saml/acs",
      slo_url: "https://cmm.example.com/api/v1/auth/saml/slo",
      metadata_url: "https://cmm.example.com/api/v1/auth/saml/metadata",
      entity_id: "https://cmm.example.com",
    });
    vi.mocked(api.saveAuthConfig).mockResolvedValue({
      value: { ...mockSAMLAuthConfig, providers: [{ type: "saml", sign_requests: true }] },
      restartRequired: false,
    } as never);

    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }),
    );
    fireEvent.click(screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(screen.getByText("Settings saved successfully.")).toBeInTheDocument(),
    );
    expect(screen.queryByText(/Restart required/i)).not.toBeInTheDocument();
  });

  it("shows a restart banner after saving only when the backend reports restart_required", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.fetchSAMLEndpoints).mockResolvedValue({
      acs_url: "https://cmm.example.com/api/v1/auth/saml/acs",
      slo_url: "https://cmm.example.com/api/v1/auth/saml/slo",
      metadata_url: "https://cmm.example.com/api/v1/auth/saml/metadata",
      entity_id: "https://cmm.example.com",
    });
    vi.mocked(api.saveAuthConfig).mockResolvedValue({
      value: { ...mockSAMLAuthConfig, providers: [{ type: "saml", sign_requests: true }] },
      restartRequired: true,
    } as never);

    render(<AdminAuthPage />);
    await waitFor(() =>
      screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }),
    );
    fireEvent.click(screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(screen.getByText(/Restart required/i)).toBeInTheDocument(),
    );
  });

  it("re-fetches SAML endpoints after save so ACS reflects the new base URL", async () => {
    vi.mocked(api.fetchAuthConfig).mockResolvedValue(mockSAMLAuthConfig as never);
    vi.mocked(api.fetchSAMLEndpoints)
      .mockResolvedValueOnce({
        acs_url: "https://old.example.com/api/v1/auth/saml/acs",
        slo_url: "https://old.example.com/api/v1/auth/saml/slo",
        metadata_url: "https://old.example.com/api/v1/auth/saml/metadata",
        entity_id: "https://old.example.com",
      })
      .mockResolvedValueOnce({
        acs_url: "https://new.example.com/api/v1/auth/saml/acs",
        slo_url: "https://new.example.com/api/v1/auth/saml/slo",
        metadata_url: "https://new.example.com/api/v1/auth/saml/metadata",
        entity_id: "https://new.example.com",
      });
    vi.mocked(api.saveAuthConfig).mockResolvedValue({
      value: { ...mockSAMLAuthConfig, providers: [{ type: "saml", sign_requests: true }] },
      restartRequired: false,
    } as never);

    render(<AdminAuthPage />);
    await waitFor(() =>
      expect(
        screen.getByDisplayValue("https://old.example.com/api/v1/auth/saml/acs"),
      ).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole("checkbox", { name: /Sign AuthnRequests/ }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    // After save the ACS copy field reflects the freshly-fetched base URL.
    await waitFor(() =>
      expect(
        screen.getByDisplayValue("https://new.example.com/api/v1/auth/saml/acs"),
      ).toBeInTheDocument(),
    );
    expect(
      screen.queryByDisplayValue("https://old.example.com/api/v1/auth/saml/acs"),
    ).not.toBeInTheDocument();
  });
});
