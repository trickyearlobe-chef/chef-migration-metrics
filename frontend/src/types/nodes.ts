// SPDX-License-Identifier: Apache-2.0

import type { PaginatedResponse } from "./common";
import type { CookbookSourceVerdict } from "./cookbooks";

export interface NodeReadinessSummary {
  target_chef_version: string;
  is_ready: boolean;
  all_cookbooks_compatible: boolean;
  sufficient_disk_space: boolean | null;
  blocking_cookbook_count: number;
  stale_data: boolean;
  disk_status?: "sufficient" | "insufficient" | "unknown";
  cookstyle_status?: "passed" | "failed" | "unknown";
  kitchen_status?: "passed" | "failed" | "partial" | "unknown";
  disk_detail?: string | null;
  cookstyle_detail?: string | null;
  kitchen_detail?: string | null;
}

export interface NodeListItem {
  id: string;
  organisation_id: string;
  organisation_name: string;
  node_name: string;
  chef_environment?: string;
  chef_version?: string;
  platform?: string;
  platform_version?: string;
  platform_family?: string;
  platform_display_name?: string | null;
  policy_name?: string;
  policy_group?: string;
  is_stale: boolean;
  staleness_tier?: "fresh" | "warning" | "critical";
  ohai_time?: number;
  ohai_time_age_hours?: number;
  collected_at: string;
  readiness?: NodeReadinessSummary[];
}

export type NodeListResponse = PaginatedResponse<NodeListItem>;

export interface NodeSnapshot {
  id: string;
  collection_run_id: string;
  organisation_id: string;
  node_name: string;
  chef_environment: string;
  chef_version: string;
  platform: string;
  platform_version: string;
  platform_family: string;
  platform_display_name?: string | null;
  filesystem: Record<string, unknown> | null;
  cookbooks: Record<string, unknown> | null;
  run_list: string[] | null;
  roles: string[] | null;
  policy_name: string;
  policy_group: string;
  ohai_time: number;
  is_stale: boolean;
  staleness_tier?: "fresh" | "warning" | "critical";
  ohai_time_age_hours?: number;
  collected_at: string;
  created_at: string;
}

export interface BlockingCookbook {
  name: string;
  version: string;
  reason: string;
  source: string;
  complexity_score: number;
  complexity_label: string;
  verdicts?: CookbookSourceVerdict[];
}

export interface NodeReadiness {
  id: string;
  node_snapshot_id: string;
  organisation_id: string;
  node_name: string;
  target_chef_version: string;
  is_ready: boolean;
  all_cookbooks_compatible: boolean;
  sufficient_disk_space: boolean | null;
  blocking_cookbooks: BlockingCookbook[] | null;
  available_disk_mb: number | null;
  required_disk_mb: number | null;
  stale_data: boolean;
  cookstyle_status?: string;
  kitchen_status?: string;
  evaluated_at: string;
  created_at: string;
  updated_at: string;
}

export interface NodeDetailResponse {
  node: NodeSnapshot;
  organisation_name: string;
  readiness: NodeReadiness[] | null;
}

export interface NodesByVersionResponse {
  chef_version: string;
  total: number;
  data: NodeSnapshot[];
}

export interface NodeWithOrg {
  organisation_name: string;
  node: NodeSnapshot;
}

export interface NodesByCookbookResponse {
  cookbook_name: string;
  total: number;
  data: NodeWithOrg[];
}

export interface DiskEntry {
  mount: string;
  device: string;
  fs_type: string;
  kb_size: number;
  kb_used: number;
  kb_available: number;
  percent_used: number;
  uuid?: string;
  mount_options?: string[];
  inodes_used?: number;
  total_inodes?: number;
  inodes_available?: number;
  inodes_percent_used?: number;
  drive_type?: string;
  volume_name?: string;
  encryption_status?: string;
}

export interface NodeDiskDetailResponse {
  node_name: string;
  organisation_name: string;
  platform: string;
  disks: DiskEntry[];
}

export interface NodeGraphNode {
  id: string;
  type: string;
  name: string;
  compatibility_status?: string;
  tk_status?: string;
  complexity_label?: string;
  source?: string;
}

export interface NodeGraphEdge {
  from: string;
  to: string;
  type: string;
}

export interface NodeDependencyGraphResponse {
  nodes: NodeGraphNode[];
  edges: NodeGraphEdge[];
  metadata: {
    total_roles: number;
    total_cookbooks: number;
    incompatible_cookbooks: number;
    tk_failed_cookbooks: number;
  };
}
