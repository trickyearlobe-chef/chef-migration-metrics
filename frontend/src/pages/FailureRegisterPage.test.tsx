// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import type { FailureRegisterResponse } from "../types";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchFailureRegister: vi.fn(),
    fetchGitRepos: vi.fn(),
    fetchCookbooks: vi.fn(),
    recordFailureVerdict: vi.fn(),
    resolveFailureEntry: vi.fn(),
    reviseFailureEntry: vi.fn(),
  };
});

const mockUseAuth = vi.fn();
vi.mock("../context/AuthContext", () => ({ useAuth: () => mockUseAuth() }));

import { FailureRegisterPage } from "./FailureRegisterPage";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

const brokenEntry = {
  id: "entry-1",
  subject_name: "acme-nginx-cookbook",
  cookbook_name: "nginx",
  subject_type: "git_repo" as const,
  verdict: "broken" as const,
  reason: "the service resource fails on a real converge",
  plan: "rewrite the template and re-release",
  holder_type: "ticket" as const,
  holder_ref: "PLAT-4821",
  target_date: "2026-09-30",
  status: "open" as const,
  raised_by: "alice",
  raised_at: "2026-08-01T09:00:00Z",
  updated_at: "2026-08-01T09:00:00Z",
};

function response(
  overrides: Partial<FailureRegisterResponse> = {},
): FailureRegisterResponse {
  return {
    data: [brokenEntry],
    pagination: { page: 1, per_page: 25, total_items: 1, total_pages: 1 },
    summary: {
      open: 9,
      open_broken: 7,
      open_not_broken: 2,
      open_without_holder: 3,
      open_overdue: 1,
      window_days: 7,
      raised_in_window: 4,
      resolved_in_window: 1,
      total_broken: 11,
      total_not_broken: 3,
      resolved: 5,
    },
    ...overrides,
  };
}

describe("FailureRegisterPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchFailureRegister).mockResolvedValue(response());
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: [
        {
          id: "1",
          name: "acme-mysql-cookbook",
          git_repo_url: "https://git.example.com/acme/acme-mysql-cookbook.git",
          has_test_suite: true,
          clone_status: "ok",
        },
      ],
      pagination: { page: 1, per_page: 8, total_items: 1, total_pages: 1 },
    });
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 32, total_items: 0, total_pages: 0 },
    });
  });

  // Journey 6: which repos are broken, why, and what is being done — all
  // readable at a glance rather than a click away.
  it("shows what is broken, why, and what is being done about it", async () => {
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    // Labelled with the cookbook, because that is what standup says.
    expect(await screen.findByText("nginx")).toBeInTheDocument();
    // Keyed on the repo, because that is where the fix is made.
    expect(screen.getByText("acme-nginx-cookbook")).toBeInTheDocument();
    // Why.
    expect(
      screen.getByText(/the service resource fails on a real converge/),
    ).toBeInTheDocument();
    // What is being done, and who is on it.
    expect(
      screen.getByText(/rewrite the template and re-release/),
    ).toBeInTheDocument();
    expect(screen.getByText("PLAT-4821")).toBeInTheDocument();
  });

  // The size and the direction matter as much as the contents: a register that
  // is growing is a different message from one that is shrinking.
  it("says whether the register is growing or shrinking", async () => {
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    const direction = await screen.findByTestId("register-direction");
    expect(direction).toHaveTextContent("growing");
    expect(direction).toHaveTextContent("4 raised");
    expect(direction).toHaveTextContent("1 resolved");
  });

  it("reports a shrinking register when more is resolved than raised", async () => {
    vi.mocked(api.fetchFailureRegister).mockResolvedValue(
      response({
        summary: {
          ...response().summary!,
          raised_in_window: 1,
          resolved_in_window: 5,
          open_overdue: 0,
        },
      }),
    );
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    expect(await screen.findByTestId("register-direction")).toHaveTextContent(
      "shrinking",
    );
  });

  // The two sides are the accuracy report of the automated signals, and have
  // to be told apart at a glance.
  it("distinguishes a failure the tools missed from a verdict they got wrong", async () => {
    vi.mocked(api.fetchFailureRegister).mockResolvedValue(
      response({
        data: [
          brokenEntry,
          {
            ...brokenEntry,
            id: "entry-2",
            subject_name: "acme-apache-cookbook",
            cookbook_name: "apache",
            verdict: "not_broken",
            reason: "kitchen never converged; this runs on 4000 nodes today",
          },
        ],
      }),
    );
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    expect(await screen.findByText("Broken")).toBeInTheDocument();
    expect(screen.getByText("Not broken")).toBeInTheDocument();
  });

  // Who is on it is its own standup question — "what is being done" and "who
  // is doing it" get different answers, and trailing the holder after a long
  // plan makes it unscannable.
  it("gives who is on it a column of its own", async () => {
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    const header = await screen.findByRole("columnheader", {
      name: /Who.s on it/i,
    });
    expect(header).toBeInTheDocument();

    const cells = screen.getAllByRole("cell");
    const holderCell = cells.find((c) => c.textContent?.includes("PLAT-4821"));
    expect(holderCell).toBeDefined();
    // The plan lives in a different cell — that is the point of the split.
    expect(holderCell?.textContent).not.toContain(
      "rewrite the template and re-release",
    );
  });

  // An entry nobody has been put on is a standup question in its own right.
  it("points out entries with no plan and nobody on them", async () => {
    vi.mocked(api.fetchFailureRegister).mockResolvedValue(
      response({
        data: [
          {
            ...brokenEntry,
            plan: undefined,
            holder_type: undefined,
            holder_ref: undefined,
            target_date: undefined,
          },
        ],
      }),
    );
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    expect(await screen.findByText(/No plan yet/)).toBeInTheDocument();
    expect(screen.getByText(/Nobody on it/)).toBeInTheDocument();
  });

  // Journey 4: recording a failure nobody predicted.
  it("records a verdict with its reason", async () => {
    const user = userEvent.setup();
    vi.mocked(api.recordFailureVerdict).mockResolvedValue({
      ...brokenEntry,
      id: "entry-new",
    });

    render(<FailureRegisterPage />, { wrapper: Wrapper });
    await user.click(await screen.findByRole("button", { name: /Record a failure/i }));

    // The repo is chosen from the catalogue, not typed: a name that matches
    // no repo would be stored and change nobody's readiness.
    await user.type(screen.getByLabelText(/Repo or cookbook/i), "acme-mysql");
    await user.click(
      await screen.findByRole("button", { name: /acme-mysql-cookbook/i }),
    );
    await user.clear(screen.getByLabelText(/^Cookbook/i));
    await user.type(screen.getByLabelText(/^Cookbook/i), "mysql");
    await user.type(
      screen.getByLabelText(/^Reason/i),
      "the service never starts on RHEL 9",
    );
    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(api.recordFailureVerdict).toHaveBeenCalledWith(
        expect.objectContaining({
          subject_name: "acme-mysql-cookbook",
          cookbook_name: "mysql",
          verdict: "broken",
          reason: "the service never starts on RHEL 9",
        }),
      );
    });
  });

  // A verdict with no reason is an opinion. The form must not offer to send one.
  it("will not submit a verdict without a reason", async () => {
    const user = userEvent.setup();
    render(<FailureRegisterPage />, { wrapper: Wrapper });
    await user.click(await screen.findByRole("button", { name: /Record a failure/i }));

    await user.type(screen.getByLabelText(/Repo or cookbook/i), "acme-mysql");
    await user.click(
      await screen.findByRole("button", { name: /acme-mysql-cookbook/i }),
    );

    expect(
      screen.getByRole("button", { name: /Record this verdict/i }),
    ).toBeDisabled();
  });

  // The hole the picker closes: a verdict against a repo that does not exist
  // is stored, shown, and silently changes nobody's readiness.
  it("will not submit a repo name that was typed rather than chosen", async () => {
    const user = userEvent.setup();
    render(<FailureRegisterPage />, { wrapper: Wrapper });
    await user.click(await screen.findByRole("button", { name: /Record a failure/i }));

    await user.type(screen.getByLabelText(/Repo or cookbook/i), "a-repo-that-is-not-real");
    await user.type(screen.getByLabelText(/^Cookbook/i), "whatever");
    await user.type(screen.getByLabelText(/^Reason/i), "it broke");

    expect(
      screen.getByRole("button", { name: /Record this verdict/i }),
    ).toBeDisabled();
  });

  // Nothing in the catalogue matches, and the reason says why rather than
  // showing an empty box.
  it("says why nothing matched when the catalogue has no such repo", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 8, total_items: 0, total_pages: 0 },
    });
    render(<FailureRegisterPage />, { wrapper: Wrapper });
    await user.click(await screen.findByRole("button", { name: /Record a failure/i }));

    await user.type(screen.getByLabelText(/Repo or cookbook/i), "nonesuch");

    expect(
      await screen.findByText(/Only repos and cookbooks CMM has collected/i),
    ).toBeInTheDocument();
  });

  // A reversal is a new verdict, and the form has to say the old one survives.
  it("records a reversal as a new verdict and says the old one is kept", async () => {
    const user = userEvent.setup();
    vi.mocked(api.recordFailureVerdict).mockResolvedValue(brokenEntry);

    render(<FailureRegisterPage />, { wrapper: Wrapper });
    await user.click(
      await screen.findByRole("button", { name: /Change verdict/i }),
    );

    expect(
      screen.getByText(/is kept and marked overturned/i),
    ).toBeInTheDocument();

    // It opens on the opposite verdict, which is what a reversal is for.
    expect(screen.getByRole("radio", { name: /This is not broken/i })).toBeChecked();

    await user.type(
      screen.getByLabelText(/^Reason/i),
      "the offence is a false positive",
    );
    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(api.recordFailureVerdict).toHaveBeenCalledWith(
        expect.objectContaining({
          subject_name: "acme-nginx-cookbook",
          verdict: "not_broken",
        }),
      );
    });
  });

  // Resolution is recorded, not deleted.
  it("resolves an entry", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "prompt").mockReturnValue("fixed in 4.2.0");
    vi.mocked(api.resolveFailureEntry).mockResolvedValue({
      ...brokenEntry,
      status: "resolved",
    });

    render(<FailureRegisterPage />, { wrapper: Wrapper });
    await user.click(await screen.findByRole("button", { name: /^Resolve$/i }));

    await waitFor(() => {
      expect(api.resolveFailureEntry).toHaveBeenCalledWith(
        "entry-1",
        "fixed in 4.2.0",
      );
    });
  });

  // The default read is what is standing, and what is broken: the standup
  // asks what is broken, and an entry overruling a wrong automated verdict is
  // the accuracy report rather than the agenda.
  it("opens on the standing, broken entries", async () => {
    render(<FailureRegisterPage />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(api.fetchFailureRegister).toHaveBeenCalledWith(
        expect.objectContaining({ status: "open", verdict: "broken" }),
      );
    });
  });

  // The overruled entries are one selection away, not buried.
  it("can show both sides", async () => {
    const user = userEvent.setup();
    render(<FailureRegisterPage />, { wrapper: Wrapper });
    await waitFor(() => expect(api.fetchFailureRegister).toHaveBeenCalled());

    await user.selectOptions(screen.getByLabelText(/Verdict/i), "");

    await waitFor(() => {
      const last = vi.mocked(api.fetchFailureRegister).mock.calls.at(-1)?.[0];
      expect(last?.verdict).toBeUndefined();
    });
  });

  // Somebody who cannot record anything must not be offered the buttons.
  it("hides the write actions from a viewer", async () => {
    mockUseAuth.mockReturnValue({
      isOperator: false,
      isAdmin: false,
      user: { role: "viewer", username: "test" },
    });
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    expect(await screen.findByText("nginx")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Record a failure/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /^Resolve$/i }),
    ).not.toBeInTheDocument();
  });

  // An empty register says nothing is broken, which is different from a
  // failure to load.
  it("says the register is empty rather than showing nothing", async () => {
    vi.mocked(api.fetchFailureRegister).mockResolvedValue(
      response({ data: [] }),
    );
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    expect(
      await screen.findByText(/Nothing is on the register/i),
    ).toBeInTheDocument();
  });

  // The summary failing must not take the list with it.
  it("still lists what is broken when the summary is missing", async () => {
    vi.mocked(api.fetchFailureRegister).mockResolvedValue(
      response({ summary: undefined }),
    );
    render(<FailureRegisterPage />, { wrapper: Wrapper });

    expect(await screen.findByText("nginx")).toBeInTheDocument();
    expect(screen.queryByTestId("register-direction")).not.toBeInTheDocument();
  });
});
