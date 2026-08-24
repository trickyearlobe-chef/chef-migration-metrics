// SPDX-License-Identifier: Apache-2.0

import type {
  HealthResponse,
  VersionResponse,
  OrganisationsResponse,
  TLSStatus,
} from "../types";
import { apiFetch, buildUrl } from "./client";

export function fetchHealth(): Promise<HealthResponse> {
  return apiFetch<HealthResponse>(buildUrl("/health"));
}

export function fetchVersion(): Promise<VersionResponse> {
  return apiFetch<VersionResponse>(buildUrl("/version"));
}

export function fetchOrganisations(): Promise<OrganisationsResponse> {
  return apiFetch<OrganisationsResponse>(buildUrl("/organisations"));
}

// fetchTLSStatus reports whether the server fell back to plain HTTP because the
// configured static TLS certificate failed to load at startup.
// Public + DB-independent, so it is safe to poll from a global banner.
export function fetchTLSStatus(): Promise<TLSStatus> {
  return apiFetch<TLSStatus>(buildUrl("/server/tls-status"));
}

// waitForServerHealthy resolves once GET /api/v1/health reports healthy again,
// or rejects on timeout. Used by the "Apply & Restart" flow to detect when the
// server is back online after a graceful restart. While the server is down the
// fetch rejects; we swallow those errors and keep polling until the deadline.
export async function waitForServerHealthy(
  opts: { timeoutMs?: number; intervalMs?: number } = {},
): Promise<void> {
  const timeoutMs = opts.timeoutMs ?? 120_000;
  const intervalMs = opts.intervalMs ?? 2_000;
  const deadline = Date.now() + timeoutMs;

  for (;;) {
    try {
      const h = await fetchHealth();
      if (h.status === "healthy") return;
    } catch {
      // Server still restarting — ignore and retry until the deadline.
    }
    if (Date.now() >= deadline) {
      throw new Error("Timed out waiting for the server to come back online.");
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
}

export function pollHealth(
  onUpdate: (h: HealthResponse) => void,
  intervalMs = 30_000,
): () => void {
  let active = true;

  const tick = async () => {
    if (!active) return;
    try {
      const h = await fetchHealth();
      if (active) onUpdate(h);
    } catch {
      // Silently ignore poll errors — the next tick will retry.
    }
  };

  // Kick off the first poll immediately, then set up the interval.
  tick();
  const id = setInterval(tick, intervalMs);

  return () => {
    active = false;
    clearInterval(id);
  };
}
