// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import type { OwnerDuplicatesResponse } from "../types";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchOwnerDuplicates: vi.fn(),
    mergeOwners: vi.fn(),
    rescanOwnerDuplicates: vi.fn(),
    dismissOwnerDuplicate: vi.fn(),
  };
});

const mockUseAuth = vi.fn();
vi.mock("../context/AuthContext", () => ({ useAuth: () => mockUseAuth() }));

import { OwnerDuplicatesPage } from "./OwnerDuplicatesPage";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

const candidate = {
  owner_a: "thomas-smith",
  owner_b: "tommy-smith",
  matched_on: "name",
  value_a: "thomas-smith",
  value_b: "tommy-smith",
  similarity: 0.82,
  assignments_a: 12,
  assignments_b: 1,
};

function response(overrides: Partial<OwnerDuplicatesResponse> = {}) {
  return {
    data: [candidate],
    pagination: { page: 1, per_page: 25, total_items: 1, total_pages: 1 },
    coverage: { owners_total: 40, owners_without_alias: 7 },
    scan: { scanned_at: "2026-08-02T09:00:00Z", pairs_found: 1 },
    scan_running: false,
    ...overrides,
  };
}

describe("OwnerDuplicatesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchOwnerDuplicates).mockResolvedValue(response());
    vi.mocked(api.mergeOwners).mockResolvedValue({
      from_owner: "tommy-smith",
      into_owner: "thomas-smith",
      reassigned: 1,
      skipped: 0,
      aliases_moved: 2,
      aliases_dropped: 0,
      source_name_aliased: true,
    });
  });

  it("pairs each candidate with who they might already be", async () => {
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });

    expect(await screen.findByRole("link", { name: "thomas-smith" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "tommy-smith" })).toBeInTheDocument();
    expect(screen.getByText("82%")).toBeInTheDocument();
  });

  // An empty list must not read as "there are no duplicates" when the report
  // can only see part of the catalogue.
  it("states how much of the catalogue it can see", async () => {
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });

    expect(
      await screen.findByText(/7 of 40 owners have no alias recorded/i),
    ).toBeInTheDocument();
  });

  it("folds the smaller side into the larger by default", async () => {
    const user = userEvent.setup();
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });
    await screen.findByRole("link", { name: "thomas-smith" });

    await user.click(screen.getByRole("button", { name: /merge/i }));
    await user.click(await screen.findByRole("button", { name: "Merge owners" }));

    await waitFor(() =>
      expect(api.mergeOwners).toHaveBeenCalledWith({
        from_owner: "tommy-smith",
        into_owner: "thomas-smith",
      }),
    );
  });

  it("can merge the other way round", async () => {
    const user = userEvent.setup();
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });
    await screen.findByRole("link", { name: "thomas-smith" });

    await user.click(screen.getByRole("button", { name: /merge/i }));
    await user.click(await screen.findByRole("button", { name: /swap direction/i }));
    await user.click(screen.getByRole("button", { name: "Merge owners" }));

    await waitFor(() =>
      expect(api.mergeOwners).toHaveBeenCalledWith({
        from_owner: "thomas-smith",
        into_owner: "tommy-smith",
      }),
    );
  });

  it("reloads the list after a merge so the pair leaves it", async () => {
    const user = userEvent.setup();
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });
    await screen.findByRole("link", { name: "thomas-smith" });

    await user.click(screen.getByRole("button", { name: /merge/i }));
    await user.click(await screen.findByRole("button", { name: "Merge owners" }));

    await waitFor(() => expect(api.fetchOwnerDuplicates).toHaveBeenCalledTimes(2));
  });

  it("offers no merge to somebody who cannot delete an owner", async () => {
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: false,
      user: { role: "operator", username: "test" },
    });
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });
    await screen.findByRole("link", { name: "thomas-smith" });

    expect(screen.queryByRole("button", { name: /merge/i })).not.toBeInTheDocument();
  });

  it("says so plainly when nothing looks like a duplicate", async () => {
    vi.mocked(api.fetchOwnerDuplicates).mockResolvedValue(
      response({
        data: [],
        pagination: { page: 1, per_page: 25, total_items: 0, total_pages: 0 },
        scan: { scanned_at: "2026-08-02T09:00:00Z", pairs_found: 0 },
      }),
    );
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });

    expect(await screen.findByText(/no possible duplicates/i)).toBeInTheDocument();
  });

  // An empty list because nobody has looked yet is the opposite message from
  // an empty list because nothing looks alike.
  it("distinguishes a catalogue nobody has scanned from one with no duplicates", async () => {
    vi.mocked(api.fetchOwnerDuplicates).mockResolvedValue(
      response({
        data: [],
        pagination: { page: 1, per_page: 25, total_items: 0, total_pages: 0 },
        scan: undefined,
      }),
    );
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });

    expect(await screen.findByText(/never been scanned/i)).toBeInTheDocument();
    expect(screen.queryByText(/no possible duplicates/i)).not.toBeInTheDocument();
  });

  it("shows when the list was last built", async () => {
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });

    expect(await screen.findByText(/last scanned/i)).toBeInTheDocument();
  });

  it("starts a scan and says it is running", async () => {
    const user = userEvent.setup();
    vi.mocked(api.rescanOwnerDuplicates).mockResolvedValue({ started: true });
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });
    await screen.findByRole("link", { name: "thomas-smith" });

    await user.click(screen.getByRole("button", { name: /scan for duplicates/i }));

    await waitFor(() => expect(api.rescanOwnerDuplicates).toHaveBeenCalled());
    expect(await screen.findByText(/scan is running/i)).toBeInTheDocument();
  });

  it("offers no scan to a viewer", async () => {
    mockUseAuth.mockReturnValue({
      isOperator: false,
      isAdmin: false,
      user: { role: "viewer", username: "test" },
    });
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });
    await screen.findByRole("link", { name: "thomas-smith" });

    expect(
      screen.queryByRole("button", { name: /scan for duplicates/i }),
    ).not.toBeInTheDocument();
  });
});

describe("OwnerDuplicatesPage — rejecting a pair", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchOwnerDuplicates).mockResolvedValue(response());
    vi.mocked(api.dismissOwnerDuplicate).mockResolvedValue({ dismissed: true });
  });

  // Without this the view offers a merge and nothing else, so a pair somebody
  // has already looked at comes back on every scan.
  it("records that two owners are not the same person", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "prompt").mockReturnValue("different people");

    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });
    await user.click(
      await screen.findByRole("button", { name: /Not a duplicate/i }),
    );

    await waitFor(() => {
      expect(api.dismissOwnerDuplicate).toHaveBeenCalledWith({
        owner_a: "thomas-smith",
        owner_b: "tommy-smith",
        reason: "different people",
      });
    });
  });

  it("does nothing if the reason prompt is cancelled", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "prompt").mockReturnValue(null);

    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });
    await user.click(
      await screen.findByRole("button", { name: /Not a duplicate/i }),
    );

    expect(api.dismissOwnerDuplicate).not.toHaveBeenCalled();
  });

  // An empty list worked down to nothing means something different from one
  // nobody has read.
  it("says an empty list was worked down rather than never scanned", async () => {
    vi.mocked(api.fetchOwnerDuplicates).mockResolvedValue(
      response({ data: [], dismissed_pairs: 7 }),
    );
    render(<OwnerDuplicatesPage />, { wrapper: Wrapper });

    expect(
      await screen.findByText(/7 have been rejected as different people/i),
    ).toBeInTheDocument();
  });
});
