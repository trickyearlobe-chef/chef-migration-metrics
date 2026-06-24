// SPDX-License-Identifier: Apache-2.0

import type { CookstyleViolationsResponse } from "../types";
import { apiFetch, buildUrl } from "./client";

export interface CookstyleViolationsQuery {
  source?: "server" | "git";
  target_chef_version?: string;
  status?: string;
  namespace?: string;
  severity?: string;
  cop?: string;
  page?: number;
  per_page?: number;
  sort?: string;
  order?: string;
}

export function fetchCookstyleViolations(
  params?: CookstyleViolationsQuery,
): Promise<CookstyleViolationsResponse> {
  return apiFetch<CookstyleViolationsResponse>(
    buildUrl(
      "/cookstyle/violations",
      params as Record<string, string | number | boolean | undefined>,
    ),
  );
}
