// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useRef, useMemo } from "react";
import { Link } from "react-router-dom";
import type { GraphNode, GraphEdge, SimNode } from "./types";

// ---------------------------------------------------------------------------
// Force-directed graph component (SVG + requestAnimationFrame simulation)
// ---------------------------------------------------------------------------

export interface ForceGraphProps {
  nodes: GraphNode[];
  edges: GraphEdge[];
  searchTerm: string;
  filterType: "all" | "role" | "cookbook";
  selectedNodeId: string | null;
  hoveredNodeId: string | null;
  onSelectNode: (id: string | null) => void;
  onHoverNode: (id: string | null) => void;
}

export function ForceGraph({
  nodes,
  edges,
  searchTerm,
  filterType,
  selectedNodeId,
  hoveredNodeId,
  onSelectNode,
  onHoverNode,
}: ForceGraphProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const simNodesRef = useRef<SimNode[]>([]);
  const animRef = useRef<number>(0);
  const [, forceRender] = useState(0);
  const tickCountRef = useRef(0);

  // Drag state
  const dragRef = useRef<{
    nodeId: string | null;
    startX: number;
    startY: number;
    offsetX: number;
    offsetY: number;
  }>({ nodeId: null, startX: 0, startY: 0, offsetX: 0, offsetY: 0 });

  // Pan/zoom state
  const [transform, setTransform] = useState({ x: 0, y: 0, scale: 1 });
  const panRef = useRef<{
    active: boolean;
    startX: number;
    startY: number;
    origTx: number;
    origTy: number;
  }>({
    active: false,
    startX: 0,
    startY: 0,
    origTx: 0,
    origTy: 0,
  });

  // Initialize simulation nodes when data changes
  useEffect(() => {
    const width = 900;
    const height = 600;

    // Separate roles, cookbooks, and run_list_entries for initial positioning.
    // Place run_list entries on the left, roles in the middle, cookbooks on the right.
    const runListEntries = nodes.filter((n) => n.type === "run_list_entry");
    const roles = nodes.filter((n) => n.type === "role");
    const cookbooks = nodes.filter((n) => n.type === "cookbook");

    const simNodes: SimNode[] = [];

    runListEntries.forEach((n, i) => {
      const angle = (i / Math.max(runListEntries.length, 1)) * Math.PI * 2;
      const radius = Math.min(width, height) * 0.15;
      simNodes.push({
        id: n.id,
        name: n.name,
        type: n.type,
        compatibility_status: n.compatibility_status,
        x: width * 0.2 + Math.cos(angle) * radius + (Math.random() - 0.5) * 30,
        y: height * 0.5 + Math.sin(angle) * radius + (Math.random() - 0.5) * 30,
        vx: 0,
        vy: 0,
        fx: null,
        fy: null,
      });
    });

    roles.forEach((n, i) => {
      const angle = (i / Math.max(roles.length, 1)) * Math.PI * 2;
      const radius = Math.min(width, height) * 0.2;
      simNodes.push({
        id: n.id,
        name: n.name,
        type: n.type,
        compatibility_status: n.compatibility_status,
        x: width * 0.35 + Math.cos(angle) * radius + (Math.random() - 0.5) * 30,
        y: height * 0.5 + Math.sin(angle) * radius + (Math.random() - 0.5) * 30,
        vx: 0,
        vy: 0,
        fx: null,
        fy: null,
      });
    });

    cookbooks.forEach((n, i) => {
      const angle = (i / Math.max(cookbooks.length, 1)) * Math.PI * 2;
      const radius = Math.min(width, height) * 0.25;
      simNodes.push({
        id: n.id,
        name: n.name,
        type: n.type,
        compatibility_status: n.compatibility_status,
        x: width * 0.6 + Math.cos(angle) * radius + (Math.random() - 0.5) * 30,
        y: height * 0.5 + Math.sin(angle) * radius + (Math.random() - 0.5) * 30,
        vx: 0,
        vy: 0,
        fx: null,
        fy: null,
      });
    });

    simNodesRef.current = simNodes;
    tickCountRef.current = 0;

    // Reset transform on data change
    setTransform({ x: 0, y: 0, scale: 1 });
  }, [nodes]);

  // Run force simulation
  useEffect(() => {
    const width = 900;
    const height = 600;
    const centerX = width / 2;
    const centerY = height / 2;
    let alpha = 1.0;
    const alphaDecay = 0.005;
    const alphaMin = 0.001;

    const nodeMap = new Map<string, SimNode>();

    const tick = () => {
      const simNodes = simNodesRef.current;
      if (simNodes.length === 0) return;

      // Rebuild map each tick (nodes array is stable ref but values mutate)
      nodeMap.clear();
      for (const n of simNodes) {
        nodeMap.set(n.id, n);
      }

      // Only simulate while alpha is above threshold
      if (alpha > alphaMin) {
        // 1. Repulsion (charge) — all pairs
        const repulsionStrength = -120;
        for (let i = 0; i < simNodes.length; i++) {
          for (let j = i + 1; j < simNodes.length; j++) {
            const a = simNodes[i];
            const b = simNodes[j];
            const dx = b.x - a.x;
            const dy = b.y - a.y;
            let dist = Math.sqrt(dx * dx + dy * dy);
            if (dist < 1) dist = 1;
            const force = (repulsionStrength * alpha) / (dist * dist);
            const fx = (dx / dist) * force;
            const fy = (dy / dist) * force;
            if (a.fx === null) {
              a.vx -= fx;
              a.vy -= fy;
            }
            if (b.fx === null) {
              b.vx += fx;
              b.vy += fy;
            }
          }
        }

        // 2. Link attraction (spring)
        const linkStrength = 0.15;
        const idealLength = 100;
        for (const edge of edges) {
          const source = nodeMap.get(edge.source);
          const target = nodeMap.get(edge.target);
          if (!source || !target) continue;
          const dx = target.x - source.x;
          const dy = target.y - source.y;
          let dist = Math.sqrt(dx * dx + dy * dy);
          if (dist < 1) dist = 1;
          const displacement = dist - idealLength;
          const force = displacement * linkStrength * alpha;
          const fx = (dx / dist) * force;
          const fy = (dy / dist) * force;
          if (source.fx === null) {
            source.vx += fx;
            source.vy += fy;
          }
          if (target.fx === null) {
            target.vx -= fx;
            target.vy -= fy;
          }
        }

        // 3. Center gravity
        const gravityStrength = 0.02;
        for (const n of simNodes) {
          if (n.fx !== null) continue;
          n.vx += (centerX - n.x) * gravityStrength * alpha;
          n.vy += (centerY - n.y) * gravityStrength * alpha;
        }

        // 4. Velocity damping and position update
        const damping = 0.6;
        for (const n of simNodes) {
          if (n.fx !== null) {
            n.x = n.fx;
            n.y = n.fy!;
            n.vx = 0;
            n.vy = 0;
          } else {
            n.vx *= damping;
            n.vy *= damping;
            n.x += n.vx;
            n.y += n.vy;
            // Keep within bounds (loosely)
            n.x = Math.max(30, Math.min(width - 30, n.x));
            n.y = Math.max(30, Math.min(height - 30, n.y));
          }
        }

        alpha -= alphaDecay;
      }

      tickCountRef.current++;
      // Re-render every frame during active simulation, then every 10th frame
      if (alpha > alphaMin || tickCountRef.current % 10 === 0) {
        forceRender((c) => c + 1);
      }

      animRef.current = requestAnimationFrame(tick);
    };

    animRef.current = requestAnimationFrame(tick);

    return () => {
      cancelAnimationFrame(animRef.current);
    };
  }, [edges, nodes]);

  // Build adjacency sets for highlighting
  const adjacency = useMemo(() => {
    const map = new Map<string, Set<string>>();
    for (const e of edges) {
      if (!map.has(e.source)) map.set(e.source, new Set());
      if (!map.has(e.target)) map.set(e.target, new Set());
      map.get(e.source)!.add(e.target);
      map.get(e.target)!.add(e.source);
    }
    return map;
  }, [edges]);

  // Compute which nodes/edges to show based on filters
  const simNodes = simNodesRef.current;
  const nodeMap = new Map(simNodes.map((n) => [n.id, n]));

  const searchLower = searchTerm.toLowerCase();
  const isSearchActive = searchTerm.length > 0;

  // Determine connected set for selected node
  const connectedToSelected = useMemo(() => {
    if (!selectedNodeId) return null;
    const set = new Set<string>();
    set.add(selectedNodeId);
    const adj = adjacency.get(selectedNodeId);
    if (adj) {
      for (const id of adj) set.add(id);
    }
    return set;
  }, [selectedNodeId, adjacency]);

  // Determine connected set for hovered node
  const connectedToHovered = useMemo(() => {
    if (!hoveredNodeId || hoveredNodeId === selectedNodeId) return null;
    const set = new Set<string>();
    set.add(hoveredNodeId);
    const adj = adjacency.get(hoveredNodeId);
    if (adj) {
      for (const id of adj) set.add(id);
    }
    return set;
  }, [hoveredNodeId, selectedNodeId, adjacency]);

  const isNodeVisible = (n: SimNode): boolean => {
    if (filterType !== "all" && n.type !== filterType) return false;
    return true;
  };

  const getNodeOpacity = (n: SimNode): number => {
    if (!isNodeVisible(n)) return 0;

    // Search highlighting
    if (isSearchActive) {
      const matches = n.name.toLowerCase().includes(searchLower);
      if (!matches) return 0.15;
    }

    // Selection highlighting
    if (connectedToSelected) {
      if (!connectedToSelected.has(n.id)) return 0.12;
    }

    // Hover highlighting (only if not already selected)
    if (connectedToHovered && !connectedToSelected) {
      if (!connectedToHovered.has(n.id)) return 0.25;
    }

    return 1;
  };

  const getEdgeOpacity = (e: GraphEdge): number => {
    const sourceNode = nodeMap.get(e.source);
    const targetNode = nodeMap.get(e.target);
    if (!sourceNode || !targetNode) return 0;
    if (!isNodeVisible(sourceNode) || !isNodeVisible(targetNode)) return 0;

    if (isSearchActive) {
      const sourceMatches = sourceNode.name.toLowerCase().includes(searchLower);
      const targetMatches = targetNode.name.toLowerCase().includes(searchLower);
      if (!sourceMatches && !targetMatches) return 0.05;
      if (sourceMatches && targetMatches) return 0.8;
      return 0.3;
    }

    if (connectedToSelected) {
      if (
        (e.source === selectedNodeId || e.target === selectedNodeId) &&
        connectedToSelected.has(e.source) &&
        connectedToSelected.has(e.target)
      ) {
        return 1;
      }
      return 0.06;
    }

    if (connectedToHovered && !connectedToSelected) {
      if (
        (e.source === hoveredNodeId || e.target === hoveredNodeId) &&
        connectedToHovered.has(e.source) &&
        connectedToHovered.has(e.target)
      ) {
        return 0.8;
      }
      return 0.15;
    }

    return 0.4;
  };

  // Drag handlers
  const handleMouseDown = (e: React.MouseEvent, nodeId: string) => {
    e.preventDefault();
    e.stopPropagation();
    const node = nodeMap.get(nodeId);
    if (!node) return;

    const svgRect = svgRef.current?.getBoundingClientRect();
    if (!svgRect) return;

    const clientX = e.clientX;
    const clientY = e.clientY;
    // Convert screen coords to SVG coords using current transform
    const svgX = (clientX - svgRect.left - transform.x) / transform.scale;
    const svgY = (clientY - svgRect.top - transform.y) / transform.scale;

    dragRef.current = {
      nodeId,
      startX: clientX,
      startY: clientY,
      offsetX: node.x - svgX,
      offsetY: node.y - svgY,
    };

    node.fx = node.x;
    node.fy = node.y;

    const handleMouseMove = (ev: MouseEvent) => {
      const drag = dragRef.current;
      if (!drag.nodeId) return;
      const n = nodeMap.get(drag.nodeId);
      if (!n) return;

      const mx = (ev.clientX - svgRect.left - transform.x) / transform.scale;
      const my = (ev.clientY - svgRect.top - transform.y) / transform.scale;
      n.fx = mx + drag.offsetX;
      n.fy = my + drag.offsetY;
      n.x = n.fx;
      n.y = n.fy;
      forceRender((c) => c + 1);
    };

    const handleMouseUp = () => {
      const drag = dragRef.current;
      if (drag.nodeId) {
        const n = nodeMap.get(drag.nodeId);
        if (n) {
          // If barely moved, treat as a click
          const dist =
            Math.abs(e.clientX - drag.startX) +
            Math.abs(e.clientY - drag.startY);
          if (dist < 5) {
            onSelectNode(selectedNodeId === nodeId ? null : nodeId);
          }
          // Release fixed position
          n.fx = null;
          n.fy = null;
        }
      }
      dragRef.current.nodeId = null;
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
    };

    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
  };

  // Pan handlers on SVG background
  const handleSvgMouseDown = (e: React.MouseEvent) => {
    // Only pan on direct SVG background clicks (not on nodes)
    if (e.target !== svgRef.current) return;
    e.preventDefault();
    panRef.current = {
      active: true,
      startX: e.clientX,
      startY: e.clientY,
      origTx: transform.x,
      origTy: transform.y,
    };

    const handlePanMove = (ev: MouseEvent) => {
      if (!panRef.current.active) return;
      const dx = ev.clientX - panRef.current.startX;
      const dy = ev.clientY - panRef.current.startY;
      setTransform((t) => ({
        ...t,
        x: panRef.current.origTx + dx,
        y: panRef.current.origTy + dy,
      }));
    };

    const handlePanUp = () => {
      panRef.current.active = false;
      document.removeEventListener("mousemove", handlePanMove);
      document.removeEventListener("mouseup", handlePanUp);
    };

    document.addEventListener("mousemove", handlePanMove);
    document.addEventListener("mouseup", handlePanUp);
  };

  // Zoom via scroll
  const handleWheel = (e: React.WheelEvent) => {
    e.preventDefault();
    const svgRect = svgRef.current?.getBoundingClientRect();
    if (!svgRect) return;

    const mouseX = e.clientX - svgRect.left;
    const mouseY = e.clientY - svgRect.top;

    const zoomFactor = e.deltaY < 0 ? 1.1 : 0.9;
    const newScale = Math.max(0.2, Math.min(5, transform.scale * zoomFactor));

    // Zoom toward mouse position
    const newX = mouseX - (mouseX - transform.x) * (newScale / transform.scale);
    const newY = mouseY - (mouseY - transform.y) * (newScale / transform.scale);

    setTransform({ x: newX, y: newY, scale: newScale });
  };

  // Zoom controls
  const handleZoomIn = () => {
    setTransform((t) => ({
      ...t,
      scale: Math.min(5, t.scale * 1.3),
    }));
  };

  const handleZoomOut = () => {
    setTransform((t) => ({
      ...t,
      scale: Math.max(0.2, t.scale / 1.3),
    }));
  };

  const handleZoomReset = () => {
    setTransform({ x: 0, y: 0, scale: 1 });
  };

  // Node radius
  const nodeRadius = (n: SimNode) =>
    n.type === "role" ? 8 : n.type === "run_list_entry" ? 10 : 6;
  // Node fill colour — roles are blue, run_list_entry purple, cookbooks coloured by compatibility_status
  const getNodeFill = (n: SimNode): string => {
    if (n.type === "role") return "#3b82f6";
    if (n.type === "run_list_entry") return "#8b5cf6";
    switch (n.compatibility_status) {
      case "incompatible":
        return "#ef4444";
      case "compatible":
        return "#10b981";
      default:
        return "#9ca3af";
    }
  };


  return (
    <div className="card relative overflow-hidden p-0">
      {/* Zoom controls */}
      <div className="absolute right-3 top-3 z-10 flex flex-col gap-1 rounded-lg border border-gray-200 bg-white/90 p-1 shadow-sm backdrop-blur-sm">
        <button
          onClick={handleZoomIn}
          className="rounded p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
          title="Zoom in"
        >
          <svg
            className="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 4.5v15m7.5-7.5h-15"
            />
          </svg>
        </button>
        <button
          onClick={handleZoomOut}
          className="rounded p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
          title="Zoom out"
        >
          <svg
            className="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M19.5 12h-15"
            />
          </svg>
        </button>
        <div className="mx-1 border-t border-gray-200" />
        <button
          onClick={handleZoomReset}
          className="rounded p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
          title="Reset view"
        >
          <svg
            className="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M9 9V4.5M9 9H4.5M9 9 3.75 3.75M9 15v4.5M9 15H4.5M9 15l-5.25 5.25M15 9h4.5M15 9V4.5M15 9l5.25-5.25M15 15h4.5M15 15v4.5m0-4.5 5.25 5.25"
            />
          </svg>
        </button>
      </div>

      {/* Selected node info panel */}
      {selectedNodeId && (
        <SelectedNodePanel
          nodeId={selectedNodeId}
          simNodes={simNodes}
          edges={edges}
          adjacency={adjacency}
          onClose={() => onSelectNode(null)}
        />
      )}

      <svg
        ref={svgRef}
        viewBox="0 0 900 600"
        className="h-[600px] w-full cursor-grab active:cursor-grabbing"
        style={{ background: "#fafbfc" }}
        onMouseDown={handleSvgMouseDown}
        onWheel={handleWheel}
      >
        <defs>
          <marker
            id="arrowhead"
            viewBox="0 0 10 7"
            refX="10"
            refY="3.5"
            markerWidth="8"
            markerHeight="6"
            orient="auto-start-reverse"
          >
            <polygon points="0 0, 10 3.5, 0 7" fill="#94a3b8" />
          </marker>
          <marker
            id="arrowhead-highlight"
            viewBox="0 0 10 7"
            refX="10"
            refY="3.5"
            markerWidth="8"
            markerHeight="6"
            orient="auto-start-reverse"
          >
            <polygon points="0 0, 10 3.5, 0 7" fill="#3b82f6" />
          </marker>
        </defs>

        <g
          transform={`translate(${transform.x}, ${transform.y}) scale(${transform.scale})`}
        >
          {/* Edges */}
          {edges.map((e, i) => {
            const source = nodeMap.get(e.source);
            const target = nodeMap.get(e.target);
            if (!source || !target) return null;
            const opacity = getEdgeOpacity(e);
            if (opacity === 0) return null;

            // Shorten line to not overlap with node circles
            const dx = target.x - source.x;
            const dy = target.y - source.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist < 1) return null;
            const sourceR = nodeRadius(source) + 2;
            const targetR = nodeRadius(target) + 10; // account for arrowhead
            const sx = source.x + (dx / dist) * sourceR;
            const sy = source.y + (dy / dist) * sourceR;
            const tx = target.x - (dx / dist) * targetR;
            const ty = target.y - (dy / dist) * targetR;

            const isHighlighted =
              opacity > 0.6 && (connectedToSelected || connectedToHovered);

            return (
              <line
                key={`edge-${i}`}
                x1={sx}
                y1={sy}
                x2={tx}
                y2={ty}
                stroke={isHighlighted ? "#3b82f6" : "#94a3b8"}
                strokeWidth={isHighlighted ? 2 : 1}
                opacity={opacity}
                markerEnd={
                  isHighlighted
                    ? "url(#arrowhead-highlight)"
                    : "url(#arrowhead)"
                }
                style={{ transition: "opacity 0.2s" }}
              />
            );
          })}

          {/* Nodes */}
          {simNodes.map((n) => {
            const opacity = getNodeOpacity(n);
            if (opacity === 0) return null;

            const r = nodeRadius(n);
            const isRole = n.type === "role";
            const isSelected = n.id === selectedNodeId;
            const isHovered = n.id === hoveredNodeId;
            const fill = getNodeFill(n);
            const highlightRing = isSelected
              ? "#1d4ed8"
              : isHovered
                ? "#60a5fa"
                : "none";

            return (
              <g
                key={n.id}
                transform={`translate(${n.x}, ${n.y})`}
                style={{ cursor: "pointer", transition: "opacity 0.2s" }}
                opacity={opacity}
                onMouseDown={(e) => handleMouseDown(e, n.id)}
                onMouseEnter={() => onHoverNode(n.id)}
                onMouseLeave={() => onHoverNode(null)}
              >
                {/* Selection/hover ring */}
                {(isSelected || isHovered) && (
                  <circle
                    r={r + 4}
                    fill="none"
                    stroke={highlightRing}
                    strokeWidth={2}
                    opacity={0.5}
                  />
                )}

                {/* Node shape: diamond for run_list_entry, square for roles, circle for cookbooks */}
                {n.type === "run_list_entry" ? (
                  <polygon
                    points={`0,${-r} ${r},0 0,${r} ${-r},0`}
                    fill={fill}
                    stroke={isSelected ? "#6d28d9" : "white"}
                    strokeWidth={isSelected ? 2.5 : 1.5}
                  />
                ) : isRole ? (
                  <rect
                    x={-r}
                    y={-r}
                    width={r * 2}
                    height={r * 2}
                    rx={2}
                    fill={fill}
                    stroke={isSelected ? "#1d4ed8" : "white"}
                    strokeWidth={isSelected ? 2.5 : 1.5}
                  />
                ) : (
                  <circle
                    r={r}
                    fill={fill}
                    stroke={isSelected ? "#065f46" : "white"}
                    strokeWidth={isSelected ? 2.5 : 1.5}
                  />
                )}

                {/* Label */}
                <text
                  y={r + 12}
                  textAnchor="middle"
                  className="select-none"
                  style={{
                    fontSize: "9px",
                    fill: "#374151",
                    fontWeight: isSelected ? 600 : 400,
                    pointerEvents: "none",
                  }}
                >
                  {n.name.length > 18 ? n.name.slice(0, 16) + "…" : n.name}
                </text>
              </g>
            );
          })}
        </g>
      </svg>
    </div>
  );
}

export function SelectedNodePanel({
  nodeId,
  simNodes,
  edges,
  adjacency,
  onClose,
}: {
  nodeId: string;
  simNodes: SimNode[];
  edges: GraphEdge[];
  adjacency: Map<string, Set<string>>;
  onClose: () => void;
}) {
  const node = simNodes.find((n) => n.id === nodeId);
  if (!node) return null;

  const isRole = node.type === "role";

  // Find direct connections
  const outgoing = edges.filter((e) => e.source === nodeId);
  const incoming = edges.filter((e) => e.target === nodeId);

  const connectedNodeIds = adjacency.get(nodeId) ?? new Set();
  const connectedNodes = simNodes.filter((n) => connectedNodeIds.has(n.id));

  const depCookbooks = outgoing
    .filter((e) => e.type === "cookbook")
    .map((e) => simNodes.find((n) => n.id === e.target))
    .filter(Boolean) as SimNode[];

  const depRoles = outgoing
    .filter((e) => e.type === "role")
    .map((e) => simNodes.find((n) => n.id === e.target))
    .filter(Boolean) as SimNode[];

  const dependedOnBy = incoming
    .map((e) => simNodes.find((n) => n.id === e.source))
    .filter(Boolean) as SimNode[];

  return (
    <div className="absolute left-3 top-3 z-10 w-72 rounded-lg border border-gray-200 bg-white/95 p-4 shadow-lg backdrop-blur-sm">
      <div className="mb-3 flex items-start justify-between">
        <div className="flex items-center gap-2">
          {isRole ? (
            <span className="inline-block h-3 w-3 rounded-sm bg-blue-500" />
          ) : (
            <span className="inline-block h-3 w-3 rounded-full bg-emerald-500" />
          )}
          <div>
            <h4 className="text-sm font-semibold text-gray-800">{node.name}</h4>
            <span className="text-[10px] uppercase tracking-wide text-gray-400">
              {node.type}
            </span>
          </div>
        </div>
        <button
          onClick={onClose}
          className="rounded p-0.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600"
        >
          <svg
            className="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M6 18 18 6M6 6l12 12"
            />
          </svg>
        </button>
      </div>

      <div className="space-y-2 text-xs">
        {/* Stats */}
        <div className="grid grid-cols-3 gap-2">
          <div className="rounded-md bg-gray-50 p-2 text-center">
            <div className="text-lg font-bold text-gray-700">
              {connectedNodes.length}
            </div>
            <div className="text-[10px] text-gray-500">Connected</div>
          </div>
          <div className="rounded-md bg-gray-50 p-2 text-center">
            <div className="text-lg font-bold text-blue-600">
              {outgoing.length}
            </div>
            <div className="text-[10px] text-gray-500">Depends on</div>
          </div>
          <div className="rounded-md bg-gray-50 p-2 text-center">
            <div className="text-lg font-bold text-amber-600">
              {incoming.length}
            </div>
            <div className="text-[10px] text-gray-500">Used by</div>
          </div>
        </div>

        {/* Cookbook dependencies */}
        {depCookbooks.length > 0 && (
          <div>
            <h5 className="mb-1 font-medium text-gray-600">
              Cookbook Dependencies
            </h5>
            <div className="flex flex-wrap gap-1">
              {depCookbooks.map((n) => (
                <Link
                  key={n.id}
                  to={`/cookbooks/${encodeURIComponent(n.name)}`}
                  className="inline-flex items-center gap-1 rounded-full bg-emerald-50 px-2 py-0.5 text-[10px] font-medium text-emerald-700 transition-colors hover:bg-emerald-100"
                >
                  <span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-500" />
                  {n.name}
                </Link>
              ))}
            </div>
          </div>
        )}

        {/* Role dependencies */}
        {depRoles.length > 0 && (
          <div>
            <h5 className="mb-1 font-medium text-gray-600">
              Role Dependencies
            </h5>
            <div className="flex flex-wrap gap-1">
              {depRoles.map((n) => (
                <Link
                  key={n.id}
                  to={`/roles/${encodeURIComponent(n.name)}`}
                  className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2 py-0.5 text-[10px] font-medium text-blue-700 transition-colors hover:bg-blue-100"
                >
                  <span className="inline-block h-1.5 w-1.5 rounded-sm bg-blue-500" />
                  {n.name}
                </Link>
              ))}
            </div>
          </div>
        )}

        {/* Depended on by */}
        {dependedOnBy.length > 0 && (
          <div>
            <h5 className="mb-1 font-medium text-gray-600">Depended on by</h5>
            <div className="flex flex-wrap gap-1">
              {dependedOnBy.map((n) => (
                <Link
                  key={n.id}
                  to={
                    n.type === "role"
                      ? `/roles/${encodeURIComponent(n.name)}`
                      : `/cookbooks/${encodeURIComponent(n.name)}`
                  }
                  className="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-medium text-amber-700 transition-colors hover:bg-amber-100"
                >
                  <span
                    className={`inline-block h-1.5 w-1.5 ${n.type === "role" ? "rounded-sm bg-blue-500" : "rounded-full bg-emerald-500"}`}
                  />
                  {n.name}
                </Link>
              ))}
            </div>
          </div>
        )}

        {/* Link to detail page */}
        {(node.type === "cookbook" || node.type === "role") && (
          <Link
            to={
              node.type === "role"
                ? `/roles/${encodeURIComponent(node.name)}`
                : `/cookbooks/${encodeURIComponent(node.name)}`
            }
            className="mt-2 flex items-center justify-center gap-1 rounded-md bg-blue-50 px-3 py-1.5 text-xs font-medium text-blue-700 transition-colors hover:bg-blue-100"
          >
            {node.type === "role"
              ? "View Role Details"
              : "View Cookbook Details"}
            <svg
              className="h-3.5 w-3.5"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={2}
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M13.5 4.5 21 12m0 0-7.5 7.5M21 12H3"
              />
            </svg>
          </Link>
        )}
      </div>
    </div>
  );
}
