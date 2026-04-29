// SPDX-License-Identifier: Apache-2.0

import type { Pagination } from "./common";

export interface RoleCompatibility {
  target_chef_version: string;
  status: string;
  compatible_count: number;
  incompatible_count: number;
  untested_count: number;
}

export interface RoleListItem {
  role_name: string;
  organisations: string[];
  node_count: number;
  direct_cookbook_count: number;
  transitive_cookbook_count: number;
  total_cookbook_count: number;
  compatibility_status: string;
  compatible_count: number;
  incompatible_count: number;
  untested_count: number;
  tk_status?: string;
}

export interface RoleSummary {
  target_chef_version: string;
  compatible_roles: number;
  incompatible_roles: number;
  untested_roles: number;
  total_roles: number;
}

export interface RoleListResponse {
  data: RoleListItem[];
  summary: RoleSummary;
  pagination: Pagination;
}

export interface RoleBlockingCookbook {
  cookbook_name: string;
  cookbook_version: string;
  target_chef_version: string;
  complexity_score: number;
  complexity_label: string;
  auto_correctable: number;
  manual_fix: number;
  dependency_path: string[];
}

export interface RoleChainNode {
  name: string;
  type: "role" | "cookbook";
  compatibility_status?: string;
  source?: "server" | "git" | "both";
  tk_status?: "passed" | "failed" | "partial" | "untested";
  complexity_score?: number;
  children?: RoleChainNode[];
}

export interface RoleOrgCount {
  organisation: string;
  count: number;
}

export interface RoleEnvCount {
  environment: string;
  count: number;
}

export interface RolePlatformCount {
  platform: string;
  platform_version: string;
  count: number;
}

export interface RoleDetailResponse {
  role_name: string;
  organisations: string[];
  node_count: number;
  direct_cookbooks: string[];
  direct_roles: string[];
  transitive_cookbooks: string[];
  blocking_cookbooks: RoleBlockingCookbook[];
  nested_role_chain: RoleChainNode;
  nodes_by_organisation: RoleOrgCount[];
  nodes_by_environment: RoleEnvCount[];
  nodes_by_platform: RolePlatformCount[];
}

export interface RoleGraphNode {
  id: string;
  type: string;
  name: string;
  compatibility_status?: string;
  complexity_label?: string;
}

export interface RoleGraphEdge {
  from: string;
  to: string;
  type: string;
}

export interface RoleGraphResponse {
  nodes: RoleGraphNode[];
  edges: RoleGraphEdge[];
  metadata: {
    total_roles: number;
    total_cookbooks: number;
    incompatible_cookbooks: number;
  };
}
