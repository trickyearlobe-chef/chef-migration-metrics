// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { OwnerFilter } from "./OwnerFilter";
import * as api from "../api";
import type { Owner } from "../types";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return { ...actual, fetchOwners: vi.fn() };
});

const mockedFetchOwners = vi.mocked(api.fetchOwners);

function owner(name: string, displayName?: string): Owner {
  return {
    name,
    display_name: displayName,
    owner_type: "person",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function renderControl(
  props: Partial<React.ComponentProps<typeof OwnerFilter>> = {},
) {
  const onChange = props.onChange ?? vi.fn();
  const result = render(
    <OwnerFilter owners={[]} unowned={false} {...props} onChange={onChange} />,
  );
  return { ...result, onChange };
}

/** Resolve the promise chain the fetch runs through. */
async function flush() {
  await act(async () => {
    for (let i = 0; i < 10; i++) await Promise.resolve();
  });
}

async function open() {
  fireEvent.click(screen.getByRole("button", { name: /^Owner/ }));
  await flush();
}

async function typeSearch(text: string, ms = 300) {
  fireEvent.change(screen.getByPlaceholderText(/search owners/i), {
    target: { value: text },
  });
  act(() => {
    vi.advanceTimersByTime(ms);
  });
  await flush();
}

describe("OwnerFilter", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockedFetchOwners.mockReset();
    mockedFetchOwners.mockResolvedValue({
      data: [owner("alice.brown", "Alice Brown"), owner("bob.jones")],
      pagination: { page: 1, per_page: 50, total_items: 2, total_pages: 1 },
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  // 1,862 owners is not a list anybody scrolls, so the control has to be
  // searchable and the search has to reach the server rather than filter a
  // page of fifty locally.
  it("searches the server for owners as somebody types", async () => {
    renderControl();
    await open();

    await typeSearch("ali");

    expect(mockedFetchOwners).toHaveBeenLastCalledWith(
      expect.objectContaining({ search: "ali" }),
    );
  });

  it("reports the owners that were ticked", async () => {
    const { onChange } = renderControl();
    await open();

    fireEvent.click(screen.getByRole("checkbox", { name: /Alice Brown/ }));

    expect(onChange).toHaveBeenCalledWith({
      owners: ["alice.brown"],
      unowned: false,
    });
  });

  // The API rejects owner and unowned together with a 400. The control has to
  // make that unreachable rather than let somebody hit the error.
  it("drops any chosen owner when asked for the ones with nobody", async () => {
    const { onChange } = renderControl({ owners: ["alice.brown"] });
    await open();

    fireEvent.click(screen.getByRole("checkbox", { name: /No owner/i }));

    expect(onChange).toHaveBeenCalledWith({ owners: [], unowned: true });
  });

  // The other half of the same rule, approached from the other side.
  it("cannot tick an owner while the no-owner question is being asked", async () => {
    renderControl({ unowned: true });
    await open();

    expect(screen.getByRole("checkbox", { name: /Alice Brown/ })).toBeDisabled();
  });

  it("shows the chosen owners as removable chips", async () => {
    const { onChange } = renderControl({ owners: ["alice.brown"] });

    fireEvent.click(screen.getByRole("button", { name: "Remove alice.brown" }));

    expect(onChange).toHaveBeenCalledWith({ owners: [], unowned: false });
  });

  // An empty result reads as "nobody by that name", not as a broken control.
  it("says so when no owner matches", async () => {
    mockedFetchOwners.mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 50, total_items: 0, total_pages: 0 },
    });
    renderControl();
    await open();

    expect(screen.getByText(/no owners match/i)).toBeInTheDocument();
  });

  // A catalogue that cannot be read must say so. Rendering an empty list would
  // read as "this estate has no owners", which is a different and wrong answer.
  it("reports a failure to load the owners rather than showing none", async () => {
    mockedFetchOwners.mockRejectedValue(new Error("network is down"));
    renderControl();
    await open();

    expect(screen.getByText(/could not load owners/i)).toBeInTheDocument();
  });
});
