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
});
