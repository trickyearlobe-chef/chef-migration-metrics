// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fetchFilterTags } from "./filters";

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("fetchFilterTags", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("GETs the tags facet endpoint and returns the data array", async () => {
    vi.mocked(fetch).mockResolvedValue(
      jsonResponse({ data: ["prepare", "upgrade", "rollback"] }),
    );
    const res = await fetchFilterTags();
    expect(vi.mocked(fetch)).toHaveBeenCalledWith(
      "/api/v1/filters/tags",
      expect.anything(),
    );
    expect(res.data).toEqual(["prepare", "upgrade", "rollback"]);
  });

  it("scopes to an organisation when provided", async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ data: [] }));
    await fetchFilterTags("acme");
    expect(vi.mocked(fetch)).toHaveBeenCalledWith(
      "/api/v1/filters/tags?organisation=acme",
      expect.anything(),
    );
  });
});
