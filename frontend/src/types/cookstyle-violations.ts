// SPDX-License-Identifier: Apache-2.0

import type { Pagination } from "./common";

// ---------------------------------------------------------------------------
// Cop Analysis (classification-aware aggregation)
// ---------------------------------------------------------------------------

export type CopClassification = "blocker" | "review" | "noise";
export type ClassificationSource =
  | "operator_override"
  | "custom_cop"
  | "verified_removal"
  | "structural_noise"
  | "review_default";

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
  // Retained for backwards compatibility with the API payload; under the
  // "trustworthy reds" model there is no unclassified level, so this is always 0.
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

// ---------------------------------------------------------------------------
// Cop drift / coverage report (live registry vs. static classification tables)
// ---------------------------------------------------------------------------

// StaleCopEntry is a static-table cop the running cookstyle binary no longer
// emits. The only static classification table is the removed-in mapping, so
// `source` is "removed_in_mapping".
export interface StaleCopEntry {
  cop_name: string;
  source: string;
}

// CoverageGapEntry is a live Chef/* cop with no explicit classification — it
// resolves to the review default (the honest worklist bucket) rather than to a
// blocker or noise decision.
export interface CoverageGapEntry {
  cop_name: string;
  department: string;
  enabled: boolean;
}

export interface CopDriftReport {
  registry_available: boolean;
  registry_version: string;
  // Nil Go slices serialise as null, so treat these as possibly absent.
  stale: StaleCopEntry[] | null;
  coverage_gaps: CoverageGapEntry[] | null;
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
