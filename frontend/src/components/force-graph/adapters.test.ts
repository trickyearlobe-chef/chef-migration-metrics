// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import type { DependencyGraphNode, DependencyGraphEdge } from "../../types/dependencies";
import type { RoleGraphNode, RoleGraphEdge } from "../../types/roles";
import {
  adaptDependencyNodes,
  adaptDependencyEdges,
  adaptRoleGraphNodes,
  adaptRoleGraphEdges,
} from "./adapters";

describe("adaptDependencyNodes", () => {
  it("passes through id, name, and type", () => {
    const input: DependencyGraphNode[] = [
      { id: "role:web", name: "web", type: "role" },
      { id: "cookbook:nginx", name: "nginx", type: "cookbook" },
    ];
    const result = adaptDependencyNodes(input);
    expect(result).toEqual([
      { id: "role:web", name: "web", type: "role" },
      { id: "cookbook:nginx", name: "nginx", type: "cookbook" },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(adaptDependencyNodes([])).toEqual([]);
  });

  it("does not include compatibility_status or complexity_label", () => {
    const input: DependencyGraphNode[] = [
      { id: "role:base", name: "base", type: "role" },
    ];
    const result = adaptDependencyNodes(input);
    expect(result[0]).not.toHaveProperty("compatibility_status");
    expect(result[0]).not.toHaveProperty("complexity_label");
  });
});

describe("adaptDependencyEdges", () => {
  it("maps dependency_type to type", () => {
    const input: DependencyGraphEdge[] = [
      { source: "role:web", target: "cookbook:nginx", dependency_type: "cookbook" },
      { source: "role:web", target: "role:base", dependency_type: "role" },
    ];
    const result = adaptDependencyEdges(input);
    expect(result).toEqual([
      { source: "role:web", target: "cookbook:nginx", type: "cookbook" },
      { source: "role:web", target: "role:base", type: "role" },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(adaptDependencyEdges([])).toEqual([]);
  });
});

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
