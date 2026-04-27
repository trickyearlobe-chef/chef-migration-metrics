// SPDX-License-Identifier: Apache-2.0

import type {
  GitRepoListResponse,
  GitRepoDetailResponse,
  GitRepoRemediationResponse,
  CookbookCommittersResponse,
  ResetGitCookbookResponse,
} from "../types";
import { apiFetch, buildUrl, BASE } from "./client";
import type { GitRepoFilterQuery, PaginationQuery } from "./client";

export function fetchGitRepos(
  filters?: GitRepoFilterQuery,
): Promise<GitRepoListResponse> {
  return apiFetch<GitRepoListResponse>(
    buildUrl(
      "/git-repos",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchGitRepoDetail(
  name: string,
  organisation?: string,
): Promise<GitRepoDetailResponse> {
  return apiFetch<GitRepoDetailResponse>(
    buildUrl(`/git-repos/${encodeURIComponent(name)}`, { organisation }),
  );
}

export async function requestGitRepoRescan(name: string): Promise<{
  git_repo_name: string;
  repos_invalidated: number;
  message: string;
}> {
  const res = await fetch(
    `${BASE}/git-repos/${encodeURIComponent(name)}/rescan`,
    { method: "POST", headers: { Accept: "application/json" } },
  );
  if (!res.ok) throw new Error(`Rescan git repo failed: ${res.status}`);
  return res.json();
}

export async function resetGitRepo(
  name: string,
): Promise<ResetGitCookbookResponse> {
  const res = await fetch(
    `${BASE}/git-repos/${encodeURIComponent(name)}/reset`,
    { method: "POST", headers: { Accept: "application/json" } },
  );
  if (!res.ok) throw new Error(`Reset git repo failed: ${res.status}`);
  return res.json() as Promise<ResetGitCookbookResponse>;
}

export function fetchGitRepoRemediation(
  name: string,
  version: string,
  params?: { target_chef_version?: string },
): Promise<GitRepoRemediationResponse> {
  return apiFetch<GitRepoRemediationResponse>(
    buildUrl(
      `/git-repos/${encodeURIComponent(name)}/${encodeURIComponent(version)}/remediation`,
      params,
    ),
  );
}

export function fetchGitRepoCommitters(
  repoName: string,
  filters?: PaginationQuery,
): Promise<CookbookCommittersResponse> {
  return apiFetch<CookbookCommittersResponse>(
    buildUrl(
      `/git-repos/${encodeURIComponent(repoName)}/committers`,
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}
