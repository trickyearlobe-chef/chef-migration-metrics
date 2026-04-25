// SPDX-License-Identifier: Apache-2.0

import type { Pagination, ComplexityLabel } from "./common";

export interface RemediationPriorityItem {
  cookbook_name: string;
  cookbook_version?: string;
  cookbook_id: string;
  organisation_id?: string;
  complexity_score: number;
  complexity_label: ComplexityLabel | string;
  affected_node_count: number;
  affected_role_count: number;
  priority_score: number;
  auto_correctable_count: number;
  manual_fix_count: number;
  deprecation_count: number;
  error_count: number;
  target_chef_version: string;
  version_count: number;
}

export interface RemediationPriorityResponse {
  target_chef_version: string;
  total_cookbooks: number;
  total_auto_correctable: number;
  total_manual_fix: number;
  total_deprecations: number;
  total_errors: number;
  data: RemediationPriorityItem[];
  pagination: Pagination;
}

export interface RemediationSummaryResponse {
  target_chef_version: string;
  total_cookbooks_evaluated: number;
  total_needing_remediation: number;
  quick_wins: number;
  manual_fixes: number;
  blocked_nodes_by_complexity: number;
  blocked_nodes_by_readiness: number;
  total_auto_correctable: number;
  total_manual_fix: number;
}

export interface RemediationOffenseLocation {
  file: string;
  start_line: number;
  start_column: number;
  last_line: number;
  last_column: number;
}

export interface RemediationOffense {
  cop_name: string;
  severity: string;
  message: string;
  correctable: boolean;
  location: RemediationOffenseLocation;
}

export interface CopRemediation {
  cop_name: string;
  description: string;
  migration_url: string;
  introduced_in?: string;
  removed_in?: string;
  replacement_pattern?: string;
}

export interface OffenseGroup {
  cop_name: string;
  severity: string;
  count: number;
  correctable_count: number;
  remediation?: CopRemediation | null;
  offenses: RemediationOffense[];
}

export interface AutocorrectPreview {
  available: boolean;
  total_offenses: number;
  correctable_offenses: number;
  remaining_offenses: number;
  files_modified: number;
  diff_output: string;
  generated_at?: string;
}

export interface RemediationStatistics {
  total_offenses: number;
  correctable_offenses: number;
  remaining_offenses: number;
  auto_correctable_count: number;
  manual_fix_count: number;
  deprecation_count: number;
  error_count: number;
  offense_groups: number;
}

export interface CookbookRemediationResponse {
  cookbook_name: string;
  cookbook_version: string;
  target_chef_version: string;
  complexity_score: number;
  complexity_label: ComplexityLabel | string;
  cookstyle_passed: boolean | null;
  scanned_at: string;
  statistics: RemediationStatistics;
  offense_groups: OffenseGroup[];
  autocorrect_preview: AutocorrectPreview;
}

export interface GitRepoRemediationResponse {
  git_repo_name: string;
  version: string;
  target_chef_version: string;
  source: string;
  complexity_score: number;
  complexity_label: ComplexityLabel | string;
  cookstyle_passed: boolean | null;
  scanned_at: string;
  statistics: RemediationStatistics;
  offense_groups: OffenseGroup[];
  autocorrect_preview: AutocorrectPreview;
}
