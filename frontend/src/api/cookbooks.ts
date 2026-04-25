// SPDX-License-Identifier: Apache-2.0

import type {
  CookbookListResponse,
  CookbookDetailResponse,
  CookbookPlatformCoverage,
  CookbookRemediationResponse,
  ResetGitCookbookResponse,
} from "../types";
import { apiFetch, buildUrl, BASE } from "./client";
import type { CookbookFilterQuery } from "./client";

export function fetchCookbooks(
  filters?: CookbookFilterQuery,
): Promise<CookbookListResponse> {
  return apiFetch<CookbookListResponse>(
    buildUrl(
      "/cookbooks",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchCookbookDetail(
  name: string,
  organisation?: string,
): Promise<CookbookDetailResponse> {
  return apiFetch<CookbookDetailResponse>(
    buildUrl(`/cookbooks/${encodeURIComponent(name)}`, { organisation }),
  );
}

export function fetchCookbookPlatformCoverage(
  cookbookName: string,
  organisation?: string,
): Promise<CookbookPlatformCoverage> {
  return apiFetch<CookbookPlatformCoverage>(
    buildUrl(
      `/cookbooks/${encodeURIComponent(cookbookName)}/platform-coverage`,
      { organisation },
    ),
  );
}

export async function requestCookbookRescan(name: string): Promise<{
  cookbook_name: string;
  versions_invalidated: number;
  message: string;
}> {
  const res = await fetch(
    `${BASE}/cookbooks/${encodeURIComponent(name)}/rescan`,
    { method: "POST", headers: { Accept: "application/json" } },
  );
  if (!res.ok) throw new Error(`Rescan failed: ${res.status}`);
  return res.json();
}

export async function rescanAllCookstyle(): Promise<{ message: string }> {
  const res = await fetch(buildUrl("/admin/rescan-all-cookstyle"), {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`Rescan all failed: ${res.status}`);
  return res.json() as Promise<{ message: string }>;
}

export async function rerunAllTestKitchen(): Promise<{ message: string }> {
  const res = await fetch(buildUrl("/admin/rerun-all-test-kitchen"), {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`Rerun all TK failed: ${res.status}`);
  return res.json() as Promise<{ message: string }>;
}

export async function resetGitCookbook(
  name: string,
): Promise<ResetGitCookbookResponse> {
  const res = await fetch(
    `${BASE}/cookbooks/${encodeURIComponent(name)}/reset-git`,
    { method: "POST", headers: { Accept: "application/json" } },
  );
  if (!res.ok) throw new Error(`Reset git cookbook failed: ${res.status}`);
  return res.json() as Promise<ResetGitCookbookResponse>;
}

export function fetchCookbookRemediation(
  name: string,
  version: string,
  params?: { target_chef_version?: string },
): Promise<CookbookRemediationResponse> {
  return apiFetch<CookbookRemediationResponse>(
    buildUrl(
      `/cookbooks/${encodeURIComponent(name)}/${encodeURIComponent(version)}/remediation`,
      params,
    ),
  );
}
