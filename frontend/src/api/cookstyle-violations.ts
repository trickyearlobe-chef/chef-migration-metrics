// SPDX-License-Identifier: Apache-2.0

import type {
  CopAggregationResponse,
  CopCookbookResponse,
  CopCookbookGroupResponse,
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

// fetchCookstyleServerCopCookbooks fetches the server drill-down grouped by
// cookbook name (source=server). The response nests per-version/org detail under
// each name and paginates by name, so its total equals the header "cookbooks
// affected" count. Use fetchCookstyleCopCookbooks (source=git) for the flat repo
// list on the Git tab.
export function fetchCookstyleServerCopCookbooks(
  copName: string,
  params?: { target_chef_version?: string; page?: number; per_page?: number },
): Promise<CopCookbookGroupResponse> {
  return apiFetch<CopCookbookGroupResponse>(
    buildUrl(
      `/cookstyle/cops/${copName}/cookbooks`,
      { ...params, source: "server" } as Record<
        string,
        string | number | boolean | undefined
      >,
    ),
  );
}

/**
 * A classification is ours, not a version's: the service stores one per cop and
 * reads no version from this call. It used to be sent anyway and silently
 * dropped, which is why it is not here.
 */
export function setCopClassification(
  copName: string,
  body: { classification: string; reason?: string },
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

// ---------------------------------------------------------------------------
// Scan scope — which files a converge never executes
// ---------------------------------------------------------------------------

/**
 * One row of the effective scan-scope list. `source` is "curated" for a seeded
 * entry standing unmodified and "operator" for anything a person decided —
 * including a seeded entry somebody overturned, which stays in the list with
 * excluded=false so the decision can be found and reversed.
 */
export interface ScanScopeEntry {
  pattern: string;
  excluded: boolean;
  reason: string;
  source: "curated" | "operator";
  created_by?: string;
  updated_at?: string;
}

export function fetchScanScope(): Promise<{ data: ScanScopeEntry[] }> {
  return apiFetch<{ data: ScanScopeEntry[] }>(buildUrl("/cookstyle/scan-scope"));
}

/**
 * Saving re-derives every stored verdict against the new scope and reports how
 * many moved, so a decision shows its effect immediately rather than waiting
 * for the next scan.
 */
export function saveScanScopeEntry(body: {
  pattern: string;
  excluded: boolean;
  reason: string;
}): Promise<{ status: string; verdicts_changed: number }> {
  return apiFetch<{ status: string; verdicts_changed: number }>("/api/v1/cookstyle/scan-scope", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function deleteScanScopeEntry(
  pattern: string,
): Promise<{ status: string; verdicts_changed: number }> {
  return apiFetch<{ status: string; verdicts_changed: number }>(
    buildUrl("/cookstyle/scan-scope", { pattern }),
    { method: "DELETE" },
  );
}
