// SPDX-License-Identifier: Apache-2.0

import type {
  RemediationPriorityResponse,
  RemediationSummaryResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";

export interface RemediationQuery {
  organisation?: string;
  target_chef_version?: string;
  complexity_label?: string;
  search?: string;
  sort?: string;
  order?: string;
  page?: number;
  per_page?: number;
}

export function fetchRemediationPriority(
  filters?: RemediationQuery,
): Promise<RemediationPriorityResponse> {
  return apiFetch<RemediationPriorityResponse>(
    buildUrl(
      "/remediation/priority",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchRemediationSummary(params?: {
  organisation?: string;
  target_chef_version?: string;
}): Promise<RemediationSummaryResponse> {
  return apiFetch<RemediationSummaryResponse>(
    buildUrl("/remediation/summary", params),
  );
}
