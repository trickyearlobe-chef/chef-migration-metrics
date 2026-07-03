// SPDX-License-Identifier: Apache-2.0

import type {
  CopAggregationResponse,
  CopCookbookResponse,
  CopDriftReport,
  CustomCopDefinition,
} from "../types";
import { apiFetch, buildUrl } from "./client";

// ---------------------------------------------------------------------------
// Cop Analysis
// ---------------------------------------------------------------------------

export interface CopAggregationQuery {
  target_chef_version?: string;
  source?: "server" | "git";
  classification?: string;
  sort?: string;
  order?: string;
  page?: number;
  per_page?: number;
  // When true, restrict the data page to cops that have triggered at least one
  // scan offence. The list otherwise returns every known cop (curated defaults +
  // RemovedIn mappings + scanned + custom).
  triggered_only?: boolean;
}

export function fetchCookstyleCops(
  params?: CopAggregationQuery,
): Promise<CopAggregationResponse> {
  return apiFetch<CopAggregationResponse>(
    buildUrl(
      "/cookstyle/cops",
      params as Record<string, string | number | boolean | undefined>,
    ),
  );
}

// fetchCopDrift returns the classification-table drift report (stale entries +
// Chef/* coverage gaps) for the given target version. Omit the target to use
// the server's configured default.
export function fetchCopDrift(params?: {
  target_chef_version?: string;
}): Promise<CopDriftReport> {
  return apiFetch<CopDriftReport>(
    buildUrl(
      "/cookstyle/cop-drift",
      params as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchCookstyleCopCookbooks(
  copName: string,
  params?: { target_chef_version?: string; source?: string; page?: number; per_page?: number },
): Promise<CopCookbookResponse> {
  return apiFetch<CopCookbookResponse>(
    buildUrl(
      `/cookstyle/cops/${copName}/cookbooks`,
      params as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function setCopClassification(
  copName: string,
  body: { target_chef_version: string; classification: string; reason?: string },
): Promise<{ status: string }> {
  return apiFetch<{ status: string }>(
    `/api/v1/cookstyle/cops/${copName}/classification`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}

export function deleteCopClassification(
  copName: string,
  targetChefVersion: string,
): Promise<{ status: string }> {
  return apiFetch<{ status: string }>(
    buildUrl(`/cookstyle/cops/${copName}/classification`, {
      target_chef_version: targetChefVersion,
    }),
    { method: "DELETE" },
  );
}

// ---------------------------------------------------------------------------
// Custom Cops
// ---------------------------------------------------------------------------

export function fetchCustomCops(): Promise<{ data: CustomCopDefinition[] }> {
  return apiFetch<{ data: CustomCopDefinition[] }>(
    buildUrl("/cookstyle/custom-cops"),
  );
}

export function createCustomCop(
  cop: Omit<CustomCopDefinition, "id" | "created_at" | "updated_at">,
): Promise<CustomCopDefinition> {
  return apiFetch<CustomCopDefinition>("/api/v1/cookstyle/custom-cops", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(cop),
  });
}

export function updateCustomCop(
  copName: string,
  cop: Partial<CustomCopDefinition>,
): Promise<CustomCopDefinition> {
  return apiFetch<CustomCopDefinition>(
    `/api/v1/cookstyle/custom-cops/${copName}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(cop),
    },
  );
}

export function deleteCustomCop(copName: string): Promise<{ status: string }> {
  return apiFetch<{ status: string }>(
    `/api/v1/cookstyle/custom-cops/${copName}`,
    { method: "DELETE" },
  );
}
