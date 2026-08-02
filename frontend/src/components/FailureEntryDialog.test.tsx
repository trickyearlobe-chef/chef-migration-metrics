// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return { ...actual, fetchGitRepos: vi.fn() };
});

import { FailureEntryDialog } from "./FailureEntryDialog";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

// ---------------------------------------------------------------------------
// Who is on it
//
// The rule is that half a commitment cannot be chased: a reference with no
// kind, or a kind with no reference. The rule is right; making somebody
// satisfy it by hand after they have already typed the reference is not.
// ---------------------------------------------------------------------------

describe("FailureEntryDialog — who is on it", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 8, total_items: 0, total_pages: 0 },
    });
  });

  function renderRecording(onSubmit = vi.fn().mockResolvedValue(undefined)) {
    render(
      <FailureEntryDialog
        mode="record"
        fixedRepo={{ name: "cron", cookbookName: "cron" }}
        onCancel={vi.fn()}
        onSubmit={onSubmit}
      />,
      { wrapper: Wrapper },
    );
    return onSubmit;
  }

  // Typing a reference is saying somebody is on it. The kind should follow,
  // visibly, rather than being demanded afterwards.
  it("assumes a reference is a ticket rather than refusing it", async () => {
    const user = userEvent.setup();
    const onSubmit = renderRecording();

    await user.type(screen.getByLabelText(/^Reason/i), "Bogus failure");
    await user.type(screen.getByLabelText(/^Reference/i), "Jira 999");

    // Visible in the control, so it can be corrected — not applied silently
    // at submit time.
    expect(screen.getByLabelText(/Who is on it/i)).toHaveValue("ticket");

    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          holder_type: "ticket",
          holder_ref: "Jira 999",
        }),
      );
    });
  });

  // Somebody who means an owner can still say so, and the guess must not
  // overwrite a choice already made.
  it("does not overwrite a kind that was chosen deliberately", async () => {
    const user = userEvent.setup();
    const onSubmit = renderRecording();

    await user.type(screen.getByLabelText(/^Reason/i), "Bogus failure");
    await user.selectOptions(screen.getByLabelText(/Who is on it/i), "owner");
    await user.type(screen.getByLabelText(/^Reference/i), "platform-team");

    expect(screen.getByLabelText(/Who is on it/i)).toHaveValue("owner");

    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          holder_type: "owner",
          holder_ref: "platform-team",
        }),
      );
    });
  });

  // A kind with no reference carries no information, so it is nobody on it —
  // not an error to argue with.
  it("treats a kind with no reference as nobody on it", async () => {
    const user = userEvent.setup();
    const onSubmit = renderRecording();

    await user.type(screen.getByLabelText(/^Reason/i), "Bogus failure");
    await user.selectOptions(screen.getByLabelText(/Who is on it/i), "ticket");

    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled();
    });
    const body = onSubmit.mock.calls[0][0];
    expect(body.holder_type).toBeUndefined();
    expect(body.holder_ref).toBeUndefined();
  });

  // Recording a failure before anybody is on it is the normal case, and must
  // not be obstructed.
  it("records with nobody on it at all", async () => {
    const user = userEvent.setup();
    const onSubmit = renderRecording();

    await user.type(screen.getByLabelText(/^Reason/i), "Bogus failure");
    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled();
    });
    expect(onSubmit.mock.calls[0][0].holder_ref).toBeUndefined();
  });

  // The whole point: filling the form in the obvious order no longer produces
  // an error somebody has to decode.
  it("does not complain about a half a holder", async () => {
    const user = userEvent.setup();
    renderRecording();

    await user.type(screen.getByLabelText(/^Reason/i), "Bogus failure");
    await user.type(screen.getByLabelText(/^Reference/i), "Jira 999");
    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    expect(
      screen.queryByText(/needs both what kind it is/i),
    ).not.toBeInTheDocument();
  });
});
