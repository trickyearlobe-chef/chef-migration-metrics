// SPDX-License-Identifier: Apache-2.0

import { apiFetch, buildUrl } from "./client";

// Viewer-readable UI feature flags. Lets the frontend hide surfaces the operator
// has switched off (e.g. Run events kept in reserve).
export interface Features {
  run_events: boolean;
}

export function fetchFeatures(): Promise<Features> {
  return apiFetch<Features>(buildUrl("/features"));
}
