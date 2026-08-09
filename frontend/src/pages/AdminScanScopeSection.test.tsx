// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api");

import { AdminScanScopeSection } from "./AdminScanScopeSection";

function renderSection() {
  return render(
    <MemoryRouter>
      <AdminScanScopeSection />
    </MemoryRouter>,
  );
}

describe("AdminScanScopeSection", () => {
  beforeEach(() => {
    vi.mocked(api.fetchScanScope).mockResolvedValue({
      data: [
        {
          pattern: "Rakefile",
          excluded: true,
          reason: "A developer task runner.",
          source: "curated",
        },
        {
          pattern: "tooling/ci/*",
          excluded: true,
          reason: "Invoked only by the build job.",
          source: "operator",
          created_by: "admin",
        },
        {
          pattern: "test/*",
          excluded: false,
          reason: "Our converge loads shared helpers from here.",
          source: "operator",
          created_by: "admin",
        },
      ],
    });
    vi.mocked(api.saveScanScopeEntry).mockResolvedValue({ status: "saved" });
    vi.mocked(api.deleteScanScopeEntry).mockResolvedValue({ status: "deleted" });
  });

  // Being judged by a list you cannot read is the thing this exists to prevent,
  // so the shipped defaults are on screen next to the local decisions.
  it("shows shipped defaults and local decisions together, with their reasons", async () => {
    renderSection();

    await waitFor(() => expect(screen.getByText("Rakefile")).toBeInTheDocument());
    expect(screen.getByText("tooling/ci/*")).toBeInTheDocument();
    expect(screen.getByText("A developer task runner.")).toBeInTheDocument();
    expect(screen.getByText("Invoked only by the build job.")).toBeInTheDocument();
    expect(screen.getByText("shipped default")).toBeInTheDocument();
  });

  // An overturned default must stay visible, or nobody can find the decision to
  // reverse it.
  it("keeps an overturned default on screen, marked as counting again", async () => {
    renderSection();

    await waitFor(() => expect(screen.getByText("test/*")).toBeInTheDocument());
    expect(screen.getByText(/counts as cookbook code/i)).toBeInTheDocument();
    expect(
      screen.getByText("Our converge loads shared helpers from here."),
    ).toBeInTheDocument();
  });

  // The reason is what makes the decision checkable by somebody else, so it is
  // not optional.
  it("will not add a pattern without a reason", async () => {
    renderSection();
    await waitFor(() => expect(screen.getByText("Rakefile")).toBeInTheDocument());

    fireEvent.change(screen.getByLabelText("Pattern"), {
      target: { value: "scripts/build/*" },
    });

    const addButton = screen.getByRole("button", { name: "Add" });
    expect(addButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "Run by the build job only." },
    });
    expect(addButton).not.toBeDisabled();

    fireEvent.click(addButton);
    await waitFor(() =>
      expect(api.saveScanScopeEntry).toHaveBeenCalledWith({
        pattern: "scripts/build/*",
        excluded: true,
        reason: "Run by the build job only.",
      }),
    );
  });

  // A shipped default can only be undone by recording a decision, never by
  // quietly removing it.
  it("records a reason when somebody says a shipped default does run", async () => {
    const promptSpy = vi
      .spyOn(window, "prompt")
      .mockReturnValue("Our converge loads this.");

    renderSection();
    await waitFor(() => expect(screen.getByText("Rakefile")).toBeInTheDocument());

    const rakefileRow = screen.getByText("Rakefile").closest("tr");
    expect(rakefileRow).not.toBeNull();

    fireEvent.click(
      within(rakefileRow as HTMLElement).getByRole("button", { name: "It does run" }),
    );

    await waitFor(() =>
      expect(api.saveScanScopeEntry).toHaveBeenCalledWith({
        pattern: "Rakefile",
        excluded: false,
        reason: "Our converge loads this.",
      }),
    );
    promptSpy.mockRestore();
  });
});
