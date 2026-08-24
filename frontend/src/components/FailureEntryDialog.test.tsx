// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchGitRepos: vi.fn(),
    fetchCookbooks: vi.fn(),
    fetchOwners: vi.fn(),
  };
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
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 32, total_items: 0, total_pages: 0 },
    });
    vi.mocked(api.fetchOwners).mockResolvedValue({
      data: [
        {
          name: "thomas-smith",
          display_name: "Thomas Smith",
          contact_email: "thomas.smith@example-corp.test",
          owner_type: "individual",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      pagination: { page: 1, per_page: 8, total_items: 1, total_pages: 1 },
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
    await user.type(screen.getByLabelText(/^Reference/i), "thomas");
    await user.click(
      await screen.findByRole("button", { name: /thomas-smith/i }),
    );

    // The ticket default must not reclaim a kind somebody chose.
    expect(screen.getByLabelText(/Who is on it/i)).toHaveValue("owner");

    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          holder_type: "owner",
          holder_ref: "thomas-smith",
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

// ---------------------------------------------------------------------------
// Editing an entry
//
// Found by trying to set an assigned item back to unassigned: the reference
// alone kept the previous kind alive, so the entry could not be unassigned,
// and the target date came back as a timestamp the date input could not read
// and rendered blank.
// ---------------------------------------------------------------------------

describe("FailureEntryDialog — editing an entry", () => {
  const assigned = {
    id: "entry-1",
    subject_name: "cron",
    subject_type: "git_repo" as const,
    cookbook_name: "cron",
    verdict: "broken" as const,
    reason: "Its a totally bogus test",
    diagnosis: "theres a negative pole on the tranny-transformer",
    plan: "Get Doctor Who to fix it",
    target_date: "2026-08-30",
    holder_type: "ticket" as const,
    holder_ref: "Jira 999",
    status: "open" as const,
    raised_by: "richard",
    raised_at: "2026-08-02T09:00:00Z",
    updated_at: "2026-08-02T09:00:00Z",
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 8, total_items: 0, total_pages: 0 },
    });
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 32, total_items: 0, total_pages: 0 },
    });
    vi.mocked(api.fetchOwners).mockResolvedValue({
      data: [
        {
          name: "thomas-smith",
          display_name: "Thomas Smith",
          contact_email: "thomas.smith@example-corp.test",
          owner_type: "individual",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      pagination: { page: 1, per_page: 8, total_items: 1, total_pages: 1 },
    });
  });

  function renderRevising(onRevise = vi.fn().mockResolvedValue(undefined)) {
    render(
      <FailureEntryDialog
        mode="revise"
        entry={assigned}
        onCancel={vi.fn()}
        onRevise={onRevise}
      />,
      { wrapper: Wrapper },
    );
    return onRevise;
  }

  // A date input cannot parse a timestamp, so it renders blank and the value
  // looks lost the moment the form opens.
  it("shows the target date already on the entry", () => {
    renderRevising();
    expect(screen.getByLabelText(/Target date/i)).toHaveValue("2026-08-30");
  });

  it("shows the plan, diagnosis and holder already on the entry", () => {
    renderRevising();
    expect(screen.getByLabelText(/^Plan/i)).toHaveValue("Get Doctor Who to fix it");
    expect(screen.getByLabelText(/^Diagnosis/i)).toHaveValue(
      "theres a negative pole on the tranny-transformer",
    );
    expect(screen.getByLabelText(/Who is on it/i)).toHaveValue("ticket");
    expect(screen.getByLabelText(/^Reference/i)).toHaveValue("Jira 999");
  });

  // The one that could not be done at all: taking somebody off an entry.
  it("can set an assigned entry back to unassigned", async () => {
    const user = userEvent.setup();
    const onRevise = renderRevising();

    await user.selectOptions(screen.getByLabelText(/Who is on it/i), "");

    // Choosing "Nobody yet" takes the reference with it: left behind, it keeps
    // the old kind alive and silently discards the unassignment.
    expect(screen.getByLabelText(/^Reference/i)).toHaveValue("");

    await user.click(screen.getByRole("button", { name: /^Save$/i }));

    await waitFor(() => {
      expect(onRevise).toHaveBeenCalledWith(
        expect.objectContaining({ holder_type: "", holder_ref: "" }),
      );
    });
  });

  // Saving without touching anything must not quietly drop what is there.
  it("keeps every field when nothing is changed", async () => {
    const user = userEvent.setup();
    const onRevise = renderRevising();

    await user.click(screen.getByRole("button", { name: /^Save$/i }));

    await waitFor(() => {
      expect(onRevise).toHaveBeenCalledWith(
        expect.objectContaining({
          plan: "Get Doctor Who to fix it",
          diagnosis: "theres a negative pole on the tranny-transformer",
          target_date: "2026-08-30",
          holder_type: "ticket",
          holder_ref: "Jira 999",
        }),
      );
    });
  });
});

// ---------------------------------------------------------------------------
// Who is on it, as an identity rather than a string
//
// A ticket is free text on purpose — it addresses a system CMM does not read.
// An owner is not: a name typed by hand is a commitment held by somebody
// nobody can be reached through, nothing ever reconciles it, and it cannot be
// grouped or filtered on later.
// ---------------------------------------------------------------------------

describe("FailureEntryDialog — who is on it, resolved", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 8, total_items: 0, total_pages: 0 },
    });
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 32, total_items: 0, total_pages: 0 },
    });
    vi.mocked(api.fetchOwners).mockResolvedValue({
      data: [
        {
          name: "thomas-smith",
          display_name: "Thomas Smith",
          contact_email: "thomas.smith@example-corp.test",
          owner_type: "individual",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      pagination: { page: 1, per_page: 8, total_items: 1, total_pages: 1 },
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

  // Everything person-shaped is an owner; a CMM user reaches one through an
  // alias. Offering both would be two identity spaces for the same thing.
  it("offers an owner or a ticket, and nothing else", () => {
    renderRecording();
    const kinds = screen.getByLabelText(/Who is on it/i);
    expect(kinds).toHaveTextContent(/An owner/i);
    expect(kinds).toHaveTextContent(/A ticket or work item/i);
    expect(kinds).not.toHaveTextContent(/A user/i);
  });

  it("picks an owner from the catalogue", async () => {
    const user = userEvent.setup();
    const onSubmit = renderRecording();

    await user.type(screen.getByLabelText(/^Reason/i), "Bogus failure");
    await user.selectOptions(screen.getByLabelText(/Who is on it/i), "owner");
    await user.type(screen.getByLabelText(/^Reference/i), "thomas");
    await user.click(
      await screen.findByRole("button", { name: /thomas-smith/i }),
    );

    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          holder_type: "owner",
          holder_ref: "thomas-smith",
        }),
      );
    });
  });

  // The black hole this closes.
  it("will not record an owner name that was typed rather than picked", async () => {
    const user = userEvent.setup();
    renderRecording();

    await user.type(screen.getByLabelText(/^Reason/i), "Bogus failure");
    await user.selectOptions(screen.getByLabelText(/Who is on it/i), "owner");
    await user.type(screen.getByLabelText(/^Reference/i), "someone made up");

    expect(
      screen.getByRole("button", { name: /Record this verdict/i }),
    ).toBeDisabled();
    expect(
      screen.getByText(/Choose an owner from the list/i),
    ).toBeInTheDocument();
  });

  // A ticket addresses a system CMM does not read, so there is nothing to
  // resolve it against and free text is the whole contract.
  it("leaves a ticket reference as free text", async () => {
    const user = userEvent.setup();
    const onSubmit = renderRecording();

    await user.type(screen.getByLabelText(/^Reason/i), "Bogus failure");
    await user.type(screen.getByLabelText(/^Reference/i), "Jira 999");

    expect(screen.getByLabelText(/Who is on it/i)).toHaveValue("ticket");
    await user.click(
      screen.getByRole("button", { name: /Record this verdict/i }),
    );

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ holder_type: "ticket", holder_ref: "Jira 999" }),
      );
    });
  });
});
