// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { saveServerConfig } from "./config";
import type { ServerConfig } from "../types";

// What the server settings screen sends when somebody saves.
//
// The screen loads the whole section, including the certificate chain and the
// ACME status, which are read-only things the service attaches on the way out.
// It then hands the same object back on save. The service refuses a call
// carrying anything it does not recognise, so those fields must be stripped
// first — these tests hold that.

function serverConfigAsLoaded(): ServerConfig {
  return {
    host: "0.0.0.0",
    port: 443,
    tls: { mode: "self_signed" },
    websocket: { enabled: true, max_connections: 100 },
    graceful_shutdown_seconds: 30,
    trusted_proxy: false,
    // Attached by the GET. Neither is a setting.
    tls_certificate_info: [{ subject: "CN=example.com" }],
    acme_status: { state: "valid" },
  } as unknown as ServerConfig;
}

function bodySentBy(call: unknown): Record<string, unknown> {
  const init = (call as [string, RequestInit])[1];
  return JSON.parse(init.body as string);
}

describe("saveServerConfig", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ value: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("does not send back the read-only things the GET attached", async () => {
    await saveServerConfig(serverConfigAsLoaded());

    const sent = bodySentBy(vi.mocked(fetch).mock.calls[0]);
    expect(sent).not.toHaveProperty("tls_certificate_info");
    expect(sent).not.toHaveProperty("acme_status");
  });

  it("still sends the settings themselves", async () => {
    // The baseline. Without it, a save that sent nothing at all would pass the
    // test above while quietly clearing every server setting there is.
    await saveServerConfig(serverConfigAsLoaded());

    const sent = bodySentBy(vi.mocked(fetch).mock.calls[0]);
    expect(sent.port).toBe(443);
    expect(sent.host).toBe("0.0.0.0");
    expect(sent.graceful_shutdown_seconds).toBe(30);
    expect(sent.trusted_proxy).toBe(false);
    expect(sent.tls).toEqual({ mode: "self_signed" });
    expect(sent.websocket).toEqual({ enabled: true, max_connections: 100 });
  });
});
