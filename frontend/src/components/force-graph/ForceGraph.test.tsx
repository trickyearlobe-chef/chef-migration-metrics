// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { ForceGraph } from "./ForceGraph";
import type { GraphNode, GraphEdge } from "./types";

const sampleNodes: GraphNode[] = [
  { id: "role:web", name: "web", type: "role" },
  {
    id: "cookbook:nginx",
    name: "nginx",
    type: "cookbook",
    compatibility_status: "incompatible",
  },
  {
    id: "cookbook:apt",
    name: "apt",
    type: "cookbook",
    compatibility_status: "compatible",
  },
  {
    id: "cookbook:base",
    name: "base",
    type: "cookbook",
    compatibility_status: "untested",
  },
];

const sampleEdges: GraphEdge[] = [
  { source: "role:web", target: "cookbook:nginx", type: "cookbook" },
  { source: "role:web", target: "cookbook:apt", type: "cookbook" },
  { source: "role:web", target: "cookbook:base", type: "cookbook" },
];

function renderGraph(nodeOverrides?: GraphNode[], edgeOverrides?: GraphEdge[]) {
  return render(
    <MemoryRouter>
      <ForceGraph
        nodes={nodeOverrides ?? sampleNodes}
        edges={edgeOverrides ?? sampleEdges}
        searchTerm=""
        filterType="all"
        selectedNodeId={null}
        hoveredNodeId={null}
        onSelectNode={() => {}}
        onHoverNode={() => {}}
      />
    </MemoryRouter>,
  );
}

describe("ForceGraph", () => {
  let rafCallCount: number;

  beforeEach(() => {
    rafCallCount = 0;
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => {
      if (rafCallCount < 3) {
        rafCallCount++;
        cb(0);
      }
      return rafCallCount;
    });
    vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders SVG element", () => {
    const { container } = renderGraph();
    const svg = container.querySelector("svg");
    expect(svg).toBeInTheDocument();
  });

  it("renders role nodes as rects and cookbook nodes as circles", () => {
    const { container } = renderGraph();
    // Role nodes use <rect> elements (excluding any other rects)
    const rects = container.querySelectorAll("svg g g rect");
    expect(rects.length).toBeGreaterThanOrEqual(1);

    // Cookbook nodes use <circle> elements — filter out highlight rings
    // by checking for a fill attribute that is not "none"
    const circles = container.querySelectorAll("svg g g circle");
    const filledCircles = Array.from(circles).filter(
      (c) => c.getAttribute("fill") !== "none",
    );
    expect(filledCircles.length).toBeGreaterThanOrEqual(3);
  });

  it("renders zoom controls", () => {
    const { container } = renderGraph();
    const zoomIn = container.querySelector('button[title="Zoom in"]');
    const zoomOut = container.querySelector('button[title="Zoom out"]');
    const resetView = container.querySelector('button[title="Reset view"]');
    expect(zoomIn).toBeInTheDocument();
    expect(zoomOut).toBeInTheDocument();
    expect(resetView).toBeInTheDocument();
  });

  it("colours incompatible cookbook nodes red", () => {
    const { container } = renderGraph();
    const circles = container.querySelectorAll("svg g g circle");
    const redCircles = Array.from(circles).filter(
      (c) => c.getAttribute("fill") === "#ef4444",
    );
    expect(redCircles.length).toBeGreaterThanOrEqual(1);
  });

  it("colours compatible cookbook nodes green", () => {
    const { container } = renderGraph();
    const circles = container.querySelectorAll("svg g g circle");
    const greenCircles = Array.from(circles).filter(
      (c) => c.getAttribute("fill") === "#10b981",
    );
    expect(greenCircles.length).toBeGreaterThanOrEqual(1);
  });

  it("colours untested cookbook nodes grey", () => {
    const { container } = renderGraph();
    const circles = container.querySelectorAll("svg g g circle");
    const greyCircles = Array.from(circles).filter(
      (c) => c.getAttribute("fill") === "#9ca3af",
    );
    expect(greyCircles.length).toBeGreaterThanOrEqual(1);
  });
});
