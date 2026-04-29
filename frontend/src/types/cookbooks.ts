// SPDX-License-Identifier: Apache-2.0

import type { PaginatedResponse, CompatibilityStatus } from "./common";

export interface CookbookSourceVerdict {
  source: string;
  status: string;
  version?: string;
  commit_sha?: string;
  complexity_score?: number;
  complexity_label?: string;
}

export interface CookbookListItem {
  id: string;
  organisation_id?: string;
  organisation_name?: string;
  name: string;
  version: string;
  is_active: boolean;
  is_stale_cookbook: boolean;
  is_frozen?: boolean;
  download_status: string;
  download_error?: string;
  compatibility?: CompatibilityStatus;
  tk_status?: string;
  target_chef_version?: string;
  maintainer?: string;
  description?: string;
  long_description?: string;
  license?: string;
  platforms?: Record<string, string>;
  dependencies?: Record<string, string>;
  first_seen_at?: string;
  last_fetched_at?: string;
  created_at?: string;
  updated_at?: string;
}

export type CookbookListResponse = PaginatedResponse<CookbookListItem>;

export interface CookbookComplexity {
  id: string;
  cookbook_id: string;
  target_chef_version: string;
  complexity_score: number;
  complexity_label: string;
  auto_correctable_count: number;
  manual_fix_count: number;
  error_count: number;
  deprecation_count: number;
  correctness_count: number;
  modernize_count: number;
  created_at: string;
}

export interface CookstyleResult {
  id: string;
  cookbook_id: string;
  target_chef_version: string;
  passed: boolean;
  offence_count: number;
  deprecation_count: number;
  error_message?: string;
  process_stderr?: string;
  scanned_at: string;
  created_at: string;
}

export interface TestKitchenResult {
  id: string;
  cookbook_id: string;
  target_chef_version: string;
  commit_sha: string;
  converge_passed: boolean;
  tests_passed: boolean;
  compatible: boolean;
  timed_out: boolean;
  driver_used: string;
  platform_tested: string;
  duration_seconds: number;
  started_at: string;
  completed_at: string;
  created_at: string;
  converge_output?: string;
  verify_output?: string;
  destroy_output?: string;
}

export interface CookbookVersionDetail {
  cookbook: CookbookListItem;
  cookstyle?: CookstyleResult[];
  test_kitchen?: TestKitchenResult[];
}

export interface ServerCookbookVersionDetail {
  cookbook: CookbookListItem;
  cookstyle?: CookstyleResult[];
}

export interface GitRepoListItem {
  id: string;
  name: string;
  git_repo_url: string;
  head_commit_sha?: string;
  default_branch?: string;
  has_test_suite: boolean;
  clone_status: string;
  clone_error?: string;
  last_fetched_at?: string;
  compatibility?: CompatibilityStatus;
  target_chef_version?: string;
  tk_status?: string;
  tk_passed?: number;
  tk_total?: number;
}

export type GitRepoListResponse = PaginatedResponse<GitRepoListItem>;

export interface GitRepoDetail {
  git_repo: GitRepoListItem;
  cookstyle?: CookstyleResult[];
  test_kitchen?: TestKitchenResult[];
  complexity?: CookbookComplexity[];
}

export interface CookbookDetailResponse {
  name: string;
  server_cookbooks: ServerCookbookVersionDetail[];
  git_repos: GitRepoDetail[];
}

export interface GitRepoDetailResponse {
  name: string;
  git_repos: GitRepoDetail[];
}

export interface ProductionPlatform {
  platform: string;
  platform_version: string;
  platform_family: string;
  node_count: number;
}

export interface TestedPlatformMatch {
  kitchen_name: string;
  platform: string;
  platform_version: string;
  node_count: number;
}

export interface CoverageReport {
  kitchen_platforms: string[];
  production_platforms: ProductionPlatform[];
  tested_and_in_production: TestedPlatformMatch[];
  tested_not_in_production: string[];
  in_production_not_tested: ProductionPlatform[];
  gap_count: number;
  total_production_nodes: number;
  covered_node_count: number;
  coverage_percentage: number;
}

export interface CookbookPlatformCoverage {
  id: string;
  git_repo_id?: string;
  cookbook_name: string;
  coverage_data: CoverageReport;
  evaluated_at: string;
  created_at: string;
  updated_at: string;
}
