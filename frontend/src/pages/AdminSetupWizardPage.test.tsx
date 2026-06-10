// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import { AdminSetupWizardPage } from "./AdminSetupWizardPage";

vi.mock("../api");

const mockCredentials = {
  data: [
    {
      name: "my-key",
      credential_type: "chef_client_key",
      created_by: "admin",
      created_at: "",
      updated_at: "",
    },
  ],
  pagination: { page: 1, per_page: 50, total: 1, total_pages: 1 },
};

function renderWizard() {
  return render(
    <MemoryRouter>
      <AdminSetupWizardPage />
    </MemoryRouter>,
  );
}

describe("AdminSetupWizardPage — completion clears setup mode without a restart", () => {
  const realLocation = window.location;
  let assign: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.mocked(api.fetchCredentials).mockResolvedValue(mockCredentials as never);
    vi.mocked(api.saveConfigOrganisations).mockResolvedValue(undefined as never);
    // jsdom's window.location.assign is non-configurable, so swap the whole
    // object for a minimal stub we can assert against.
    assign = vi.fn();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { assign },
    });
  });

  afterEach(() => {
    Object.defineProperty(window, "location", {
      configurable: true,
      value: realLocation,
    });
    vi.restoreAllMocks();
  });

  // Regression for issue #1: after saving the first org, "Go to Dashboard" must
  // do a full-page load, not a soft client-side navigate. SetupModeGuard's
  // useSetupRequired runs once on mount and stays mounted across SPA route
  // changes, so a soft navigate to "/" is bounced straight back to the wizard
  // (its setupRequired state is still stale). A full load re-initialises the
  // guard (and the org list) against the now-populated config — no app restart.
  it("triggers a full page load to the dashboard on completion", async () => {
    const user = userEvent.setup();
    renderWizard();

    await user.click(screen.getByRole("button", { name: /get started/i }));
    await user.click(screen.getByRole("button", { name: /continue/i }));

    // Organisation step: wait for the credential dropdown to load.
    await waitFor(() => screen.getByRole("combobox"));

    await user.type(
      screen.getByPlaceholderText(/e\.g\. production/i),
      "production",
    );
    await user.type(
      screen.getByPlaceholderText(/organizations\/myorg/i),
      "https://chef.example.com/organizations/myorg",
    );
    await user.selectOptions(screen.getByRole("combobox"), "my-key");

    await user.click(
      screen.getByRole("button", { name: /save organisation/i }),
    );

    // Done step.
    await waitFor(() => screen.getByText(/setup complete/i));
    await user.click(screen.getByRole("button", { name: /go to dashboard/i }));

    expect(assign).toHaveBeenCalledWith("/");
  });
});
