// SPDX-License-Identifier: Apache-2.0

import type { Pagination } from "./common";

// ---------------------------------------------------------------------------
// Cop Analysis (classification-aware aggregation)
// ---------------------------------------------------------------------------

export type CopClassification = "blocker" | "review" | "noise" | "unclassified";
export type ClassificationSource =
  | "operator_override"
  | "removed_in"
  | "curated_default"
  | "unclassified";

export interface CopAggregateItem {
  cop_name: string;
  description: string;
  category: string;
  severity: string;
  classification: CopClassification;
  classification_source: ClassificationSource;
  removed_in?: string;
  introduced_in?: string;
  migration_url?: string;
  cookbooks_affected: number;
  total_offences: number;
  auto_correctable_pct: number;
  unblocks: number;
  is_custom: boolean;
}

export interface CopAggregationSummary {
  blocker_cops: number;
  blocker_cookbooks: number;
  review_cops: number;
  review_cookbooks: number;
  noise_cops: number;
  unclassified_cops: number;
}

export interface CopAggregationResponse {
  summary: CopAggregationSummary;
  data: CopAggregateItem[];
  pagination: Pagination;
}

export interface CopCookbookItem {
  source: string;
  name: string;
  version: string;
  organisation?: string;
  offence_count: number;
  auto_correctable: number;
  would_pass_without: boolean;
}

export interface CopCookbookResponse {
  cop_name: string;
  data: CopCookbookItem[];
  pagination: Pagination;
}

export interface CustomCopDefinition {
  id?: string;
  cop_name: string;
  description: string;
  pattern_type: "regex" | "literal";
  pattern: string;
  file_glob: string;
  target_chef_version_min?: string;
  removed_in?: string;
  classification: CopClassification;
  enabled: boolean;
  created_at?: string;
  updated_at?: string;
}
