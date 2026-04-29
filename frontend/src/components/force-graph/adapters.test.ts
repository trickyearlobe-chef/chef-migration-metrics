// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import type { RoleGraphNode, RoleGraphEdge } from "../../types/roles";
import type { NodeGraphNode, NodeGraphEdge } from "../../types/nodes";
import {
  adaptRoleGraphNodes,
  adaptRoleGraphEdges,
  adaptNodeGraphNodes,
  adaptNodeGraphEdges,
} from "./adapters";

describe("adaptRoleGraphNodes", () => {
  it("passes through id, name, type, compatibility_status, and complexity_label", () => {
    const input: RoleGraphNode[] = [
      {
        id: "role:web",
        name: "web",
        type: "role",
        compatibility_status: "compatible",
        complexity_label: "low",
      },
      {
        id: "cookbook:nginx",
        name: "nginx",
        type: "cookbook",
        compatibility_status: "incompatible",
        complexity_label: "high",
      },
    ];
    const result = adaptRoleGraphNodes(input);
    expect(result).toEqual([
      {
        id: "role:web",
        name: "web",
        type: "role",
        compatibility_status: "compatible",
        complexity_label: "low",
      },
      {
        id: "cookbook:nginx",
        name: "nginx",
        type: "cookbook",
        compatibility_status: "incompatible",
        complexity_label: "high",
      },
    ]);
  });

  it("handles missing optional fields", () => {
    const input: RoleGraphNode[] = [
      { id: "role:base", name: "base", type: "role" },
    ];
    const result = adaptRoleGraphNodes(input);
    expect(result).toEqual([
      {
        id: "role:base",
        name: "base",
        type: "role",
        compatibility_status: undefined,
        complexity_label: undefined,
      },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(adaptRoleGraphNodes([])).toEqual([]);
  });
});

describe("adaptRoleGraphEdges", () => {
  it("maps from to source and to to target", () => {
    const input: RoleGraphEdge[] = [
      { from: "role:web", to: "cookbook:nginx", type: "includes_cookbook" },
      { from: "role:web", to: "role:base", type: "includes_role" },
    ];
    const result = adaptRoleGraphEdges(input);
    expect(result).toEqual([
      { source: "role:web", target: "cookbook:nginx", type: "includes_cookbook" },
      { source: "role:web", target: "role:base", type: "includes_role" },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(adaptRoleGraphEdges([])).toEqual([]);
  });
});

describe("adaptNodeGraphNodes", () => {
  it("passes through id, name, type, compatibility_status, and tk_status", () => {
    const input: NodeGraphNode[] = [
      {
        id: "run_list_entry:role[web]",
        name: "role[web]",
        type: "run_list_entry",
      },
      {
        id: "cookbook:nginx",
        name: "nginx",
        type: "cookbook",
        compatibility_status: "incompatible",
        tk_status: "failed",
      },
    ];
    const result = adaptNodeGraphNodes(input);
    expect(result).toEqual([
      {
        id: "run_list_entry:role[web]",
        name: "role[web]",
        type: "run_list_entry",
        compatibility_status: undefined,
        tk_status: undefined,
      },
      {
        id: "cookbook:nginx",
        name: "nginx",
        type: "cookbook",
        compatibility_status: "incompatible",
        tk_status: "failed",
      },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(adaptNodeGraphNodes([])).toEqual([]);
  });
});

describe("adaptNodeGraphEdges", () => {
  it("maps from to source and to to target", () => {
    const input: NodeGraphEdge[] = [
      { from: "run_list_entry:role[web]", to: "role:web", type: "includes_role" },
      { from: "role:web", to: "cookbook:nginx", type: "includes_cookbook" },
    ];
    const result = adaptNodeGraphEdges(input);
    expect(result).toEqual([
      { source: "run_list_entry:role[web]", target: "role:web", type: "includes_role" },
      { source: "role:web", target: "cookbook:nginx", type: "includes_cookbook" },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(adaptNodeGraphEdges([])).toEqual([]);
  });
});
