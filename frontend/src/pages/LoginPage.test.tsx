// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import * as api from "../api";

vi.mock("../api");

const mockUseAuth = vi.fn();

vi.mock("../context/AuthContext", () => ({
  useAuth: () => mockUseAuth(),
}));

import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      login: vi.fn(),
      error: null,
      loggingIn: false,
    });
  });

  it("shows SSO button when SAML is enabled", async () => {
    vi.mocked(api.fetchAuthInfo).mockResolvedValue({
      local_enabled: true,
      saml_enabled: true,
    });

    render(<LoginPage />);

    expect(
      await screen.findByRole("button", { name: "Sign in with SSO" }),
    ).toBeInTheDocument();
  });

  it("hides SSO button when SAML is not enabled", async () => {
    vi.mocked(api.fetchAuthInfo).mockResolvedValue({
      local_enabled: true,
      saml_enabled: false,
    });

    render(<LoginPage />);

    await waitFor(() => {
      expect(api.fetchAuthInfo).toHaveBeenCalledTimes(1);
      expect(
        screen.queryByRole("button", { name: "Sign in with SSO" }),
      ).not.toBeInTheDocument();
    });
  });

  it("shows local login form when local is enabled", async () => {
    vi.mocked(api.fetchAuthInfo).mockResolvedValue({
      local_enabled: true,
      saml_enabled: false,
    });

    render(<LoginPage />);

    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
  });

  it("shows both when both enabled", async () => {
    vi.mocked(api.fetchAuthInfo).mockResolvedValue({
      local_enabled: true,
      saml_enabled: true,
    });

    render(<LoginPage />);

    expect(
      await screen.findByRole("button", { name: "Sign in with SSO" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByText("or")).toBeInTheDocument();
  });

  it("SSO button is a link to SAML login endpoint", async () => {
    vi.mocked(api.fetchAuthInfo).mockResolvedValue({
      local_enabled: false,
      saml_enabled: true,
    });

    render(<LoginPage />);

    expect(
      await screen.findByRole("button", { name: "Sign in with SSO" }),
    ).toBeInTheDocument();
  });
});
