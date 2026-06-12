import { useState, useEffect, useCallback, useRef, Fragment } from "react";
import { useParams, Link } from "react-router-dom";
import {
  fetchNodeDetail,
  fetchFilterTargetChefVersions,
  fetchNodeKitchenRuns,
  triggerNodeKitchenRun,
  fetchNodeDependencyGraph,
} from "../api";
import type {
  NodeDetailResponse,
  NodeReadiness,
  BlockingCookbook,
  NodeKitchenRun,
  NodeDependencyGraphResponse,
} from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { StaleBadge, StatusBadge, DiskBadge, CookStyleBadge, TKBadge, DeploymentStateBadge, ConvergeBadge } from "../components/StatusBadge";
import type { NodeSnapshot } from "../types";

// Helper to build the disk detail link for a node.
function diskDetailPath(org: string, name: string): string {
  return `/nodes/${encodeURIComponent(org)}/${encodeURIComponent(name)}/disks`;
}

// ---------------------------------------------------------------------------
// Disk space analysis panel
// ---------------------------------------------------------------------------

function DiskSpacePanel({
  sufficient,
  available,
  required,
  stale,
  org,
  nodeName,
  installPath,
  minRemainingFreePercent,
}: {
  // Version-invariant disk verdict, sourced from the node snapshot (not a
  // per-target readiness row), so the panel renders even with no readiness rows.
  sufficient?: boolean | null;
  available?: number | null;
  required?: number | null;
  stale?: boolean;
  org?: string;
  nodeName?: string;
  installPath?: string;
  minRemainingFreePercent?: number;
}) {
  // Unknown / stale state
  if (sufficient === null || sufficient === undefined) {
    const reason = stale
      ? "Node data is stale — disk space cannot be determined."
      : "Disk space information is not available for this node.";
    return (
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-3">
        <div className="flex items-center gap-2">
          <span className="text-lg text-gray-400">💾</span>
          <span className="text-sm font-medium text-gray-600">Disk Space</span>
          <span className="rounded bg-gray-200 px-1.5 py-0.5 text-xs font-medium text-gray-600">
            Unknown
          </span>
        </div>
        <p className="mt-1 text-xs text-gray-500">{reason}</p>
        {required != null && (
          <p className="mt-0.5 text-xs text-gray-400">
            Minimum required: {formatMB(required)}
          </p>
        )}
        {org && nodeName && (
          <Link
            to={diskDetailPath(org, nodeName)}
            className="mt-1.5 inline-block text-xs text-blue-600 hover:text-blue-800 hover:underline"
          >
            View Filesystem Details →
          </Link>
        )}
      </div>
    );
  }

  // Known state
  const borderColor = sufficient ? "border-green-200" : "border-red-200";
  const bgColor = sufficient ? "bg-green-50" : "bg-red-50";
  const badgeBg = sufficient
    ? "bg-green-100 text-green-700"
    : "bg-red-100 text-red-700";

  return (
    <div className={`rounded-lg border ${borderColor} ${bgColor} p-3`}>
      <div className="flex items-center gap-2">
        <span className="text-lg">{sufficient ? "✅" : "⚠️"}</span>
        <span className="text-sm font-medium text-gray-700">Disk Space</span>
        <span
          className={`rounded px-1.5 py-0.5 text-xs font-medium ${badgeBg}`}
        >
          {sufficient ? "Sufficient" : "Insufficient"}
        </span>
      </div>

      {/* Disk details */}
      {available != null && required != null && (
        <div className="mt-2 space-y-0.5 text-xs text-gray-500">
          <div>
            Available:{" "}
            <strong className="text-gray-700">{formatMB(available)}</strong>
            {installPath && (
              <span className="text-gray-400"> on <code className="font-mono">{installPath}</code></span>
            )}
          </div>
          <div>
            Required:{" "}
            <strong className="text-gray-700">{formatMB(required)}</strong> for install
            {minRemainingFreePercent != null && minRemainingFreePercent > 0 && (
              <span> + <strong className="text-gray-700">{minRemainingFreePercent}%</strong> free after</span>
            )}
          </div>
          <div className={sufficient ? "text-green-600" : "text-red-600"}>
            {sufficient
              ? `✓ ${formatMB(available - required)} headroom after install`
              : `✗ ${formatMB(required - available)} short`}
          </div>
        </div>
      )}

      {available != null && required == null && (
        <p className="mt-1 text-xs text-gray-500">
          Available:{" "}
          <strong>{formatMB(available)}</strong>
          {installPath && (
            <span className="text-gray-400"> on <code className="font-mono">{installPath}</code></span>
          )}{" "}
          (no minimum configured)
        </p>
      )}

      {org && nodeName && (
        <Link
          to={diskDetailPath(org, nodeName)}
          className="mt-2 inline-block text-xs text-blue-600 hover:text-blue-800 hover:underline"
        >
          View Filesystem Details →
        </Link>
      )}
    </div>
  );
}

function formatMB(mb: number): string {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(1)} GB`;
  }
  return `${mb} MB`;
}

// ---------------------------------------------------------------------------
// Readiness card for one target version
// ---------------------------------------------------------------------------

function ReadinessCard({
  r,
  org,
  nodeName,
  targetChefVersion,
}: {
  r: NodeReadiness;
  org?: string;
  nodeName?: string;
  targetChefVersion?: string;
}) {
  const ready = r.is_ready;

  const diskStatus: string = r.sufficient_disk_space === true
    ? "sufficient"
    : r.sufficient_disk_space === false
      ? "insufficient"
      : "unknown";
  const csStatus: string = r.cookstyle_status ?? (r.all_cookbooks_compatible ? "passed" : "unknown");
  const csMapped: string = csStatus === "passed" ? "compatible" : csStatus === "failed" ? "incompatible" : "untested";
  const tkStatus: string = r.kitchen_status ?? "unknown";

  return (
    <div
      className={`rounded-lg border p-4 ${ready ? "border-green-200 bg-green-50/30" : "border-red-200 bg-red-50/20"}`}
    >
      {/* Header */}
      <div className="flex items-center gap-3">
        <StatusBadge variant={ready ? "ready" : "blocked"} />
        <div>
          <div className="text-sm font-semibold text-gray-800">
            Target: Chef Infra Client {r.target_chef_version}
          </div>
          <div className="text-xs text-gray-400">
            Evaluated {new Date(r.evaluated_at).toLocaleString()}
          </div>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <DiskBadge status={diskStatus} size="sm" />
          <CookStyleBadge status={csMapped} size="sm" />
          <TKBadge status={tkStatus} size="sm" />
          {r.stale_data && (
            <StaleBadge isStale size="sm" />
          )}
        </div>
      </div>

      {/* Overall verdict */}
      <div
        className={`mt-3 rounded-lg px-3 py-2 text-sm ${ready ? "bg-green-100 text-green-800" : "bg-red-100 text-red-800"}`}
      >
        {ready ? (
          <span className="flex items-center gap-2">
            <span className="text-base">🟢</span>
            This node is <strong>ready</strong> to upgrade — all cookbooks are
            compatible and disk space is sufficient.
          </span>
        ) : (
          <span className="flex items-center gap-2">
            <span className="text-base">🔴</span>
            This node is <strong>blocked</strong> from upgrading
            {!r.all_cookbooks_compatible && r.sufficient_disk_space !== true
              ? " — incompatible cookbooks and disk space issues."
              : !r.all_cookbooks_compatible
                ? " — one or more cookbooks are incompatible or untested."
                : r.sufficient_disk_space === false
                  ? " — insufficient disk space."
                  : " — disk space could not be determined."}
          </span>
        )}
      </div>

      {/* Analysis panel: dependency tree. Disk space is version-invariant, so it
          renders once at the node level (below) rather than per readiness card. */}
      <div className="mt-4 space-y-3">
        {org && nodeName && (
          <ReadinessDependencyTree
            org={org}
            nodeName={nodeName}
            targetChefVersion={targetChefVersion}
            blockingCookbooks={r.blocking_cookbooks}
          />
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Dependency tree embedded in readiness — shows run_list → roles → cookbooks
// with version, complexity, and CS/TK badges on cookbook nodes.
// ---------------------------------------------------------------------------

function ReadinessDependencyTree({
  org,
  nodeName,
  targetChefVersion,
  blockingCookbooks,
}: {
  org: string;
  nodeName: string;
  targetChefVersion?: string;
  blockingCookbooks?: BlockingCookbook[] | null;
}) {
  const [graphData, setGraphData] =
    useState<NodeDependencyGraphResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    fetchNodeDependencyGraph(org, nodeName, targetChefVersion)
      .then(setGraphData)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [org, nodeName, targetChefVersion]);

  if (loading) {
    return (
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-3">
        <span className="text-xs text-gray-400">Loading dependency tree…</span>
      </div>
    );
  }

  if (!graphData || graphData.nodes.length === 0) {
    return null;
  }

  const incompatibleCount = graphData.metadata.incompatible_cookbooks;
  const tkFailedCount = graphData.metadata.tk_failed_cookbooks;
  const allGood = incompatibleCount === 0 && tkFailedCount === 0;

  // Build adjacency and node lookup.
  const childrenOf = new Map<string, string[]>();
  for (const edge of graphData.edges) {
    const existing = childrenOf.get(edge.from) ?? [];
    existing.push(edge.to);
    childrenOf.set(edge.from, existing);
  }
  const nodeById = new Map(graphData.nodes.map((n) => [n.id, n]));
  const roots = graphData.nodes.filter((n) => n.type === "run_list_entry");

  // Build blocking cookbook lookup for expandable verdicts.
  const blockingByName = new Map<string, BlockingCookbook>();
  if (blockingCookbooks) {
    for (const bc of blockingCookbooks) {
      blockingByName.set(bc.name, bc);
    }
  }

  function renderNode(nodeId: string, depth: number, visited: Set<string>) {
    if (visited.has(nodeId)) return null;
    visited.add(nodeId);

    const node = nodeById.get(nodeId);
    if (!node) return null;

    const indent = depth * 1.25;
    const children = childrenOf.get(nodeId) ?? [];
    const isRole = node.type === "role";
    const isCookbook = node.type === "cookbook";
    const isRunList = node.type === "run_list_entry";
    const isBlocking = isCookbook && blockingByName.has(node.name);

    const linkTarget = isRole
      ? `/roles/${encodeURIComponent(node.name)}`
      : isCookbook
        ? `/cookbooks/${encodeURIComponent(node.name)}`
        : undefined;

    const icon = isRunList ? "📋" : isRole ? "📁" : "📦";

    return (
      <div key={nodeId}>
        <div
          className={`flex flex-wrap items-center gap-1.5 py-0.5 ${isBlocking ? "rounded bg-red-50/50" : ""}`}
          style={{ paddingLeft: `${indent}rem` }}
        >
          <span className="text-xs text-gray-400">{icon}</span>
          {linkTarget ? (
            <Link
              to={linkTarget}
              className={`text-sm hover:underline ${
                isRole ? "font-medium text-blue-600" : isBlocking ? "font-medium text-red-700" : "text-gray-800"
              }`}
            >
              {node.name}
            </Link>
          ) : (
            <span className="text-sm font-medium text-purple-700">
              {node.name}
            </span>
          )}
          {isCookbook && node.version && (
            <span className="text-xs text-gray-400">@{node.version}</span>
          )}
          {isCookbook && (
            <>
              <CookStyleBadge
                status={node.compatibility_status ?? "untested"}
                size="sm"
              />
              <TKBadge status={node.tk_status ?? "untested"} size="sm" />
            </>
          )}
          {isCookbook && node.complexity_score != null && node.complexity_score > 0 && (
            <span
              className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${
                node.complexity_label === "critical"
                  ? "bg-red-100 text-red-700"
                  : node.complexity_label === "high"
                    ? "bg-orange-100 text-orange-700"
                    : node.complexity_label === "medium"
                      ? "bg-yellow-100 text-yellow-700"
                      : "bg-green-100 text-green-700"
              }`}
            >
              {node.complexity_score} CS offenses
            </span>
          )}
        </div>
        {children.map((childId) => renderNode(childId, depth + 1, visited))}
      </div>
    );
  }

  return (
    <div
      className={`rounded-lg border p-3 ${allGood ? "border-green-200 bg-green-50" : "border-red-200 bg-red-50/30"}`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-lg">📦</span>
        <span className="text-sm font-medium text-gray-700">
          Cookbook Dependencies
        </span>
        <span className="rounded-full bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700">
          {graphData.metadata.total_roles} roles
        </span>
        <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
          {graphData.metadata.total_cookbooks} cookbooks
        </span>
        {incompatibleCount > 0 && (
          <span className="rounded-full bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700">
            {incompatibleCount} CS incompatible
          </span>
        )}
        {tkFailedCount > 0 && (
          <span className="rounded-full bg-orange-50 px-2 py-0.5 text-xs font-medium text-orange-700">
            {tkFailedCount} TK failed
          </span>
        )}
      </div>
      <div className="mt-2 space-y-0.5">
        {roots.map((root) => renderNode(root.id, 0, new Set<string>()))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Readiness section (container)
// ---------------------------------------------------------------------------

function ReadinessSection({
  data,
  org,
  nodeName,
}: {
  data: NodeDetailResponse;
  org?: string;
  nodeName?: string;
}) {
  if (!data.readiness || data.readiness.length === 0) return null;

  return (
    <div className="card">
      <h3 className="card-header">Upgrade Readiness</h3>
      <div className="space-y-6">
        {data.readiness.map((r) => (
          <ReadinessCard
            key={r.id}
            r={r}
            org={org}
            nodeName={nodeName}
            targetChefVersion={r.target_chef_version}
          />
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Deployment State panel (parallel deployment tracking)
// ---------------------------------------------------------------------------

/** Maps raw migration_state values to UI labels. */
function migrationStateLabel(raw?: string | null): string {
  switch (raw) {
    case "omnibus_only":
      return "Current only";
    case "hab_dormant":
      return "Staged";
    case "hab_active":
      return "Activated";
    default:
      return "";
  }
}

function DeploymentStatePanel({ node }: { node: NodeSnapshot }) {
  // Hide panel entirely when migration cookbook is not deployed
  if (!node.migration_state) return null;

  const label = migrationStateLabel(node.migration_state);
  const readyToActivate =
    node.migration_state === "hab_dormant" &&
    node.target_converge_status === "success";

  const hasConvergeData = !!node.target_converge_status;

  return (
    <div className="card">
      <h3 className="card-header">Deployment State</h3>
      <div className="space-y-4">
        {/* Ready to Activate callout */}
        {readyToActivate && (
          <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3">
            <div className="flex items-center gap-2">
              <span className="text-lg">🟢</span>
              <span className="text-sm font-semibold text-green-800">
                Ready to Activate
              </span>
            </div>
            <p className="mt-1 text-xs text-green-700">
              Target version is staged and the last speculative converge passed.
              This node can be safely switched to the new version.
            </p>
          </div>
        )}

        {/* State + versions */}
        <div className="flex flex-wrap gap-x-6 gap-y-2 text-sm">
          <span className="flex items-center gap-2">
            <span className="text-gray-400">State:</span>
            <DeploymentStateBadge state={label} />
          </span>
          {node.active_chef_version && (
            <span>
              <span className="text-gray-400">Active version:</span>{" "}
              <span className="font-medium text-gray-800">
                {node.active_chef_version}
              </span>
            </span>
          )}
          {node.dormant_chef_version && (
            <span>
              <span className="text-gray-400">Staged version:</span>{" "}
              <span className="font-medium text-gray-800">
                {node.dormant_chef_version}
              </span>
            </span>
          )}
        </div>

        {/* Speculative converge section */}
        {hasConvergeData && (
          <div className="rounded-lg border border-gray-200 bg-gray-50 p-3">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-gray-700">
                Speculative Converge
              </span>
              <ConvergeBadge status={node.target_converge_status} />
            </div>
            <div className="mt-2 flex flex-wrap gap-x-6 gap-y-1 text-xs text-gray-500">
              {node.target_version && (
                <span>
                  Version tested:{" "}
                  <strong className="text-gray-700">
                    {node.target_version}
                  </strong>
                </span>
              )}
              {node.target_execution_time && (
                <span>
                  Last run:{" "}
                  <strong className="text-gray-700">
                    {new Date(node.target_execution_time).toLocaleString()}
                  </strong>
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Node Kitchen Testing section
// ---------------------------------------------------------------------------

function NodeKitchenSection({
  org,
  nodeName,
  targetVersions,
}: {
  org: string;
  nodeName: string;
  targetVersions: string[];
}) {
  const [runs, setRuns] = useState<NodeKitchenRun[]>([]);
  const [loadingRuns, setLoadingRuns] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [targetVersion, setTargetVersion] = useState(targetVersions[0] || "");
  const [cookbookSource, setCookbookSource] = useState<
    "server" | "git" | "hybrid"
  >("server");
  const [triggering, setTriggering] = useState(false);
  const [triggerMsg, setTriggerMsg] = useState<string | null>(null);
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const [expandedError, setExpandedError] = useState<string | null>(null);
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const loadRuns = useCallback(async () => {
    try {
      const data = await fetchNodeKitchenRuns(org, nodeName);
      setRuns(data);
    } catch {
      /* ignore */
    } finally {
      setLoadingRuns(false);
    }
  }, [org, nodeName]);

  useEffect(() => {
    loadRuns();
  }, [loadRuns]);
  useEffect(() => {
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current);
    };
  }, []);

  const handleTrigger = async () => {
    setTriggering(true);
    setTriggerMsg(null);
    try {
      const resp = await triggerNodeKitchenRun({
        node_name: nodeName,
        organisation_name: org,
        target_chef_version: targetVersion,
        cookbook_source: cookbookSource,
      });
      setTriggerMsg(`Started: ${resp.message}`);
      if (pollingRef.current) clearInterval(pollingRef.current);
      const startTime = Date.now();
      pollingRef.current = setInterval(async () => {
        if (Date.now() - startTime > 30 * 60 * 1000) {
          if (pollingRef.current) clearInterval(pollingRef.current);
          return;
        }
        try {
          const fresh = await fetchNodeKitchenRuns(org, nodeName);
          setRuns(fresh);
          if (fresh.length > 0 && fresh[0].completed_at) {
            if (pollingRef.current) clearInterval(pollingRef.current);
            setTriggerMsg(null);
          }
        } catch {
          /* ignore polling errors */
        }
      }, 5000);
    } catch (e: unknown) {
      setTriggerMsg(`Error: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setTriggering(false);
    }
  };

  const convergeIcon = (r: NodeKitchenRun) =>
    r.converge_passed === true
      ? "✅"
      : r.converge_passed === false
        ? "❌"
        : "⏳";
  const verifyIcon = (r: NodeKitchenRun) =>
    r.verify_passed === true ? "✅" : r.verify_passed === false ? "❌" : "—";
  const formatDuration = (s?: number) => {
    if (s == null) return "—";
    return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
  };

  return (
    <div className="card">
      <h3 className="card-header">Node Kitchen Testing</h3>
      <div className="mb-4">
        <button
          className="text-sm text-blue-600 hover:underline"
          onClick={() => setShowForm(!showForm)}
        >
          {showForm ? "▾ Hide Test Form" : "▸ New Test Run…"}
        </button>
        {showForm && (
          <div className="mt-3 rounded border border-gray-200 bg-gray-50 p-4 space-y-3">
            <div className="flex flex-wrap gap-4 items-end">
              <label className="text-sm">
                <span className="block font-medium text-gray-700 mb-1">
                  Target Chef Version
                </span>
                <select
                  className="rounded border border-gray-300 px-2 py-1 text-sm"
                  value={targetVersion}
                  onChange={(e) => setTargetVersion(e.target.value)}
                >
                  {targetVersions.map((v) => (
                    <option key={v} value={v}>
                      {v}
                    </option>
                  ))}
                </select>
              </label>
              <fieldset className="text-sm">
                <legend className="font-medium text-gray-700 mb-1">
                  Cookbook Source
                </legend>
                <div className="flex gap-3">
                  {(["server", "git", "hybrid"] as const).map((src) => (
                    <label
                      key={src}
                      className="flex items-center gap-1 cursor-pointer"
                    >
                      <input
                        type="radio"
                        name="cookbook_source"
                        value={src}
                        checked={cookbookSource === src}
                        onChange={() => setCookbookSource(src)}
                      />
                      <span className="capitalize">{src}</span>
                    </label>
                  ))}
                </div>
              </fieldset>
              <button
                className="rounded bg-blue-600 px-3 py-1 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
                onClick={handleTrigger}
                disabled={triggering || targetVersions.length === 0}
              >
                {triggering ? "Starting…" : "Run Test"}
              </button>
            </div>
            {triggerMsg && (
              <p className="text-sm text-blue-600">{triggerMsg}</p>
            )}
          </div>
        )}
      </div>
      {loadingRuns ? (
        <p className="text-sm text-gray-400">Loading runs…</p>
      ) : runs.length === 0 ? (
        <p className="text-sm text-gray-400">No kitchen runs yet.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left text-xs text-gray-500">
                <th className="py-1 pr-3">Target</th>
                <th className="py-1 pr-3">Source</th>
                <th className="py-1 pr-3">Platform</th>
                <th className="py-1 pr-3">Converge</th>
                <th className="py-1 pr-3">Verify</th>
                <th className="py-1 pr-3">Duration</th>
                <th className="py-1 pr-3">Started</th>
                <th className="py-1 pr-3">Error</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => (
                <Fragment key={r.id}>
                  <tr
                    className="border-b cursor-pointer hover:bg-gray-50"
                    onClick={() =>
                      setExpandedRow(expandedRow === r.id ? null : r.id)
                    }
                  >
                    <td className="py-1 pr-3">{r.target_chef_version}</td>
                    <td className="py-1 pr-3 capitalize">
                      {r.cookbook_source}
                    </td>
                    <td className="py-1 pr-3">{r.platform_name || "—"}</td>
                    <td className="py-1 pr-3">{convergeIcon(r)}</td>
                    <td className="py-1 pr-3">{verifyIcon(r)}</td>
                    <td className="py-1 pr-3">
                      {formatDuration(r.duration_seconds)}
                    </td>
                    <td className="py-1 pr-3">
                      {r.started_at
                        ? new Date(r.started_at).toLocaleString()
                        : "—"}
                    </td>
                    <td className="py-1 pr-3">
                      {r.error_message ? (
                        <span
                          className="text-red-600 cursor-pointer"
                          title={r.error_message}
                          onClick={(e) => {
                            e.stopPropagation();
                            setExpandedError(
                              expandedError === r.id ? null : r.id,
                            );
                          }}
                        >
                          {expandedError === r.id
                            ? r.error_message
                            : r.error_message.slice(0, 40) +
                              (r.error_message.length > 40 ? "…" : "")}
                        </span>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                  {expandedRow === r.id && (
                    <tr>
                      <td colSpan={8} className="bg-gray-50 p-3">
                        <div className="space-y-2">
                          {r.converge_output && (
                            <div>
                              <span className="text-xs font-medium text-gray-500">
                                Converge Output
                              </span>
                              <pre className="mt-1 max-h-48 overflow-auto rounded bg-gray-900 p-2 text-xs text-green-300">
                                {r.converge_output}
                              </pre>
                            </div>
                          )}
                          {r.verify_output && (
                            <div>
                              <span className="text-xs font-medium text-gray-500">
                                Verify Output
                              </span>
                              <pre className="mt-1 max-h-48 overflow-auto rounded bg-gray-900 p-2 text-xs text-green-300">
                                {r.verify_output}
                              </pre>
                            </div>
                          )}
                          {!r.converge_output && !r.verify_output && (
                            <p className="text-xs text-gray-400">
                              No output available yet.
                            </p>
                          )}
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main page component
// ---------------------------------------------------------------------------

export function NodeDetailPage() {
  const { org, name } = useParams<{ org: string; name: string }>();
  const [data, setData] = useState<NodeDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [targetVersions, setTargetVersions] = useState<string[]>([]);

  const load = useCallback(() => {
    if (!org || !name) return;
    setLoading(true);
    setError(null);
    fetchNodeDetail(org, name)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [org, name]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    fetchFilterTargetChefVersions()
      .then((r) => setTargetVersions(r.data))
      .catch(() => {});
  }, []);

  if (loading) return <LoadingSpinner message="Loading node detail…" />;
  if (error) return <ErrorAlert message={error} onRetry={load} />;
  if (!data) return null;

  const node = data.node;

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <Link to="/nodes" className="hover:text-blue-600 hover:underline">
          Nodes
        </Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">{node.node_name}</span>
      </nav>

      {/* Header */}
      <div className="flex items-center gap-3">
        <h2 className="text-xl font-bold text-gray-800">{node.node_name}</h2>
        <StaleBadge
          isStale={node.is_stale}
          stalenesTier={node.staleness_tier}
          ageHours={node.ohai_time_age_hours}
        />
      </div>

      {/* Info bar — compact two-line summary */}
      <div className="card p-3">
        <div className="flex flex-wrap gap-x-6 gap-y-1 text-sm">
          <span>
            <span className="text-gray-400">Org:</span>{" "}
            <span className="font-medium text-gray-800">
              {data.organisation_name || node.organisation_id}
            </span>
          </span>
          <span>
            <span className="text-gray-400">Env:</span>{" "}
            <span className="font-medium text-gray-800">
              {node.chef_environment || "—"}
            </span>
          </span>
          <span>
            <span className="text-gray-400">Chef:</span>{" "}
            <span className="font-medium text-gray-800">
              {node.chef_version || "—"}
            </span>
          </span>
          <span>
            <span className="text-gray-400">Platform:</span>{" "}
            <span className="font-medium text-gray-800">
              {node.platform || "—"} {node.platform_version || ""}
              {node.platform_family ? ` (${node.platform_family})` : ""}
            </span>
          </span>
          {node.policy_name && (
            <span>
              <span className="text-gray-400">Policy:</span>{" "}
              <span className="font-medium text-gray-800">
                {node.policy_name} / {node.policy_group}
              </span>
            </span>
          )}
        </div>
        <div className="mt-1 flex flex-wrap gap-x-6 gap-y-1 text-xs text-gray-400">
          <span>
            Collected:{" "}
            {new Date(node.collected_at).toLocaleString()}
          </span>
          <span>
            Ohai:{" "}
            {node.ohai_time
              ? new Date(Number(node.ohai_time) * 1000).toLocaleString()
              : "—"}
          </span>
          <Link
            to={diskDetailPath(org!, name!)}
            className="text-blue-500 hover:text-blue-700 hover:underline"
          >
            Filesystem Details →
          </Link>
        </div>
      </div>

      {/* Deployment State — parallel deployment tracking */}
      <DeploymentStatePanel node={node} />

      {/* Disk space — version-invariant node-level verdict (self-labelled panel),
          shown even when there are no readiness rows (e.g. no target configured). */}
      <DiskSpacePanel
        sufficient={node.sufficient_disk_space}
        available={node.available_disk_mb}
        required={node.required_disk_mb}
        stale={node.is_stale}
        org={org}
        nodeName={name}
        installPath={data.install_path}
        minRemainingFreePercent={data.min_remaining_free_percent}
      />

      {/* Readiness — promoted above run list / roles / cookbooks for visibility */}
      <ReadinessSection data={data} org={org} nodeName={name} />

      {/* Node Kitchen Testing */}
      {org && name && (
        <NodeKitchenSection
          org={org}
          nodeName={name}
          targetVersions={targetVersions}
        />
      )}
    </div>
  );
}


