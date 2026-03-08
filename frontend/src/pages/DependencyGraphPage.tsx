import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { Link } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import {
  fetchDependencyGraph,
  fetchDependencyGraphTable,
  type DependencyGraphTableQuery,
} from "../api";
import type {
  DependencyGraphResponse,
  DependencyGraphNode,
  DependencyGraphEdge,
  DependencyGraphTableResponse,
  DependencyTableRow,
  SharedCookbook,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";

// ---------------------------------------------------------------------------
// Dependency Graph page
//
// Two views switchable via tabs:
//   1. Graph View — interactive force-directed graph (SVG, pure TS simulation)
//   2. Table View — flat list of roles with dependency counts + detail expand
//
// The graph view colours nodes by type (role vs cookbook) and highlights
// incompatible/connected subgraphs on click. Supports filtering by name,
// type, and search. The table view supports sorting and pagination.
// ---------------------------------------------------------------------------

type ViewMode = "graph" | "table";

export function DependencyGraphPage() {
  const { selectedOrg, organisations } = useOrg();
  const org = selectedOrg || "";

  const [viewMode, setViewMode] = useState<ViewMode>("graph");

  // If no org is selected and we have orgs available, show a prompt.
  const hasOrg = org !== "";
  const hasOrgs = organisations.length > 0;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-xl font-bold text-gray-800">Dependency Graph</h2>
          <p className="mt-1 text-sm text-gray-500">
            Role → cookbook dependency relationships
            {org && <span className="font-medium text-gray-700"> — {org}</span>}
          </p>
        </div>

        {/* View mode toggle */}
        <div className="flex rounded-lg border border-gray-200 bg-white p-0.5 shadow-sm">
          <button
            className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${viewMode === "graph"
              ? "bg-blue-50 text-blue-700"
              : "text-gray-600 hover:text-gray-900"
              }`}
            onClick={() => setViewMode("graph")}
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M7.217 10.907a2.25 2.25 0 1 0 0 2.186m0-2.186c.18.324.283.696.283 1.093s-.103.77-.283 1.093m0-2.186 9.566-5.314m-9.566 7.5 9.566 5.314m0 0a2.25 2.25 0 1 0 3.935 2.186 2.25 2.25 0 0 0-3.935-2.186Zm0-12.814a2.25 2.25 0 1 0 3.933-2.185 2.25 2.25 0 0 0-3.933 2.185Z" />
            </svg>
            Graph
          </button>
          <button
            className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${viewMode === "table"
              ? "bg-blue-50 text-blue-700"
              : "text-gray-600 hover:text-gray-900"
              }`}
            onClick={() => setViewMode("table")}
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 0 1-1.125-1.125M3.375 19.5h7.5c.621 0 1.125-.504 1.125-1.125m-9.75 0V5.625m0 12.75v-1.5c0-.621.504-1.125 1.125-1.125m18.375 2.625V5.625m0 12.75c0 .621-.504 1.125-1.125 1.125m1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125m0 3.75h-7.5A1.125 1.125 0 0 1 12 18.375m9.75-12.75c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125m19.5 0v1.5c0 .621-.504 1.125-1.125 1.125M2.25 5.625v1.5c0 .621.504 1.125 1.125 1.125m0 0h17.25m-17.25 0h7.5c.621 0 1.125.504 1.125 1.125M3.375 8.25c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125m17.25-3.75h-7.5c-.621 0-1.125.504-1.125 1.125m8.625-1.125c.621 0 1.125.504 1.125 1.125v1.5c0 .621-.504 1.125-1.125 1.125m-17.25 0h7.5m-7.5 0c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125M12 10.875v-1.5m0 1.5c0 .621-.504 1.125-1.125 1.125M12 10.875c0 .621.504 1.125 1.125 1.125m-2.25 0c.621 0 1.125.504 1.125 1.125M10.875 12h-1.5m1.5 0c.621 0 1.125.504 1.125 1.125M12 12h7.5m-7.5 0c0 .621-.504 1.125-1.125 1.125M21.375 12c.621 0 1.125.504 1.125 1.125v1.5c0 .621-.504 1.125-1.125 1.125m-17.25 0h7.5m-7.5 0c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125m17.25-3.75h-7.5c-.621 0-1.125.504-1.125 1.125m8.625-1.125c.621 0 1.125.504 1.125 1.125v1.5c0 .621-.504 1.125-1.125 1.125m-2.25 0h.008v.008h-.008v-.008Zm0-3.75h.008v.008h-.008V12Zm0-3.75h.008v.008h-.008V8.25Z" />
            </svg>
            Table
          </button>
        </div>
      </div>

      {/* Content */}
      {!hasOrg && hasOrgs ? (
        <div className="card">
          <EmptyState
            title="Select an organisation"
            description="Choose an organisation from the dropdown above to view its dependency graph."
          />
        </div>
      ) : !hasOrgs ? (
        <div className="card">
          <EmptyState
            title="No organisations available"
            description="No organisations have been configured or collected yet."
          />
        </div>
      ) : viewMode === "graph" ? (
        <GraphView organisation={org} />
      ) : (
        <TableView organisation={org} />
      )}
    </div>
  );
}

// ===========================================================================
// Graph View
// ===========================================================================

// ---------------------------------------------------------------------------
// Force simulation types — lightweight D3-like force-directed layout
// ---------------------------------------------------------------------------

interface SimNode {
  id: string;
  name: string;
  type: "role" | "cookbook";
  x: number;
  y: number;
  vx: number;
  vy: number;
  fx: number | null; // fixed x (when dragging)
  fy: number | null; // fixed y (when dragging)
}

interface SimEdge {
  source: string;
  target: string;
  dependency_type: string;
}

function GraphView({ organisation }: { organisation: string }) {
  const [data, setData] = useState<DependencyGraphResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Interaction state
  const [searchTerm, setSearchTerm] = useState("");
  const [filterType, setFilterType] = useState<"all" | "role" | "cookbook">("all");
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchDependencyGraph(organisation)
      .then((res) => {
        setData(res);
        setSelectedNodeId(null);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation]);

  useEffect(() => {
    load();
  }, [load]);

  if (loading) {
    return (
      <div className="card">
        <LoadingSpinner message="Loading dependency graph…" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="card">
        <ErrorAlert message={error} onRetry={load} />
      </div>
    );
  }

  if (!data || data.nodes.length === 0) {
    return (
      <div className="card">
        <EmptyState
          title="No dependency data"
          description="No role/cookbook dependencies have been collected for this organisation yet."
        />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Summary stats */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <div className="stat-card">
          <span className="stat-label">Total Nodes</span>
          <span className="stat-value text-gray-800">{data.summary.total_nodes}</span>
        </div>
        <div className="stat-card">
          <span className="stat-label">Total Edges</span>
          <span className="stat-value text-gray-800">{data.summary.total_edges}</span>
        </div>
        <div className="stat-card">
          <span className="stat-label">Roles</span>
          <span className="stat-value text-blue-600">{data.summary.role_count}</span>
        </div>
        <div className="stat-card">
          <span className="stat-label">Cookbooks</span>
          <span className="stat-value text-emerald-600">{data.summary.cookbook_count}</span>
        </div>
      </div>

      {/* Filters */}
      <div className="card">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          {/* Search */}
          <div className="relative flex-1">
            <svg className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z" />
            </svg>
            <input
              type="text"
              placeholder="Search nodes…"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full rounded-md border border-gray-300 py-1.5 pl-9 pr-3 text-sm text-gray-700 placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </div>

          {/* Type filter */}
          <div className="flex items-center gap-2">
            <span className="text-xs font-medium text-gray-500">Show:</span>
            {(["all", "role", "cookbook"] as const).map((type) => (
              <button
                key={type}
                onClick={() => setFilterType(type)}
                className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${filterType === type
                  ? type === "role"
                    ? "bg-blue-100 text-blue-800"
                    : type === "cookbook"
                      ? "bg-emerald-100 text-emerald-800"
                      : "bg-gray-200 text-gray-800"
                  : "bg-gray-100 text-gray-500 hover:bg-gray-200"
                  }`}
              >
                {type === "all" ? "All" : type === "role" ? "Roles" : "Cookbooks"}
              </button>
            ))}
          </div>

          {/* Clear selection */}
          {selectedNodeId && (
            <button
              onClick={() => setSelectedNodeId(null)}
              className="flex items-center gap-1 rounded-md bg-gray-100 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200"
            >
              <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
              </svg>
              Clear selection
            </button>
          )}
        </div>

        {/* Legend */}
        <div className="mt-3 flex flex-wrap items-center gap-4 border-t border-gray-100 pt-3 text-xs text-gray-500">
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-3 rounded-sm bg-blue-500" />
            Role
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-3 rounded-full bg-emerald-500" />
            Cookbook
          </span>
          <span className="flex items-center gap-1.5">
            <svg className="h-3 w-8" viewBox="0 0 32 12">
              <line x1="0" y1="6" x2="32" y2="6" stroke="#94a3b8" strokeWidth="1.5" />
              <polygon points="28,3 32,6 28,9" fill="#94a3b8" />
            </svg>
            Depends on
          </span>
          <span className="ml-auto text-[10px] text-gray-400">Click a node to highlight its connections • Drag nodes to reposition</span>
        </div>
      </div>

      {/* Graph canvas */}
      <ForceGraph
        nodes={data.nodes}
        edges={data.edges}
        searchTerm={searchTerm}
        filterType={filterType}
        selectedNodeId={selectedNodeId}
        hoveredNodeId={hoveredNodeId}
        onSelectNode={setSelectedNodeId}
        onHoverNode={setHoveredNodeId}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Force-directed graph component (SVG + requestAnimationFrame simulation)
// ---------------------------------------------------------------------------

interface ForceGraphProps {
  nodes: DependencyGraphNode[];
  edges: DependencyGraphEdge[];
  searchTerm: string;
  filterType: "all" | "role" | "cookbook";
  selectedNodeId: string | null;
  hoveredNodeId: string | null;
  onSelectNode: (id: string | null) => void;
  onHoverNode: (id: string | null) => void;
}

function ForceGraph({
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
  const panRef = useRef<{ active: boolean; startX: number; startY: number; origTx: number; origTy: number }>({
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

    // Separate roles and cookbooks for initial positioning.
    // Place roles on the left side and cookbooks on the right.
    const roles = nodes.filter((n) => n.type === "role");
    const cookbooks = nodes.filter((n) => n.type === "cookbook");

    const simNodes: SimNode[] = [];

    roles.forEach((n, i) => {
      const angle = (i / Math.max(roles.length, 1)) * Math.PI * 2;
      const radius = Math.min(width, height) * 0.2;
      simNodes.push({
        id: n.id,
        name: n.name,
        type: n.type,
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
            let dx = b.x - a.x;
            let dy = b.y - a.y;
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
          let dx = target.x - source.x;
          let dy = target.y - source.y;
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

  const getEdgeOpacity = (e: SimEdge): number => {
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
          const dist = Math.abs(e.clientX - drag.startX) + Math.abs(e.clientY - drag.startY);
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
      setTransform((t) => ({ ...t, x: panRef.current.origTx + dx, y: panRef.current.origTy + dy }));
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
  const nodeRadius = (n: SimNode) => (n.type === "role" ? 8 : 6);

  return (
    <div className="card relative overflow-hidden p-0">
      {/* Zoom controls */}
      <div className="absolute right-3 top-3 z-10 flex flex-col gap-1 rounded-lg border border-gray-200 bg-white/90 p-1 shadow-sm backdrop-blur-sm">
        <button
          onClick={handleZoomIn}
          className="rounded p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
          title="Zoom in"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
        </button>
        <button
          onClick={handleZoomOut}
          className="rounded p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
          title="Zoom out"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 12h-15" />
          </svg>
        </button>
        <div className="mx-1 border-t border-gray-200" />
        <button
          onClick={handleZoomReset}
          className="rounded p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
          title="Reset view"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 9V4.5M9 9H4.5M9 9 3.75 3.75M9 15v4.5M9 15H4.5M9 15l-5.25 5.25M15 9h4.5M15 9V4.5M15 9l5.25-5.25M15 15h4.5M15 15v4.5m0-4.5 5.25 5.25" />
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

        <g transform={`translate(${transform.x}, ${transform.y}) scale(${transform.scale})`}>
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
              opacity > 0.6 &&
              (connectedToSelected || connectedToHovered);

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
                markerEnd={isHighlighted ? "url(#arrowhead-highlight)" : "url(#arrowhead)"}
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
            const fill = isRole ? "#3b82f6" : "#10b981";
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

                {/* Node shape: square for roles, circle for cookbooks */}
                {isRole ? (
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

// ---------------------------------------------------------------------------
// Selected node info panel (overlay inside the graph card)
// ---------------------------------------------------------------------------

function SelectedNodePanel({
  nodeId,
  simNodes,
  edges,
  adjacency,
  onClose,
}: {
  nodeId: string;
  simNodes: SimNode[];
  edges: DependencyGraphEdge[];
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
    .filter((e) => e.dependency_type === "cookbook")
    .map((e) => simNodes.find((n) => n.id === e.target))
    .filter(Boolean) as SimNode[];

  const depRoles = outgoing
    .filter((e) => e.dependency_type === "role")
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
            <span className="text-[10px] uppercase tracking-wide text-gray-400">{node.type}</span>
          </div>
        </div>
        <button
          onClick={onClose}
          className="rounded p-0.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div className="space-y-2 text-xs">
        {/* Stats */}
        <div className="grid grid-cols-3 gap-2">
          <div className="rounded-md bg-gray-50 p-2 text-center">
            <div className="text-lg font-bold text-gray-700">{connectedNodes.length}</div>
            <div className="text-[10px] text-gray-500">Connected</div>
          </div>
          <div className="rounded-md bg-gray-50 p-2 text-center">
            <div className="text-lg font-bold text-blue-600">{outgoing.length}</div>
            <div className="text-[10px] text-gray-500">Depends on</div>
          </div>
          <div className="rounded-md bg-gray-50 p-2 text-center">
            <div className="text-lg font-bold text-amber-600">{incoming.length}</div>
            <div className="text-[10px] text-gray-500">Used by</div>
          </div>
        </div>

        {/* Cookbook dependencies */}
        {depCookbooks.length > 0 && (
          <div>
            <h5 className="mb-1 font-medium text-gray-600">Cookbook Dependencies</h5>
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
            <h5 className="mb-1 font-medium text-gray-600">Role Dependencies</h5>
            <div className="flex flex-wrap gap-1">
              {depRoles.map((n) => (
                <span
                  key={n.id}
                  className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2 py-0.5 text-[10px] font-medium text-blue-700"
                >
                  <span className="inline-block h-1.5 w-1.5 rounded-sm bg-blue-500" />
                  {n.name}
                </span>
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
                <span
                  key={n.id}
                  className="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-medium text-amber-700"
                >
                  <span
                    className={`inline-block h-1.5 w-1.5 ${n.type === "role" ? "rounded-sm bg-blue-500" : "rounded-full bg-emerald-500"}`}
                  />
                  {n.name}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Link to detail page */}
        {node.type === "cookbook" && (
          <Link
            to={`/cookbooks/${encodeURIComponent(node.name)}`}
            className="mt-2 flex items-center justify-center gap-1 rounded-md bg-blue-50 px-3 py-1.5 text-xs font-medium text-blue-700 transition-colors hover:bg-blue-100"
          >
            View Cookbook Details
            <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5 21 12m0 0-7.5 7.5M21 12H3" />
            </svg>
          </Link>
        )}
      </div>
    </div>
  );
}

// ===========================================================================
// Table View
// ===========================================================================

function TableView({ organisation }: { organisation: string }) {
  const [data, setData] = useState<DependencyGraphTableResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Sort & pagination
  const [sortField, setSortField] = useState<string>("total_dependencies");
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">("desc");
  const [page, setPage] = useState(1);
  const perPage = 25;

  // Expanded row
  const [expandedRole, setExpandedRole] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    const filters: DependencyGraphTableQuery = {
      organisation,
      sort: sortField,
      order: sortOrder,
      page,
      per_page: perPage,
    };
    fetchDependencyGraphTable(filters)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [organisation, sortField, sortOrder, page]);

  useEffect(() => {
    load();
  }, [load]);

  const handleSort = (field: string) => {
    if (field === sortField) {
      setSortOrder((o) => (o === "asc" ? "desc" : "asc"));
    } else {
      setSortField(field);
      setSortOrder(field === "role_name" ? "asc" : "desc");
    }
    setPage(1);
  };

  const sortIndicator = (field: string) => {
    if (field !== sortField) return null;
    return sortOrder === "asc" ? " ↑" : " ↓";
  };

  if (loading && !data) {
    return (
      <div className="card">
        <LoadingSpinner message="Loading dependency table…" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="card">
        <ErrorAlert message={error} onRetry={load} />
      </div>
    );
  }

  if (!data || data.data.length === 0) {
    return (
      <div className="card">
        <EmptyState
          title="No dependency data"
          description="No role/cookbook dependencies have been collected for this organisation yet."
        />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Summary cards */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
        <div className="stat-card">
          <span className="stat-label">Total Roles</span>
          <span className="stat-value text-blue-600">{data.total_roles}</span>
        </div>
        <div className="stat-card">
          <span className="stat-label">Shared Cookbooks</span>
          <span className="stat-value text-emerald-600">
            {data.shared_cookbooks?.length ?? 0}
          </span>
          <span className="stat-sub">Used by 2+ roles</span>
        </div>
        {data.shared_cookbooks && data.shared_cookbooks.length > 0 && (
          <div className="stat-card sm:col-span-1 col-span-2">
            <span className="stat-label">Most Shared</span>
            <span className="stat-value text-gray-800 text-base">
              {data.shared_cookbooks[0].cookbook_name}
            </span>
            <span className="stat-sub">
              Used by {data.shared_cookbooks[0].role_count} roles
            </span>
          </div>
        )}
      </div>

      {/* Shared cookbooks bar */}
      {data.shared_cookbooks && data.shared_cookbooks.length > 0 && (
        <SharedCookbooksCard cookbooks={data.shared_cookbooks} />
      )}

      {/* Table */}
      <div className="table-container">
        <table className="table">
          <thead>
            <tr>
              <th className="w-8" />
              <SortableHeader
                label="Role Name"
                field="role_name"
                currentField={sortField}
                currentOrder={sortOrder}
                onSort={handleSort}
                indicator={sortIndicator}
              />
              <SortableHeader
                label="Cookbooks"
                field="cookbook_count"
                currentField={sortField}
                currentOrder={sortOrder}
                onSort={handleSort}
                indicator={sortIndicator}
              />
              <SortableHeader
                label="Roles"
                field="role_count"
                currentField={sortField}
                currentOrder={sortOrder}
                onSort={handleSort}
                indicator={sortIndicator}
              />
              <SortableHeader
                label="Total Deps"
                field="total_dependencies"
                currentField={sortField}
                currentOrder={sortOrder}
                onSort={handleSort}
                indicator={sortIndicator}
              />
              <th>Depended on by</th>
              <th>Dependencies</th>
            </tr>
          </thead>
          <tbody>
            {data.data.map((row) => (
              <TableRow
                key={row.role_name}
                row={row}
                isExpanded={expandedRole === row.role_name}
                onToggle={() =>
                  setExpandedRole(
                    expandedRole === row.role_name ? null : row.role_name,
                  )
                }
              />
            ))}
          </tbody>
        </table>

        {/* Pagination */}
        <div className="border-t border-gray-200 px-4">
          <Pagination
            pagination={data.pagination}
            onPageChange={(p) => setPage(p)}
          />
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Shared Cookbooks mini-bar chart
// ---------------------------------------------------------------------------

function SharedCookbooksCard({ cookbooks }: { cookbooks: SharedCookbook[] }) {
  const maxCount = Math.max(...cookbooks.map((c) => c.role_count), 1);
  // Show top 10 max
  const shown = cookbooks.slice(0, 10);

  return (
    <div className="card">
      <h3 className="card-header text-sm">
        Most Shared Cookbooks
        <span className="ml-2 text-xs font-normal text-gray-400">
          (used by multiple roles)
        </span>
      </h3>
      <div className="space-y-1">
        {shown.map((cb) => {
          const pct = (cb.role_count / maxCount) * 100;
          return (
            <div key={cb.cookbook_name} className="bar-chart-row">
              <Link
                to={`/cookbooks/${encodeURIComponent(cb.cookbook_name)}`}
                className="bar-chart-label text-blue-600 hover:text-blue-800 hover:underline"
                title={cb.cookbook_name}
              >
                {cb.cookbook_name}
              </Link>
              <div className="bar-chart-track">
                <div
                  className="bar-chart-fill bg-emerald-500"
                  style={{ width: `${Math.max(pct, 4)}%` }}
                >
                  {pct >= 20 && <span>{cb.role_count} roles</span>}
                </div>
              </div>
              <span className="bar-chart-value">{cb.role_count}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sortable Table Header
// ---------------------------------------------------------------------------

function SortableHeader({
  label,
  field,
  currentField,
  currentOrder: _currentOrder,
  onSort,
  indicator,
}: {
  label: string;
  field: string;
  currentField: string;
  currentOrder: "asc" | "desc";
  onSort: (field: string) => void;
  indicator: (field: string) => string | null;
}) {
  const isActive = field === currentField;
  return (
    <th
      className="cursor-pointer select-none hover:text-gray-700"
      onClick={() => onSort(field)}
    >
      <span className={isActive ? "text-blue-600" : ""}>
        {label}
        {indicator(field)}
      </span>
    </th>
  );
}

// ---------------------------------------------------------------------------
// Table Row (expandable)
// ---------------------------------------------------------------------------

function TableRow({
  row,
  isExpanded,
  onToggle,
}: {
  row: DependencyTableRow;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const cookbookDeps = row.dependencies.filter((d) => d.type === "cookbook");
  const roleDeps = row.dependencies.filter((d) => d.type === "role");

  return (
    <>
      <tr
        className={`cursor-pointer ${isExpanded ? "bg-blue-50/50" : ""}`}
        onClick={onToggle}
      >
        <td className="w-8 text-center">
          <svg
            className={`inline-block h-4 w-4 text-gray-400 transition-transform duration-200 ${isExpanded ? "rotate-90" : ""
              }`}
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="m8.25 4.5 7.5 7.5-7.5 7.5" />
          </svg>
        </td>
        <td className="font-medium text-gray-900">{row.role_name}</td>
        <td>
          <span className="inline-flex items-center gap-1">
            <span className="inline-block h-2 w-2 rounded-full bg-emerald-500" />
            {row.cookbook_count}
          </span>
        </td>
        <td>
          <span className="inline-flex items-center gap-1">
            <span className="inline-block h-2 w-2 rounded-sm bg-blue-500" />
            {row.role_count}
          </span>
        </td>
        <td className="font-medium">{row.total_dependencies}</td>
        <td>
          {row.depended_on_by > 0 ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 ring-1 ring-inset ring-amber-600/20">
              {row.depended_on_by} {row.depended_on_by === 1 ? "role" : "roles"}
            </span>
          ) : (
            <span className="text-gray-400">—</span>
          )}
        </td>
        <td>
          {/* Mini dependency pills, show first few */}
          <div className="flex flex-wrap gap-1">
            {row.dependencies.slice(0, 4).map((d) => (
              <span
                key={`${d.type}:${d.name}`}
                className={`inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[10px] font-medium ${d.type === "cookbook"
                  ? "bg-emerald-50 text-emerald-700"
                  : "bg-blue-50 text-blue-700"
                  }`}
              >
                <span
                  className={`inline-block h-1 w-1 ${d.type === "cookbook" ? "rounded-full bg-emerald-500" : "rounded-sm bg-blue-500"}`}
                />
                {d.name}
              </span>
            ))}
            {row.dependencies.length > 4 && (
              <span className="inline-flex items-center rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500">
                +{row.dependencies.length - 4} more
              </span>
            )}
          </div>
        </td>
      </tr>

      {/* Expanded detail row */}
      {isExpanded && (
        <tr>
          <td colSpan={7} className="bg-gray-50/50 px-8 py-4">
            <div className="grid gap-4 sm:grid-cols-2">
              {/* Cookbook dependencies */}
              <div>
                <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-500">
                  Cookbook Dependencies ({cookbookDeps.length})
                </h4>
                {cookbookDeps.length === 0 ? (
                  <p className="text-xs text-gray-400">None</p>
                ) : (
                  <div className="flex flex-wrap gap-1.5">
                    {cookbookDeps.map((d) => (
                      <Link
                        key={d.name}
                        to={`/cookbooks/${encodeURIComponent(d.name)}`}
                        className="inline-flex items-center gap-1 rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700 ring-1 ring-inset ring-emerald-600/20 transition-colors hover:bg-emerald-100"
                      >
                        <span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-500" />
                        {d.name}
                      </Link>
                    ))}
                  </div>
                )}
              </div>

              {/* Role dependencies */}
              <div>
                <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-500">
                  Role Dependencies ({roleDeps.length})
                </h4>
                {roleDeps.length === 0 ? (
                  <p className="text-xs text-gray-400">None</p>
                ) : (
                  <div className="flex flex-wrap gap-1.5">
                    {roleDeps.map((d) => (
                      <span
                        key={d.name}
                        className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2.5 py-1 text-xs font-medium text-blue-700 ring-1 ring-inset ring-blue-600/20"
                      >
                        <span className="inline-block h-1.5 w-1.5 rounded-sm bg-blue-500" />
                        {d.name}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </td>
        </tr>
      )}
    </>
  );
}
