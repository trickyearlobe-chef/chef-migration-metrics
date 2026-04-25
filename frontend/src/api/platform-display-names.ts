// SPDX-License-Identifier: Apache-2.0

import { apiFetch, buildUrl } from "./client";

export interface DisplayNameMapping {
  platform: string;
  version_prefix: string;
  display_name: string;
}

export interface PlatformDisplayNamesResponse {
  mappings: DisplayNameMapping[];
  is_default: boolean;
}

export function fetchPlatformDisplayNames(): Promise<PlatformDisplayNamesResponse> {
  return apiFetch<PlatformDisplayNamesResponse>(
    buildUrl("/admin/platform-display-names"),
  );
}

export function updatePlatformDisplayNames(
  mappings: DisplayNameMapping[],
): Promise<PlatformDisplayNamesResponse> {
  return apiFetch<PlatformDisplayNamesResponse>(
    buildUrl("/admin/platform-display-names"),
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(mappings),
    },
  );
}

export function resetPlatformDisplayNames(): Promise<PlatformDisplayNamesResponse> {
  return apiFetch<PlatformDisplayNamesResponse>(
    buildUrl("/admin/platform-display-names/reset"),
    {
      method: "POST",
    },
  );
}
