// SPDX-License-Identifier: Apache-2.0

import type {
  GitRepoListResponse,
  GitRepoDetailResponse,
  GitRepoRemediationResponse,
  CookbookCommittersResponse,
  ResetGitCookbookResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";
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

export function requestGitRepoRescan(name: string): Promise<{
  git_repo_name: string;
  repos_invalidated: number;
  message: string;
}> {
  return apiFetch<{ git_repo_name: string; repos_invalidated: number; message: string }>(
    buildUrl(`/git-repos/${encodeURIComponent(name)}/rescan`),
    { method: "POST" },
  );
}

export function resetGitRepo(
  name: string,
): Promise<ResetGitCookbookResponse> {
  return apiFetch<ResetGitCookbookResponse>(
    buildUrl(`/git-repos/${encodeURIComponent(name)}/reset`),
    { method: "POST" },
  );
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
