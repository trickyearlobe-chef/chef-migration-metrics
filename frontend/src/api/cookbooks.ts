// SPDX-License-Identifier: Apache-2.0

import type {
  CookbookListResponse,
  CookbookDetailResponse,
  CookbookPlatformCoverage,
  CookbookRemediationResponse,
  ResetGitCookbookResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";
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

export function requestCookbookRescan(name: string): Promise<{
  cookbook_name: string;
  versions_invalidated: number;
  message: string;
}> {
  return apiFetch<{ cookbook_name: string; versions_invalidated: number; message: string }>(
    buildUrl(`/cookbooks/${encodeURIComponent(name)}/rescan`),
    { method: "POST" },
  );
}

export function rescanAllCookstyle(): Promise<{ message: string }> {
  return apiFetch<{ message: string }>(buildUrl("/admin/rescan-all-cookstyle"), {
    method: "POST",
  });
}

export function resetGitCookbook(
  name: string,
): Promise<ResetGitCookbookResponse> {
  return apiFetch<ResetGitCookbookResponse>(
    buildUrl(`/cookbooks/${encodeURIComponent(name)}/reset-git`),
    { method: "POST" },
  );
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
