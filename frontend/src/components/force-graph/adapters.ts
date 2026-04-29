// SPDX-License-Identifier: Apache-2.0

import type { DependencyGraphNode, DependencyGraphEdge } from "../../types/dependencies";
import type { RoleGraphNode, RoleGraphEdge } from "../../types/roles";
import type { NodeGraphNode, NodeGraphEdge } from "../../types/nodes";
import type { GraphNode, GraphEdge } from "./types";

export function adaptDependencyNodes(nodes: DependencyGraphNode[]): GraphNode[] {
  return nodes.map((n) => ({ id: n.id, name: n.name, type: n.type }));
}

export function adaptDependencyEdges(edges: DependencyGraphEdge[]): GraphEdge[] {
  return edges.map((e) => ({ source: e.source, target: e.target, type: e.dependency_type }));
}

export function adaptRoleGraphNodes(nodes: RoleGraphNode[]): GraphNode[] {
  return nodes.map((n) => ({
    id: n.id,
    name: n.name,
    type: n.type as "role" | "cookbook",
    compatibility_status: n.compatibility_status,
    complexity_label: n.complexity_label,
  }));
}

export function adaptRoleGraphEdges(edges: RoleGraphEdge[]): GraphEdge[] {
  return edges.map((e) => ({ source: e.from, target: e.to, type: e.type }));
}

export function adaptNodeGraphNodes(nodes: NodeGraphNode[]): GraphNode[] {
  return nodes.map((n) => ({
    id: n.id,
    name: n.name,
    type: n.type as "role" | "cookbook" | "run_list_entry",
    compatibility_status: n.compatibility_status,
    tk_status: n.tk_status,
  }));
}

export function adaptNodeGraphEdges(edges: NodeGraphEdge[]): GraphEdge[] {
  return edges.map((e) => ({ source: e.from, target: e.to, type: e.type }));
}
