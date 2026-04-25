// SPDX-License-Identifier: Apache-2.0
// Git repo types are co-located with cookbook types to avoid circular imports.
// This file re-exports them for domain-based import convenience.

export type {
  GitRepoListItem,
  GitRepoListResponse,
  GitRepoDetail,
  GitRepoDetailResponse,
} from "./cookbooks";

export interface GitRepoCommitter {
  git_repo_url: string;
  author_name: string;
  author_email: string;
  commit_count: number;
  first_commit_at: string;
  last_commit_at: string;
  collected_at: string;
  is_owner?: boolean;
}

import type { Pagination } from "./common";

export interface CookbookCommittersResponse {
  cookbook_name: string;
  git_repo_url: string;
  data: GitRepoCommitter[];
  pagination: Pagination;
}

export interface CommitterAssignResponse {
  owners_created: number;
  assignments_created: number;
  skipped: number;
}

export interface ResetGitCookbookResponse {
  cookbook_name?: string;
  git_repo_name?: string;
  repos_deleted: number;
  committers_deleted: number;
  repo_urls_removed: string[];
  local_clone_removed: boolean;
  message: string;
}
