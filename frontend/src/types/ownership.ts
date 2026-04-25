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

export interface ImportResponse {
  imported: number;
  skipped: number;
  errors: ImportError[];
}

export interface ImportError {
  line: number;
  error: string;
}

export type OwnerListResponse = PaginatedResponse<Owner>;
export type AssignmentListResponse = PaginatedResponse<OwnershipAssignment>;
export type AuditLogResponse = PaginatedResponse<OwnershipAuditEntry>;
