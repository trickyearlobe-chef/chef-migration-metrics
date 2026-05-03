// SPDX-License-Identifier: Apache-2.0

import type {
  GitRepoListResponse,
  GitRepoDetailResponse,
  GitRepoRemediationResponse,
  CookbookCommittersResponse,
  CommitterAssignResponse,
  ResetGitCookbookResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";
import type { GitRepoFilterQuery } from "./client";
import type { CommitterFilterQuery } from "./ownership";

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
  filters?: CommitterFilterQuery,
): Promise<CookbookCommittersResponse> {
  return apiFetch<CookbookCommittersResponse>(
    buildUrl(
      `/git-repos/${encodeURIComponent(repoName)}/committers`,
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function assignGitRepoCommitters(
  repoName: string,
  body: {
    committers: {
      author_email: string;
      owner_name: string;
      display_name?: string;
    }[];
  },
): Promise<CommitterAssignResponse> {
  return apiFetch<CommitterAssignResponse>(
    buildUrl(`/git-repos/${encodeURIComponent(repoName)}/committers/assign`),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}

export interface GitRepoFileEntry {
  name: string;
  type: "file" | "dir";
  size?: number;
}

export interface GitRepoFileContentResponse {
  path: string;
  encoding: "text" | "base64";
  content: string;
  size: number;
}

export function fetchGitRepoFiles(
  repoName: string,
  path?: string,
): Promise<GitRepoFileEntry[]> {
  return apiFetch<GitRepoFileEntry[]>(
    buildUrl(`/git-repos/${encodeURIComponent(repoName)}/files`, { path }),
  );
}

export function fetchGitRepoFileContent(
  repoName: string,
  path: string,
): Promise<GitRepoFileContentResponse> {
  return apiFetch<GitRepoFileContentResponse>(
    buildUrl(`/git-repos/${encodeURIComponent(repoName)}/files/content`, { path }),
  );
}
