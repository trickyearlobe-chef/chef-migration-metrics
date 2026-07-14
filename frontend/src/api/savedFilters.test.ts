// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  listSavedFilters,
  createSavedFilter,
  updateSavedFilter,
  deleteSavedFilter,
} from "./savedFilters";
import { ApiError } from "./client";
import type { SavedFilter } from "../types";

const filter: SavedFilter = {
  id: "11111111-1111-1111-1111-111111111111",
  name: "All Windows OS",
  view: "nodes",
  filters: { role: ["win-base", "win-iis"] },
  owner_username: "org-a-user",
  shared: false,
  created_at: "2026-07-14T10:00:00Z",
  updated_at: "2026-07-14T10:00:00Z",
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("saved filters api", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists the filters for a view", async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse([filter]));

    const got = await listSavedFilters("nodes");

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/saved-filters?view=nodes",
      expect.anything(),
    );
    expect(got).toEqual([filter]);
  });

  // The backend returns [] rather than null, but a caller must not blow up if
  // that ever changes — the control renders a list unconditionally.
  it("returns an array when the body is null", async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse(null));
    await expect(listSavedFilters("nodes")).resolves.toEqual([]);
  });

  it("creates a filter", async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse(filter, 201));

    const got = await createSavedFilter({
      name: "All Windows OS",
      view: "nodes",
      filters: { role: ["win-base", "win-iis"] },
      shared: false,
    });

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/saved-filters",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          name: "All Windows OS",
          view: "nodes",
          filters: { role: ["win-base", "win-iis"] },
          shared: false,
        }),
      }),
    );
    expect(got).toEqual(filter);
  });

  // 409 on a duplicate name for the (owner, view) — the control surfaces the
  // backend's message rather than inventing its own.
  it("surfaces a duplicate-name conflict as an ApiError", async () => {
    vi.mocked(fetch).mockResolvedValue(
      jsonResponse(
        {
          error: "validation_error",
          message: "a saved filter named \"All Windows OS\" already exists",
        },
        409,
      ),
    );

    const err = await createSavedFilter({
      name: "All Windows OS",
      view: "nodes",
      filters: {},
      shared: false,
    }).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(409);
    expect((err as ApiError).message).toContain("already exists");
  });

  // One PATCH serves rename, re-select and share/unshare; absent fields are
  // left alone by the backend, so the client must not pad the body.
  it("patches only the fields given", async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ ...filter, shared: true }));

    await updateSavedFilter(filter.id, { shared: true });

    expect(fetch).toHaveBeenCalledWith(
      `/api/v1/saved-filters/${filter.id}`,
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ shared: true }),
      }),
    );
  });

  it("deletes a filter", async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 204 }));

    await expect(deleteSavedFilter(filter.id)).resolves.toBeUndefined();

    expect(fetch).toHaveBeenCalledWith(
      `/api/v1/saved-filters/${filter.id}`,
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("encodes the id into the path", async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 204 }));

    await deleteSavedFilter("a/b c");

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/saved-filters/a%2Fb%20c",
      expect.anything(),
    );
  });
});
