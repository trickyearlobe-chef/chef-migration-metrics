// SPDX-License-Identifier: Apache-2.0

export interface GraphNode {
  id: string;
  name: string;
  type: "role" | "cookbook";
  compatibility_status?: string;
  complexity_label?: string;
}

export interface GraphEdge {
  source: string;
  target: string;
  type: string;
}

export interface SimNode {
  id: string;
  name: string;
  type: "role" | "cookbook";
  compatibility_status?: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
  fx: number | null;
  fy: number | null;
}

export interface SimEdge {
  source: string;
  target: string;
  type: string;
}
