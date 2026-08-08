// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import type { IntakeMapping } from "../types";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchImportMappings: vi.fn(),
    runImportNow: vi.fn(),
    previewClearImportedOwnership: vi.fn(),
    clearImportedOwnership: vi.fn(),
  };
});

import { ScheduledImports } from "./ScheduledImports";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

function savedImport(overrides: Partial<IntakeMapping> = {}): IntakeMapping {
  return {
    id: 1,
    name: "cmdb-nightly",
    source_kind: "database",
    delimiter: ",",
    created_by: "admin",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
    db_credential: "cmdb-connection",
    schedule: "0 2 * * *",
    schedule_enabled: true,
    ...overrides,
  };
}

function respondWith(imports: IntakeMapping[]) {
  vi.mocked(api.fetchImportMappings).mockResolvedValue({
    data: imports,
    pagination: { page: 1, per_page: 50, total_items: imports.length, total_pages: 1 },
  } as never);
}

describe("saved database imports", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.previewClearImportedOwnership).mockResolvedValue({
      assignments: 41,
      owners: 7,
    });
  });

  // An import that runs with nobody watching has to be checkable. "It is
  // scheduled" and "it is working" are different claims, and only the second
  // one matters — so the last run sits beside the schedule.
  it("says in English when each import runs, not just the cron", async () => {
    respondWith([savedImport()]);
    render(<ScheduledImports />, { wrapper: Wrapper });

    expect(await screen.findByText("cmdb-nightly")).toBeInTheDocument();
    expect(await screen.findByText(/At 02:00/i)).toBeInTheDocument();
  });

  it("shows a failed run and why it failed", async () => {
    respondWith([
      savedImport({
        last_run_at: "2026-08-06T02:00:00Z",
        last_run_status: "failed",
        last_run_detail: 'could not read the credential "cmdb"',
      }),
    ]);
    render(<ScheduledImports />, { wrapper: Wrapper });

    expect(await screen.findByText(/could not read the credential/)).toBeInTheDocument();
  });

  // A schedule that has never fired must not read like one that succeeded.
  it("distinguishes never-run from succeeded", async () => {
    respondWith([savedImport()]);
    render(<ScheduledImports />, { wrapper: Wrapper });

    expect(await screen.findByText(/has not run yet/i)).toBeInTheDocument();
  });

  // An unscheduled saved import still belongs here: it is the thing you want
  // to run by hand while judging whether a source is any good. Listing only
  // the scheduled ones would hide exactly the imports being worked on.
  it("lists unscheduled imports too, marked as unscheduled", async () => {
    respondWith([
      savedImport(),
      savedImport({ id: 2, name: "one-off", schedule: "", schedule_enabled: false }),
    ]);
    render(<ScheduledImports />, { wrapper: Wrapper });

    expect(await screen.findByText("one-off")).toBeInTheDocument();
    expect(await screen.findByText(/Not scheduled/i)).toBeInTheDocument();
  });

  // A file import has no stored source, so there is nothing to re-run.
  it("leaves out file imports, which cannot be re-run", async () => {
    respondWith([
      savedImport(),
      savedImport({ id: 3, name: "a-csv-mapping", source_kind: "csv", schedule: "", schedule_enabled: false }),
    ]);
    render(<ScheduledImports />, { wrapper: Wrapper });

    expect(await screen.findByText("cmdb-nightly")).toBeInTheDocument();
    expect(screen.queryByText("a-csv-mapping")).not.toBeInTheDocument();
  });

  it("says so when nothing is saved, rather than showing an empty box", async () => {
    respondWith([]);
    render(<ScheduledImports />, { wrapper: Wrapper });

    expect(await screen.findByText(/No database import is saved/i)).toBeInTheDocument();
  });

  // An unreadable list and an empty one look the same and mean opposite
  // things — the same fault this codebase has hit repeatedly.
  it("says so when the list cannot be loaded", async () => {
    vi.mocked(api.fetchImportMappings).mockRejectedValue(new Error("nope"));
    render(<ScheduledImports />, { wrapper: Wrapper });

    expect(await screen.findByText(/Could not load/i)).toBeInTheDocument();
  });

  // Run now exists for judging a source, so it has to report what the run did
  // rather than that it started.
  it("runs an import on demand and reports what it did", async () => {
    respondWith([savedImport()]);
    vi.mocked(api.runImportNow).mockResolvedValue({
      summary: { row_count: 11, filtered_out: 0, counts: {} },
      detail: "11 rows, 8 created, 2 rejected",
    });
    const user = userEvent.setup();
    render(<ScheduledImports />, { wrapper: Wrapper });

    await user.click(await screen.findByRole("button", { name: /Run now/ }));

    await waitFor(() => expect(api.runImportNow).toHaveBeenCalledWith(1));
    expect(await screen.findByText(/11 rows, 8 created, 2 rejected/)).toBeInTheDocument();
  });

  it("reports a failed run rather than falling silent", async () => {
    respondWith([savedImport()]);
    vi.mocked(api.runImportNow).mockRejectedValue(new Error("could not connect"));
    const user = userEvent.setup();
    render(<ScheduledImports />, { wrapper: Wrapper });

    await user.click(await screen.findByRole("button", { name: /Run now/ }));

    expect(await screen.findByText(/could not connect/)).toBeInTheDocument();
  });
});

// Throwing away what an import brought in, so the next one can be judged on its
// own. This deletes data, so the confirmation has to say how much.
describe("clearing imported ownership", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchImportMappings).mockResolvedValue({
      data: [savedImport()],
      pagination: { page: 1, per_page: 50, total_items: 1, total_pages: 1 },
    } as never);
    vi.mocked(api.previewClearImportedOwnership).mockResolvedValue({
      assignments: 41,
      owners: 7,
    });
  });

  it("names the numbers before asking, and does not delete on the first click", async () => {
    const user = userEvent.setup();
    render(<ScheduledImports />, { wrapper: Wrapper });

    await user.click(await screen.findByRole("button", { name: /Clear imported ownership/ }));

    expect(await screen.findByText(/41/)).toBeInTheDocument();
    expect(await screen.findByText(/7/)).toBeInTheDocument();
    expect(api.clearImportedOwnership).not.toHaveBeenCalled();
  });

  it("removes only after the confirmation, and says what went", async () => {
    vi.mocked(api.clearImportedOwnership).mockResolvedValue({ assignments: 41, owners: 7 });
    const user = userEvent.setup();
    render(<ScheduledImports />, { wrapper: Wrapper });

    await user.click(await screen.findByRole("button", { name: /Clear imported ownership/ }));
    await user.click(await screen.findByRole("button", { name: /Yes, remove them/ }));

    await waitFor(() => expect(api.clearImportedOwnership).toHaveBeenCalled());
    expect(await screen.findByText(/Removed 41 assignments and 7 owners/)).toBeInTheDocument();
  });

  it("can be backed out of", async () => {
    const user = userEvent.setup();
    render(<ScheduledImports />, { wrapper: Wrapper });

    await user.click(await screen.findByRole("button", { name: /Clear imported ownership/ }));
    await user.click(await screen.findByRole("button", { name: /Cancel/ }));

    expect(api.clearImportedOwnership).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: /Yes, remove them/ })).not.toBeInTheDocument();
  });

  // Nothing to remove must not present as a destructive act waiting to happen.
  it("says there is nothing to remove when there is nothing to remove", async () => {
    vi.mocked(api.previewClearImportedOwnership).mockResolvedValue({
      assignments: 0,
      owners: 0,
    });
    const user = userEvent.setup();
    render(<ScheduledImports />, { wrapper: Wrapper });

    await user.click(await screen.findByRole("button", { name: /Clear imported ownership/ }));

    expect(await screen.findByText(/Nothing to remove/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Yes, remove them/ })).not.toBeInTheDocument();
  });
});
