// SPDX-License-Identifier: Apache-2.0

import type {
  CookstyleViolationsResponse,
  CopAggregationResponse,
  CopCookbookResponse,
  CustomCopDefinition,
} from "../types";
import { apiFetch, buildUrl } from "./client";

// ---------------------------------------------------------------------------
// Violations browser (legacy)
// ---------------------------------------------------------------------------

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
