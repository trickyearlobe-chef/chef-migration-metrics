// SPDX-License-Identifier: Apache-2.0

import type { PaginatedResponse } from "./common";

export type OwnerType =
  | "team"
  | "individual"
  | "business_unit"
  | "cost_centre"
  | "custom";
export type EntityType = "node" | "cookbook" | "git_repo" | "role" | "policy";
export type AssignmentSource = "manual" | "auto_rule" | "import";
export type OwnershipConfidence = "definitive" | "inferred";

export interface AssignmentCounts {
  node: number;
  cookbook: number;
  git_repo: number;
  role: number;
  policy: number;
}

export interface OwnerReadiness {
  target_chef_version: string;
  total_nodes: number;
  ready: number;
  blocked: number;
  stale: number;
}

export interface Owner {
  name: string;
  display_name?: string;
  contact_email?: string;
  contact_channel?: string;
  owner_type: OwnerType;
  metadata?: Record<string, unknown>;
  assignment_counts?: AssignmentCounts;
  readiness?: OwnerReadiness;
  created_at: string;
  updated_at: string;
}

export interface OwnerDetail extends Owner {
  readiness_summary?: OwnerReadinessSummary;
  cookbook_summary?: OwnerCookbookSummary;
  git_repo_summary?: OwnerGitRepoSummary;
}

export interface BlockingCookbookSummary {
  cookbook_name: string;
  complexity_label: string;
  affected_node_count: number;
}

export interface OwnerReadinessSummary {
  target_chef_version: string;
  total_nodes: number;
  ready: number;
  blocked: number;
  stale: number;
  blocking_cookbooks: BlockingCookbookSummary[];
}

export interface OwnerCookbookSummary {
  total: number;
  compatible: number;
  incompatible: number;
  untested: number;
}

export interface OwnerGitRepoSummary {
  total: number;
  compatible: number;
  incompatible: number;
}

export interface OwnershipAssignment {
  id: string;
  owner_id: string;
  owner_name: string;
  entity_type: EntityType;
  entity_key: string;
  organisation_id?: string;
  organisation_name?: string;
  assignment_source: AssignmentSource;
  auto_rule_name?: string;
  confidence: OwnershipConfidence;
  notes?: string;
  created_at: string;
  updated_at: string;
}

export interface OwnershipLookupResult {
  owner_name: string;
  owner_type: OwnerType;
  assignment_source: AssignmentSource;
  confidence: OwnershipConfidence;
  resolution: string;
}

export interface OwnershipLookupResponse {
  entity_type: string;
  entity_key: string;
  organisation: string;
  owners: OwnershipLookupResult[];
}

export interface OwnershipAuditEntry {
  id: string;
  timestamp: string;
  action: string;
  actor: string;
  owner_name: string;
  entity_type?: string;
  entity_key?: string;
  organisation?: string;
  details?: Record<string, unknown>;
}

export interface ReassignResponse {
  reassigned: number;
  skipped: number;
  from_owner: string;
  to_owner: string;
  source_owner_deleted: boolean;
}

export type OwnerListResponse = PaginatedResponse<Owner>;
export type AssignmentListResponse = PaginatedResponse<OwnershipAssignment>;
export type AuditLogResponse = PaginatedResponse<OwnershipAuditEntry>;

// ---------------------------------------------------------------------------
// Discovery-driven ownership intake
//
// The source's shape is not known in advance, so the administrator profiles the
// file, maps its columns onto these fields, previews the result, then commits.
// See journeys/ownership-intake.md.
// ---------------------------------------------------------------------------

export type IntakeTargetField =
  | "owner"
  | "entity_type"
  | "entity_key"
  | "organisation"
  | "notes"
  | "display_name";

export interface IntakeColumnProfile {
  name: string;
  sample_values: string[];
  non_empty_pct: number;
  distinct_count: number;
  /** distinct_count stopped at the tracking cap, so it is a floor rather than
   * a total. */
  distinct_capped?: boolean;
}

export interface IntakeSourceProfile {
  columns: IntakeColumnProfile[];
  row_count: number;
  /** Source rows skipped by the import filter. Not failures — they were
   * asked to be left out. */
  filtered_out?: number;
  /** The per-row detail was capped for display. Counts and the commit are
   * unaffected: every row was processed. */
  rows_truncated?: boolean;
  malformed_rows: number;
  warnings: string[];
}

export interface IntakeSource {
  kind: "column" | "constant" | "concat";
  column?: string;
  value?: string;
  columns?: string[];
  separator?: string;
}

export interface IntakeTransform {
  kind: string;
  value?: string;
  from?: string;
  to?: string;
  pattern?: string;
}

export interface IntakeFieldMapping {
  source: IntakeSource;
  transforms?: IntakeTransform[];
}

export type IntakeFieldMap = Partial<Record<IntakeTargetField, IntakeFieldMapping>>;

export interface IntakeOwnerSuggestion {
  owner_name: string;
  score: number;
}

export interface IntakeReportRow {
  source_row: number;
  malformed: boolean;
  raw: Record<string, string>;
  owner: string;
  owner_raw: string;
  entity_type: string;
  entity_key: string;
  organisation: string;
  notes: string;
  display_name: string;
  rejected_reason?: string;
  owner_match: string;
  entity_match: string;
  outcome: string;
  creates_owner: boolean;
  existing_owners?: string[];
  alias_conflict: boolean;
  alias_conflict_owner?: string;
  owner_suggestions?: IntakeOwnerSuggestion[];
}

export interface IntakeUnmatchedOwner {
  value: string;
  count: number;
}

export interface IntakeNewOwner {
  name: string;
  display_name: string;
  source_value: string;
  row_count: number;
  /** Source rows skipped by the import filter. Not failures — they were
   * asked to be left out. */
  filtered_out?: number;
  /** The per-row detail was capped for display. Counts and the commit are
   * unaffected: every row was processed. */
  rows_truncated?: boolean;
  suggestions?: IntakeOwnerSuggestion[];
}

export interface IntakeReport {
  rows: IntakeReportRow[];
  new_owners: IntakeNewOwner[];
  counts: Record<string, number>;
  alias_conflict_count: number;
  row_count: number;
  /** Source rows skipped by the import filter. Not failures — they were
   * asked to be left out. */
  filtered_out?: number;
  /** The per-row detail was capped for display. Counts and the commit are
   * unaffected: every row was processed. */
  rows_truncated?: boolean;
  unmatched_owners: IntakeUnmatchedOwner[];
  committed: boolean;
  created: number;
}

export interface IntakeMapping {
  id: number;
  name: string;
  source_kind: string;
  delimiter: string;
  field_map?: IntakeFieldMap;
  created_by: string;
  created_at: string;
  updated_at: string;

  // Where a database import reads from. db_connection names a connection set
  // up on the import screen; the password behind it never reaches the browser.
  db_connection?: string;
  db_query?: string;

  filter_column?: string;
  filter_value?: string;
  create_owners?: boolean;

  // Standard 5-field cron. Empty means unscheduled.
  schedule?: string;
  schedule_enabled?: boolean;

  // What the last unattended run did, so a silently failing import shows up
  // without anybody reading the audit log.
  last_run_at?: string;
  last_run_status?: string;
  last_run_detail?: string;
}
