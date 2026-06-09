// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { apiFetch } from "./client";

function makeResponse(
  status: number,
  body: string,
  contentType = "application/json",
): Response {
  return new Response(body || null, {
    status,
    headers: body ? { "Content-Type": contentType } : {},
  });
}

describe("apiFetch", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("parses a JSON success response", async () => {
    vi.mocked(fetch).mockResolvedValue(
      makeResponse(200, JSON.stringify({ id: 1, name: "test" })),
    );
    const result = await apiFetch<{ id: number; name: string }>("/foo");
    expect(result).toEqual({ id: 1, name: "test" });
  });

  it("returns undefined for a 204 No Content response", async () => {
    vi.mocked(fetch).mockResolvedValue(makeResponse(204, ""));
    const result = await apiFetch<void>("/foo", { method: "DELETE" });
    expect(result).toBeUndefined();
  });

  it("returns undefined for a 205 Reset Content response", async () => {
    vi.mocked(fetch).mockResolvedValue(makeResponse(205, ""));
    const result = await apiFetch<void>("/foo", { method: "POST" });
    expect(result).toBeUndefined();
  });

  it("returns undefined for an empty 200 body", async () => {
    vi.mocked(fetch).mockResolvedValue(makeResponse(200, ""));
    const result = await apiFetch<void>("/foo");
    expect(result).toBeUndefined();
  });

  it("returns undefined for a whitespace-only 200 body", async () => {
    vi.mocked(fetch).mockResolvedValue(makeResponse(200, "   "));
    const result = await apiFetch<void>("/foo");
    expect(result).toBeUndefined();
  });

  it("throws ApiError with structured message from JSON error body", async () => {
    vi.mocked(fetch).mockResolvedValue(
      makeResponse(422, JSON.stringify({ message: "validation failed" })),
    );
    await expect(apiFetch("/foo")).rejects.toMatchObject({
      name: "ApiError",
      status: 422,
      message: "validation failed",
    });
  });

  it("throws ApiError with plain text error body as message", async () => {
    vi.mocked(fetch).mockResolvedValue(
      makeResponse(500, "internal server error", "text/plain"),
    );
    await expect(apiFetch("/foo")).rejects.toMatchObject({
      name: "ApiError",
      status: 500,
      message: "internal server error",
    });
  });

  it("throws ApiError using error field when message is absent", async () => {
    vi.mocked(fetch).mockResolvedValue(
      makeResponse(403, JSON.stringify({ error: "forbidden" })),
    );
    await expect(apiFetch("/foo")).rejects.toMatchObject({
      name: "ApiError",
      status: 403,
      message: "forbidden",
    });
  });

  it("prepends BASE path when url does not start with /api/", async () => {
    vi.mocked(fetch).mockResolvedValue(makeResponse(200, "{}"));
    await apiFetch("/nodes");
    expect(vi.mocked(fetch)).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/nodes"),
      expect.anything(),
    );
  });

  it("uses url as-is when it starts with /api/", async () => {
    vi.mocked(fetch).mockResolvedValue(makeResponse(200, "{}"));
    await apiFetch("/api/v1/nodes");
    expect(vi.mocked(fetch)).toHaveBeenCalledWith(
      "/api/v1/nodes",
      expect.anything(),
    );
  });
});
