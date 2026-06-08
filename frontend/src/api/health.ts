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
// configured static TLS certificate failed to load at startup (tls.md § 2.4).
// Public + DB-independent, so it is safe to poll from a global banner.
export function fetchTLSStatus(): Promise<TLSStatus> {
  return apiFetch<TLSStatus>(buildUrl("/server/tls-status"));
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
