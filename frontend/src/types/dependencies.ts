// SPDX-License-Identifier: Apache-2.0

import type { Pagination } from "./common";

export interface DependencyGraphNode {
  id: string;
  name: string;
  type: "role" | "cookbook";
}

export interface DependencyGraphEdge {
  source: string;
  target: string;
  dependency_type: "role" | "cookbook";
}

export interface DependencyGraphSummary {
  total_nodes: number;
  total_edges: number;
  role_count: number;
  cookbook_count: number;
}

export interface DependencyGraphResponse {
  organisation: string;
  summary: DependencyGraphSummary;
  nodes: DependencyGraphNode[];
  edges: DependencyGraphEdge[];
}

export interface DependencyEntry {
  name: string;
  type: "role" | "cookbook";
}

export interface DependencyTableRow {
  role_name: string;
  cookbook_count: number;
  role_count: number;
  total_dependencies: number;
  depended_on_by: number;
  dependencies: DependencyEntry[];
}

export interface SharedCookbook {
  cookbook_name: string;
  role_count: number;
}

export interface DependencyGraphTableResponse {
  organisation: string;
  total_roles: number;
  shared_cookbooks: SharedCookbook[];
  data: DependencyTableRow[];
  pagination: Pagination;
}
