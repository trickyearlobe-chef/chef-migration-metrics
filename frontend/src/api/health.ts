// SPDX-License-Identifier: Apache-2.0

import type { HealthResponse, VersionResponse, OrganisationsResponse } from "../types";
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
