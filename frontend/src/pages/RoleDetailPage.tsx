// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { useTargetChefVersion } from "../hooks/useTargetChefVersion";
import { fetchRoleDetail, fetchRoleDependencyGraph } from "../api";
import type {
  RoleDetailResponse,
  RoleChainNode,
  RoleGraphResponse,
} from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { CookStyleBadge, TKBadge } from "../components/StatusBadge";
import {
  ForceGraph,
  adaptRoleGraphNodes,
  adaptRoleGraphEdges,
} from "../components/force-graph";

function CompatibilitySummary({ detail }: { detail: RoleDetailResponse }) {
  const total = detail.transitive_cookbooks?.length ?? 0;
  const blocking = detail.blocking_cookbooks?.length ?? 0;
  const compatible = total - blocking;

  // Walk chain tree to collect TK status counts.
  const tkCounts = { passed: 0, failed: 0, partial: 0, untested: 0, git: 0 };
  function walkTK(node: RoleChainNode | undefined) {
    if (!node) return;
    if (node.type === "cookbook") {
      const hasGit = node.source === "git" || node.source === "both";
      if (hasGit) {
        tkCounts.git++;
        const s = node.tk_status ?? "untested";
        if (s === "passed") tkCounts.passed++;
        else if (s === "failed") tkCounts.failed++;
        else if (s === "partial") tkCounts.partial++;
        else tkCounts.untested++;
      }
    }
    node.children?.forEach(walkTK);
  }
  walkTK(detail.nested_role_chain ?? undefined);

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-3 gap-4">
        <div className="rounded-lg border border-green-200 bg-green-50 p-4 text-center">
          <div className="text-2xl font-bold text-green-700">{compatible}</div>
          <div className="text-xs text-green-600">CS Compatible</div>
        </div>
        <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-center">
          <div className="text-2xl font-bold text-red-700">{blocking}</div>
          <div className="text-xs text-red-600">CS Blocked</div>
        </div>
        <div className="rounded-lg border border-gray-200 bg-gray-50 p-4 text-center">
          <div className="text-2xl font-bold text-gray-700">{total}</div>
          <div className="text-xs text-gray-600">Total Cookbooks</div>
        </div>
      </div>
      {tkCounts.git > 0 && (
        <div className="grid grid-cols-4 gap-3">
          <div className="rounded-lg border border-green-200 bg-green-50 p-3 text-center">
            <div className="text-lg font-bold text-green-700">{tkCounts.passed}</div>
            <div className="text-[10px] text-green-600">TK Passed</div>
          </div>
          <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-center">
            <div className="text-lg font-bold text-red-700">{tkCounts.failed}</div>
            <div className="text-[10px] text-red-600">TK Failed</div>
          </div>
          <div className="rounded-lg border border-orange-200 bg-orange-50 p-3 text-center">
            <div className="text-lg font-bold text-orange-700">{tkCounts.partial}</div>
            <div className="text-[10px] text-orange-600">TK Partial</div>
          </div>
          <div className="rounded-lg border border-gray-200 bg-gray-50 p-3 text-center">
            <div className="text-lg font-bold text-gray-700">{tkCounts.untested}</div>
            <div className="text-[10px] text-gray-600">TK Untested</div>
          </div>
        </div>
      )}
    </div>
  );
}

function BlockingCookbooksTable({ detail }: { detail: RoleDetailResponse }) {
  if (!detail.blocking_cookbooks || detail.blocking_cookbooks.length === 0) {
    return (
      <p className="text-sm text-gray-500 italic">
        No blocking cookbooks — all transitive cookbooks are compatible or
        untested.
      </p>
    );
  }

  return (
    <div className="table-container">
      <table className="table">
        <thead>
          <tr>
            <th>Cookbook</th>
            <th>Version</th>
            <th>Complexity</th>
            <th>Auto-fix</th>
            <th>Manual</th>
            <th>Path</th>
          </tr>
        </thead>
        <tbody>
          {detail.blocking_cookbooks.map((cb) => (
            <tr key={cb.cookbook_name}>
              <td>
                <Link
                  to={`/cookbooks/${encodeURIComponent(cb.cookbook_name)}`}
                  className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
                >
                  {cb.cookbook_name}
                </Link>
              </td>
              <td>
                <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600">
                  {cb.cookbook_version}
                </span>
              </td>
              <td>
                <span
                  className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold ring-1 ring-inset ${
                    cb.complexity_label === "critical"
                      ? "bg-red-100 text-red-800 ring-red-600/20"
                      : cb.complexity_label === "high"
                        ? "bg-orange-100 text-orange-800 ring-orange-600/20"
                        : cb.complexity_label === "medium"
                          ? "bg-yellow-100 text-yellow-800 ring-yellow-600/20"
                          : "bg-green-100 text-green-800 ring-green-600/20"
                  }`}
                >
                  {cb.complexity_label} ({cb.complexity_score})
                </span>
              </td>
              <td className="text-right text-sm">{cb.auto_correctable}</td>
              <td className="text-right text-sm">{cb.manual_fix}</td>
              <td>
                <span className="text-xs text-gray-500">
                  {cb.dependency_path?.join(" → ") || "—"}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function BlastRadiusSection({ detail }: { detail: RoleDetailResponse }) {
  return (
    <div className="grid gap-4 md:grid-cols-3">
      <div>
        <h4 className="mb-2 text-sm font-semibold text-gray-700">
          By Organisation
        </h4>
        {detail.nodes_by_organisation?.length > 0 ? (
          <ul className="space-y-1">
            {detail.nodes_by_organisation.map((o) => (
              <li
                key={o.organisation}
                className="flex items-center justify-between text-sm"
              >
                <span className="text-gray-600">{o.organisation}</span>
                <span className="font-medium text-gray-800">{o.count}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-gray-400 italic">No data</p>
        )}
      </div>
      <div>
        <h4 className="mb-2 text-sm font-semibold text-gray-700">
          By Environment
        </h4>
        {detail.nodes_by_environment?.length > 0 ? (
          <ul className="space-y-1">
            {detail.nodes_by_environment.map((e) => (
              <li
                key={e.environment}
                className="flex items-center justify-between text-sm"
              >
                <span className="text-gray-600">{e.environment}</span>
                <span className="font-medium text-gray-800">{e.count}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-gray-400 italic">No data</p>
        )}
      </div>
      <div>
        <h4 className="mb-2 text-sm font-semibold text-gray-700">
          By Platform
        </h4>
        {detail.nodes_by_platform?.length > 0 ? (
          <ul className="space-y-1">
            {detail.nodes_by_platform.map((p) => (
              <li
                key={`${p.platform}-${p.platform_version}`}
                className="flex items-center justify-between text-sm"
              >
                <span className="text-gray-600">
                  {p.platform} {p.platform_version}
                </span>
                <span className="font-medium text-gray-800">{p.count}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-gray-400 italic">No data</p>
        )}
      </div>
    </div>
  );
}

function RoleChainTree({
  node,
  depth,
}: {
  node: RoleChainNode;
  depth: number;
}) {
  const indent = depth * 1.25;
  const isRole = node.type === "role";

  const linkTarget = isRole
    ? `/roles/${encodeURIComponent(node.name)}`
    : `/cookbooks/${encodeURIComponent(node.name)}`;

  const sourceIcon = isRole
    ? "📁"
    : node.source === "both"
      ? "📦🔀"
      : node.source === "git"
        ? "🔀"
        : "📦";

  const sourceTitle = isRole
    ? "Role"
    : node.source === "both"
      ? "Server cookbook + Git repo"
      : node.source === "git"
        ? "Git repo only"
        : "Server cookbook only";

  return (
    <div>
      <div
        className="flex items-center gap-1.5 py-0.5"
        style={{ paddingLeft: `${indent}rem` }}
      >
        <span className="text-xs text-gray-400" title={sourceTitle}>
          {sourceIcon}
        </span>
        <Link
          to={linkTarget}
          className={`text-sm hover:underline ${isRole ? "font-medium text-blue-600" : "text-gray-800"}`}
        >
          {node.name}
        </Link>
        {!isRole && (
          <CookStyleBadge
            status={node.compatibility_status ?? "untested"}
            size="sm"
          />
        )}
        {!isRole &&
          (node.source === "git" || node.source === "both") && (
            <TKBadge status={node.tk_status ?? "untested"} size="sm" />
          )}
      </div>
      {node.children?.map((child, i) => (
        <RoleChainTree
          key={`${child.type}-${child.name}-${i}`}
          node={child}
          depth={depth + 1}
        />
      ))}
    </div>
  );
}

type DetailTab = "overview" | "graph";

function RoleDependencyGraphTab({
  name,
  targetVersion,
}: {
  name: string;
  targetVersion?: string;
}) {
  const [graph, setGraph] = useState<RoleGraphResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState("");
  const [filterType, setFilterType] = useState<"all" | "role" | "cookbook">(
    "all",
  );
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchRoleDependencyGraph(name, { target_chef_version: targetVersion })
      .then((res) => {
        setGraph(res);
        setSelectedNodeId(null);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name, targetVersion]);

  useEffect(() => {
    load();
  }, [load]);

  if (loading) return <LoadingSpinner message="Loading dependency graph…" />;
  if (error) return <ErrorAlert message={error} onRetry={load} />;
  if (!graph || graph.nodes.length === 0) {
    return (
      <p className="text-sm text-gray-500 italic">
        No dependency graph data available for this role.
      </p>
    );
  }

  return (
    <div className="space-y-4">
      {/* Metadata summary */}
      <div className="flex flex-wrap gap-4 text-sm">
        <span className="rounded-full bg-blue-50 px-3 py-1 font-medium text-blue-700">
          {graph.metadata.total_roles} Roles
        </span>
        <span className="rounded-full bg-emerald-50 px-3 py-1 font-medium text-emerald-700">
          {graph.metadata.total_cookbooks} Cookbooks
        </span>
        {graph.metadata.incompatible_cookbooks > 0 && (
          <span className="rounded-full bg-red-50 px-3 py-1 font-medium text-red-700">
            {graph.metadata.incompatible_cookbooks} Incompatible
          </span>
        )}
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
            Compatible
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-3 rounded-full bg-red-500" />
            Incompatible
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-3 rounded-full bg-gray-400" />
            Untested
          </span>
          <span className="ml-auto text-[10px] text-gray-400">
            Click a node to highlight connections · Drag to reposition
          </span>
        </div>
      </div>

      {/* Graph */}
      <ForceGraph
        nodes={adaptRoleGraphNodes(graph.nodes)}
        edges={adaptRoleGraphEdges(graph.edges)}
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

export function RoleDetailPage() {
  const { name } = useParams<{ name: string }>();
  const [detail, setDetail] = useState<RoleDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<DetailTab>("overview");

  const { selectedVersion: targetVersion } = useTargetChefVersion({});

  const load = useCallback(() => {
    if (!name) return;
    setLoading(true);
    setError(null);
    fetchRoleDetail(name, targetVersion)
      .then((res) => setDetail(res))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name, targetVersion]);

  useEffect(() => {
    load();
  }, [load]);

  if (loading) return <LoadingSpinner message="Loading role detail…" />;
  if (error) return <ErrorAlert message={error} onRetry={load} />;
  if (!detail) return <ErrorAlert message="Role not found." />;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <div className="flex items-center gap-2">
          <Link
            to="/roles"
            className="text-sm text-blue-600 hover:text-blue-800 hover:underline"
          >
            ← Roles
          </Link>
        </div>
        <h2 className="mt-2 text-xl font-bold text-gray-800">
          {detail.role_name}
        </h2>
        <div className="mt-1 flex flex-wrap items-center gap-4 text-sm text-gray-600">
          <span>Organisations: {detail.organisations?.join(", ") || "—"}</span>
          <span>
            Nodes:{" "}
            <Link
              to={`/nodes?role=${encodeURIComponent(detail.role_name)}`}
              className="font-medium text-blue-600 hover:underline"
            >
              {detail.node_count.toLocaleString()}
            </Link>
          </span>
        </div>
      </div>

      {/* Tab bar */}
      <div className="flex gap-1 border-b border-gray-200">
        {(["overview", "graph"] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === tab
                ? "border-b-2 border-blue-600 text-blue-600"
                : "text-gray-500 hover:text-gray-700"
            }`}
          >
            {tab === "overview" ? "Overview" : "Dependency Graph"}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {activeTab === "overview" ? (
        <>
          {/* Compatibility Summary */}
          <section>
            <h3 className="mb-3 text-lg font-semibold text-gray-700">
              Compatibility Summary
            </h3>
            <CompatibilitySummary detail={detail} />
          </section>

          {/* Blocking Cookbooks */}
          <section>
            <h3 className="mb-3 text-lg font-semibold text-gray-700">
              Blocking Cookbooks
            </h3>
            <BlockingCookbooksTable detail={detail} />
          </section>

          {/* Blast Radius */}
          <section>
            <h3 className="mb-3 text-lg font-semibold text-gray-700">
              Blast Radius
            </h3>
            <BlastRadiusSection detail={detail} />
          </section>

          {/* Nested Role Chain */}
          {detail.nested_role_chain && (
            <section>
              <h3 className="mb-3 text-lg font-semibold text-gray-700">
                Dependency Tree
              </h3>
              <div className="rounded-lg border border-gray-200 bg-white p-4">
                <RoleChainTree node={detail.nested_role_chain} depth={0} />
              </div>
            </section>
          )}

          {/* Direct Dependencies */}
          <section>
            <h3 className="mb-3 text-lg font-semibold text-gray-700">
              Direct Dependencies
            </h3>
            <div className="grid gap-4 md:grid-cols-2">
              <div>
                <h4 className="mb-2 text-sm font-semibold text-gray-600">
                  Cookbooks ({detail.direct_cookbooks?.length ?? 0})
                </h4>
                <div className="flex flex-wrap gap-1.5">
                  {detail.direct_cookbooks?.map((cb) => (
                    <Link
                      key={cb}
                      to={`/cookbooks/${encodeURIComponent(cb)}`}
                      className="rounded bg-blue-50 px-2 py-0.5 text-xs text-blue-700 hover:bg-blue-100"
                    >
                      {cb}
                    </Link>
                  ))}
                  {(!detail.direct_cookbooks ||
                    detail.direct_cookbooks.length === 0) && (
                    <span className="text-sm text-gray-400 italic">None</span>
                  )}
                </div>
              </div>
              <div>
                <h4 className="mb-2 text-sm font-semibold text-gray-600">
                  Nested Roles ({detail.direct_roles?.length ?? 0})
                </h4>
                <div className="flex flex-wrap gap-1.5">
                  {detail.direct_roles?.map((r) => (
                    <Link
                      key={r}
                      to={`/roles/${encodeURIComponent(r)}`}
                      className="rounded bg-purple-50 px-2 py-0.5 text-xs text-purple-700 hover:bg-purple-100"
                    >
                      {r}
                    </Link>
                  ))}
                  {(!detail.direct_roles ||
                    detail.direct_roles.length === 0) && (
                    <span className="text-sm text-gray-400 italic">None</span>
                  )}
                </div>
              </div>
            </div>
          </section>
        </>
      ) : (
        <RoleDependencyGraphTab name={name!} targetVersion={targetVersion} />
      )}
    </div>
  );
}
