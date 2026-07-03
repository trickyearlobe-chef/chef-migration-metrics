// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api");

vi.mock("../context/GlobalFilterContext", () => ({
  useGlobalFilters: () => ({
    targetChefVersion: "18.5.0",
    targetVersions: ["18.5.0"],
    setTargetChefVersion: vi.fn(),
    staleTiers: [],
    setStaleTiers: vi.fn(),
    versionsLoading: false,
  }),
}));

import { AdminCustomCopsSection } from "./AdminCustomCopsSection";

function renderSection() {
  return render(
    <MemoryRouter>
      <AdminCustomCopsSection />
    </MemoryRouter>,
  );
}

describe("AdminCustomCopsSection", () => {
  beforeEach(() => {
    vi.mocked(api.fetchCustomCops).mockResolvedValue({
      data: [
        {
          id: "uuid-1",
          cop_name: "Custom/Ruby3/NilMatch",
          description: "nil.=~ removed in Ruby 3",
          pattern_type: "regex",
          pattern: "=~",
          file_glob: "*.rb",
          classification: "blocker",
          enabled: true,
          created_at: "2026-06-20T10:00:00Z",
          updated_at: "2026-06-20T10:00:00Z",
        },
      ],
    });
  });

  it("renders the list of custom cops", async () => {
    renderSection();
    await waitFor(() =>
      expect(screen.getByText("Custom/Ruby3/NilMatch")).toBeInTheDocument(),
    );
    expect(screen.getByText(/nil\.=~ removed/)).toBeInTheDocument();
  });

  it("shows the add button", async () => {
    renderSection();
    await waitFor(() =>
      expect(screen.getByText("+ Add Custom Cop")).toBeInTheDocument(),
    );
  });

  it("opens the form when Add is clicked", async () => {
    renderSection();
    await waitFor(() =>
      expect(screen.getByText("+ Add Custom Cop")).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByText("+ Add Custom Cop"));

    expect(screen.getByText("New Custom Cop")).toBeInTheDocument();
  });

  it("shows empty state when no cops exist", async () => {
    vi.mocked(api.fetchCustomCops).mockResolvedValue({ data: [] });
    renderSection();
    await waitFor(() =>
      expect(screen.getByText("No custom cops defined yet.")).toBeInTheDocument(),
    );
  });
});
