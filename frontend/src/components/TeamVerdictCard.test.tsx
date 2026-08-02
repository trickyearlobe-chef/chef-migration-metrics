// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import type { FailureRegisterEntry } from "../types";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchFailureRegisterHistory: vi.fn(),
    recordFailureVerdict: vi.fn(),
    resolveFailureEntry: vi.fn(),
  };
});

const mockUseAuth = vi.fn();
vi.mock("../context/AuthContext", () => ({ useAuth: () => mockUseAuth() }));

import { TeamVerdictCard } from "./TeamVerdictCard";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

const standing: FailureRegisterEntry = {
  id: "entry-1",
  subject_name: "acme-nginx-cookbook",
  cookbook_name: "nginx",
  subject_type: "git_repo",
  verdict: "not_broken",
  reason: "kitchen never converged; this runs on 4000 nodes today",
  status: "open",
  raised_by: "bob",
  raised_at: "2026-08-01T09:00:00Z",
  updated_at: "2026-08-01T09:00:00Z",
};

describe("TeamVerdictCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchFailureRegisterHistory).mockResolvedValue({ data: [] });
  });

  // The whole reason for putting it here: you record the verdict where the
  // evidence for it is, rather than typing a repo name somewhere else.
  it("offers to record a verdict when nobody has", async () => {
    render(<TeamVerdictCard gitRepoName="acme-nginx-cookbook" />, {
      wrapper: Wrapper,
    });

    expect(
      await screen.findByText(/Nobody has recorded a verdict/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Report this as broken/i }),
    ).toBeInTheDocument();
  });

  // Recording from the repo's own page cannot name the wrong repo, because
  // there is nothing to type.
  it("records against this repo without asking which one", async () => {
    const user = userEvent.setup();
    vi.mocked(api.recordFailureVerdict).mockResolvedValue(standing);

    render(
      <TeamVerdictCard gitRepoName="acme-nginx-cookbook" cookbookName="nginx" />,
      { wrapper: Wrapper },
    );
    await user.click(
      await screen.findByRole("button", { name: /Report this as broken/i }),
    );

    // The subject is fixed and not editable.
    const repoField = screen.getByLabelText(/Repo or cookbook/i);
    expect(repoField).toHaveValue("acme-nginx-cookbook");
    expect(repoField).toHaveAttribute("readonly");

    await user.type(screen.getByLabelText(/^Reason/i), "breaks on converge");
    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(api.recordFailureVerdict).toHaveBeenCalledWith(
        expect.objectContaining({
          subject_name: "acme-nginx-cookbook",
          cookbook_name: "nginx",
          reason: "breaks on converge",
        }),
      );
    });
  });

  // Sitting beside the CookStyle card, it has to say plainly that it wins —
  // otherwise the two cards just look like they disagree.
  it("says an overruling verdict beats the scans beside it", async () => {
    vi.mocked(api.fetchFailureRegisterHistory).mockResolvedValue({
      data: [standing],
    });
    render(<TeamVerdictCard gitRepoName="acme-nginx-cookbook" />, {
      wrapper: Wrapper,
    });

    expect(await screen.findByText("Not broken")).toBeInTheDocument();
    expect(
      screen.getByText(/This overrules the scans above/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/kitchen never converged/i),
    ).toBeInTheDocument();
  });

  it("says a broken verdict blocks whatever the scans say", async () => {
    vi.mocked(api.fetchFailureRegisterHistory).mockResolvedValue({
      data: [{ ...standing, verdict: "broken", reason: "fails on converge" }],
    });
    render(<TeamVerdictCard gitRepoName="acme-nginx-cookbook" />, {
      wrapper: Wrapper,
    });

    expect(await screen.findByText("Broken")).toBeInTheDocument();
    expect(
      screen.getByText(/blocked whatever the scans above say/i),
    ).toBeInTheDocument();
  });

  // Verdicts are superseded, never replaced — and the losing one has to stay
  // readable on the repo it was about.
  it("keeps earlier verdicts readable", async () => {
    vi.mocked(api.fetchFailureRegisterHistory).mockResolvedValue({
      data: [
        standing,
        {
          ...standing,
          id: "entry-0",
          verdict: "broken",
          reason: "thought it was the template",
          status: "superseded",
          raised_by: "alice",
        },
      ],
    });
    render(<TeamVerdictCard gitRepoName="acme-nginx-cookbook" />, {
      wrapper: Wrapper,
    });

    expect(await screen.findByText(/1 earlier verdict/i)).toBeInTheDocument();
    expect(
      screen.getByText(/thought it was the template/i),
    ).toBeInTheDocument();
  });

  it("hides the write actions from a viewer", async () => {
    mockUseAuth.mockReturnValue({
      isOperator: false,
      isAdmin: false,
      user: { role: "viewer", username: "test" },
    });
    vi.mocked(api.fetchFailureRegisterHistory).mockResolvedValue({
      data: [standing],
    });
    render(<TeamVerdictCard gitRepoName="acme-nginx-cookbook" />, {
      wrapper: Wrapper,
    });

    expect(await screen.findByText("Not broken")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Change verdict/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Resolve/i }),
    ).not.toBeInTheDocument();
  });

  // The card failing must not take the repo page with it.
  it("reports its own failure without breaking the page", async () => {
    vi.mocked(api.fetchFailureRegisterHistory).mockRejectedValue(
      new Error("nope"),
    );
    render(<TeamVerdictCard gitRepoName="acme-nginx-cookbook" />, {
      wrapper: Wrapper,
    });

    expect(await screen.findByTestId("team-verdict-card")).toBeInTheDocument();
    expect(screen.getByText(/nope/i)).toBeInTheDocument();
  });
});
