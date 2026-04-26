import { useState, useEffect, useCallback } from "react";
import { SMALL_PAGE_SIZE } from "../../constants";
import { Link } from "react-router-dom";
import { useOrg } from "../../context/OrgContext";
import { useSort } from "../../hooks/useSort";
import { SortableColumnHeader } from "../../components/SortableColumnHeader";
import {
  fetchDependencyGraph,
  fetchDependencyGraphTable,
  type DependencyGraphTableQuery,
} from "../../api";
import type {
  DependencyGraphResponse,
  DependencyGraphTableResponse,
  DependencyTableRow,
  SharedCookbook,
} from "../../types";
import { ForceGraph, adaptDependencyNodes, adaptDependencyEdges } from "../../components/force-graph";
import {
  LoadingSpinner,
  ErrorAlert,
  EmptyState,
} from "../../components/Feedback";
import { Pagination } from "../../components/Pagination";

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
            className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
              viewMode === "graph"
                ? "bg-blue-50 text-blue-700"
                : "text-gray-600 hover:text-gray-900"
            }`}
            onClick={() => setViewMode("graph")}
          >
            <svg
              className="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M7.217 10.907a2.25 2.25 0 1 0 0 2.186m0-2.186c.18.324.283.696.283 1.093s-.103.77-.283 1.093m0-2.186 9.566-5.314m-9.566 7.5 9.566 5.314m0 0a2.25 2.25 0 1 0 3.935 2.186 2.25 2.25 0 0 0-3.935-2.186Zm0-12.814a2.25 2.25 0 1 0 3.933-2.185 2.25 2.25 0 0 0-3.933 2.185Z"
              />
            </svg>
            Graph
          </button>
          <button
            className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
              viewMode === "table"
                ? "bg-blue-50 text-blue-700"
                : "text-gray-600 hover:text-gray-900"
            }`}
            onClick={() => setViewMode("table")}
          >
            <svg
              className="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 0 1-1.125-1.125M3.375 19.5h7.5c.621 0 1.125-.504 1.125-1.125m-9.75 0V5.625m0 12.75v-1.5c0-.621.504-1.125 1.125-1.125m18.375 2.625V5.625m0 12.75c0 .621-.504 1.125-1.125 1.125m1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125m0 3.75h-7.5A1.125 1.125 0 0 1 12 18.375m9.75-12.75c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125m19.5 0v1.5c0 .621-.504 1.125-1.125 1.125M2.25 5.625v1.5c0 .621.504 1.125 1.125 1.125m0 0h17.25m-17.25 0h7.5c.621 0 1.125.504 1.125 1.125M3.375 8.25c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125m17.25-3.75h-7.5c-.621 0-1.125.504-1.125 1.125m8.625-1.125c.621 0 1.125.504 1.125 1.125v1.5c0 .621-.504 1.125-1.125 1.125m-17.25 0h7.5m-7.5 0c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125M12 10.875v-1.5m0 1.5c0 .621-.504 1.125-1.125 1.125M12 10.875c0 .621.504 1.125 1.125 1.125m-2.25 0c.621 0 1.125.504 1.125 1.125M10.875 12h-1.5m1.5 0c.621 0 1.125.504 1.125 1.125M12 12h7.5m-7.5 0c0 .621-.504 1.125-1.125 1.125M21.375 12c.621 0 1.125.504 1.125 1.125v1.5c0 .621-.504 1.125-1.125 1.125m-17.25 0h7.5m-7.5 0c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125m17.25-3.75h-7.5c-.621 0-1.125.504-1.125 1.125m8.625-1.125c.621 0 1.125.504 1.125 1.125v1.5c0 .621-.504 1.125-1.125 1.125m-2.25 0h.008v.008h-.008v-.008Zm0-3.75h.008v.008h-.008V12Zm0-3.75h.008v.008h-.008V8.25Z"
              />
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

function GraphView({ organisation }: { organisation: string }) {
  const [data, setData] = useState<DependencyGraphResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Interaction state
  const [searchTerm, setSearchTerm] = useState("");
  const [filterType, setFilterType] = useState<"all" | "role" | "cookbook">(
    "all",
  );
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
          <span className="stat-value text-gray-800">
            {data.summary.total_nodes}
          </span>
        </div>
        <div className="stat-card">
          <span className="stat-label">Total Edges</span>
          <span className="stat-value text-gray-800">
            {data.summary.total_edges}
          </span>
        </div>
        <div className="stat-card">
          <span className="stat-label">Roles</span>
          <span className="stat-value text-blue-600">
            {data.summary.role_count}
          </span>
        </div>
        <div className="stat-card">
          <span className="stat-label">Cookbooks</span>
          <span className="stat-value text-emerald-600">
            {data.summary.cookbook_count}
          </span>
        </div>
      </div>

      {/* Filters */}
      <div className="card">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          {/* Search */}
          <div className="relative flex-1">
            <svg
              className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z"
              />
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
                className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
                  filterType === type
                    ? type === "role"
                      ? "bg-blue-100 text-blue-800"
                      : type === "cookbook"
                        ? "bg-emerald-100 text-emerald-800"
                        : "bg-gray-200 text-gray-800"
                    : "bg-gray-100 text-gray-500 hover:bg-gray-200"
                }`}
              >
                {type === "all"
                  ? "All"
                  : type === "role"
                    ? "Roles"
                    : "Cookbooks"}
              </button>
            ))}
          </div>

          {/* Clear selection */}
          {selectedNodeId && (
            <button
              onClick={() => setSelectedNodeId(null)}
              className="flex items-center gap-1 rounded-md bg-gray-100 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200"
            >
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
                  d="M6 18 18 6M6 6l12 12"
                />
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
              <line
                x1="0"
                y1="6"
                x2="32"
                y2="6"
                stroke="#94a3b8"
                strokeWidth="1.5"
              />
              <polygon points="28,3 32,6 28,9" fill="#94a3b8" />
            </svg>
            Depends on
          </span>
          <span className="ml-auto text-[10px] text-gray-400">
            Click a node to highlight its connections • Drag nodes to reposition
          </span>
        </div>
      </div>


      {/* Graph canvas */}
      <ForceGraph
        nodes={adaptDependencyNodes(data.nodes)}
        edges={adaptDependencyEdges(data.edges)}
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

// ===========================================================================
// Table View
// ===========================================================================

function TableView({ organisation }: { organisation: string }) {
  const [data, setData] = useState<DependencyGraphTableResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Sort & pagination
  type DepTableSortField =
    | "role_name"
    | "cookbook_count"
    | "role_count"
    | "total_dependencies";
  const {
    sortField,
    sortOrder,
    handleSort: rawHandleSort,
  } = useSort<DepTableSortField>({
    defaultField: "total_dependencies",
    defaultOrder: "desc",
    descendingFields: ["cookbook_count", "role_count", "total_dependencies"],
  });
  const [page, setPage] = useState(1);
  const perPage = SMALL_PAGE_SIZE;

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

  const handleSort = (field: DepTableSortField) => {
    rawHandleSort(field);
    setPage(1);
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
              <SortableColumnHeader
                label="Role Name"
                field="role_name"
                currentField={sortField}
                currentOrder={sortOrder}
                onSort={handleSort}
              />
              <SortableColumnHeader
                label="Cookbooks"
                field="cookbook_count"
                currentField={sortField}
                currentOrder={sortOrder}
                onSort={handleSort}
              />
              <SortableColumnHeader
                label="Roles"
                field="role_count"
                currentField={sortField}
                currentOrder={sortOrder}
                onSort={handleSort}
              />
              <SortableColumnHeader
                label="Total Deps"
                field="total_dependencies"
                currentField={sortField}
                currentOrder={sortOrder}
                onSort={handleSort}
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
            className={`inline-block h-4 w-4 text-gray-400 transition-transform duration-200 ${
              isExpanded ? "rotate-90" : ""
            }`}
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="m8.25 4.5 7.5 7.5-7.5 7.5"
            />
          </svg>
        </td>
        <td>
          <Link
            to={`/roles/${encodeURIComponent(row.role_name)}`}
            className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
          >
            {row.role_name}
          </Link>
        </td>
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
              <Link
                key={`${d.type}:${d.name}`}
                to={
                  d.type === "cookbook"
                    ? `/cookbooks/${encodeURIComponent(d.name)}`
                    : `/roles/${encodeURIComponent(d.name)}`
                }
                onClick={(e) => e.stopPropagation()}
                className={`inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[10px] font-medium transition-colors ${
                  d.type === "cookbook"
                    ? "bg-emerald-50 text-emerald-700 hover:bg-emerald-100"
                    : "bg-blue-50 text-blue-700 hover:bg-blue-100"
                }`}
              >
                <span
                  className={`inline-block h-1 w-1 ${d.type === "cookbook" ? "rounded-full bg-emerald-500" : "rounded-sm bg-blue-500"}`}
                />
                {d.name}
              </Link>
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
                      <Link
                        key={d.name}
                        to={`/roles/${encodeURIComponent(d.name)}`}
                        className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2.5 py-1 text-xs font-medium text-blue-700 ring-1 ring-inset ring-blue-600/20 transition-colors hover:bg-blue-100"
                      >
                        <span className="inline-block h-1.5 w-1.5 rounded-sm bg-blue-500" />
                        {d.name}
                      </Link>
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
