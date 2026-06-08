// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fetchSAMLMetadata } from "./saml";

function makeResponse(
  status: number,
  body: string,
  contentType = "application/samlmetadata+xml",
): Response {
  return new Response(body || null, {
    status,
    headers: body ? { "Content-Type": contentType } : {},
  });
}

const SAMPLE_XML =
  '<?xml version="1.0"?>\n<EntityDescriptor entityID="https://app.example.com/saml"></EntityDescriptor>';

describe("fetchSAMLMetadata", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns the raw XML body on success", async () => {
    vi.mocked(fetch).mockResolvedValue(makeResponse(200, SAMPLE_XML));
    const xml = await fetchSAMLMetadata();
    expect(xml).toBe(SAMPLE_XML);
  });

  it("requests the metadata endpoint", async () => {
    vi.mocked(fetch).mockResolvedValue(makeResponse(200, SAMPLE_XML));
    await fetchSAMLMetadata();
    expect(vi.mocked(fetch)).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/auth/saml/metadata"),
      expect.anything(),
    );
  });

  it("throws ApiError with status 501 when SAML is not configured", async () => {
    vi.mocked(fetch).mockResolvedValue(
      makeResponse(
        501,
        JSON.stringify({ message: "Endpoint is not yet implemented." }),
        "application/json",
      ),
    );
    await expect(fetchSAMLMetadata()).rejects.toMatchObject({
      name: "ApiError",
      status: 501,
      message: "Endpoint is not yet implemented.",
    });
  });

  it("throws ApiError on a server error", async () => {
    vi.mocked(fetch).mockResolvedValue(
      makeResponse(500, "Failed to generate SP metadata.", "text/plain"),
    );
    await expect(fetchSAMLMetadata()).rejects.toMatchObject({
      name: "ApiError",
      status: 500,
      message: "Failed to generate SP metadata.",
    });
  });
});
