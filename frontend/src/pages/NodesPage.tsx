import { useState, useEffect, useCallback } from "react";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { Link, useSearchParams } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import { useGlobalFilters } from "../context/GlobalFilterContext";
import { useSort } from "../hooks/useSort";
import { useTargetChefVersion } from "../hooks/useTargetChefVersion";
import { SortableColumnHeader } from "../components/SortableColumnHeader";
import { FilterInput } from "../components/FilterInputs";
import { FilterMultiCheckbox } from "../components/FilterMultiCheckbox";
import { FilterTypeAhead } from "../components/FilterTypeAhead";
import {
  fetchNodes,
  fetchFilterPolicyNames,
  fetchFilterPolicyGroups,
  fetchFilterEnvironments,
  fetchFilterPlatforms,
  type NodeFilterQuery,
} from "../api";
import type {
  NodeListItem,
  Pagination as PaginationType,
  ExportFilters,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StaleBadge, CookStyleBadge, TKBadge, DiskBadge, DeploymentStateBadge, ConvergeBadge } from "../components/StatusBadge";
import { ExportButton } from "../components/ExportButton";
import { PlatformLabel } from "../components/PlatformLabel";

function formatOhaiTime(ohaiTime?: number): string {
  if (!ohaiTime) return "—";
  try {
    return new Date(ohaiTime * 1000).toLocaleString();
  } catch {
    return "—";
  }
}

const READINESS_OPTIONS: { value: string; label: string }[] = [
  { value: "ready", label: "✓ Ready" },
  { value: "needs_review", label: "◐ Needs Review" },
  { value: "blocked", label: "✗ Blocked" },
  { value: "cookbooks_blocked", label: "📦 Cookbooks Blocked" },
  { value: "disk_blocked", label: "💾 Disk Blocked" },
  { value: "disk_unknown", label: "💾 Disk Unknown" },
];

const DEPLOYMENT_STATE_OPTIONS: { value: string; label: string }[] = [
  { value: "Current only", label: "Current only" },
  { value: "Staged", label: "Staged" },
  { value: "Activated", label: "Activated" },
];

const CONVERGE_STATUS_OPTIONS: { value: string; label: string }[] = [
  { value: "success", label: "Success" },
  { value: "failed", label: "Failed" },
  { value: "pending", label: "Pending" },
];

// Drill-down params NodesPage reads into its own local state from the URL. After
// consuming them on mount we strip ONLY these, leaving the global filter params
// (target_chef_version, stale_tiers — owned by GlobalFilterContext, which shares
// the same useSearchParams) intact. A blanket setSearchParams({}) would wipe those
// and desync the global staleness/target state from the URL.
export const NODE_DRILLDOWN_PARAMS = [
  "readiness",
  "target_version",
  "chef_version",
  "platform",
  "environment",
  "role",
  "policy_name",
  "policy_group",
  "migration_state",
  "target_converge_status",
] as const;

/** Returns a copy of params with the NodesPage drill-down params removed. */
export function stripNodeDrilldownParams(prev: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams(prev);
  NODE_DRILLDOWN_PARAMS.forEach((p) => next.delete(p));
  return next;
}

// ---------------------------------------------------------------------------
// Nodes list page — paginated table from GET /api/v1/nodes with filter
// dropdowns for environment, platform, chef_version, role, policy name,
// policy group, stale status (global), and readiness (multi-select).
// Each row links to node detail. Stale nodes are colour-coded.
// ---------------------------------------------------------------------------

export function NodesPage() {
  const { selectedOrg } = useOrg();
  const { staleTiers } = useGlobalFilters();
  const [nodes, setNodes] = useState<NodeListItem[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // URL search params — read before filter state so initial values can be
  // seeded from query strings (e.g. links from the dashboard).
  const [searchParams, setSearchParams] = useSearchParams();

  // Filter state
  const [nodeName, setNodeName] = useState("");
  const [environments, setEnvironments] = useState<string[]>(
    searchParams.get("environment")?.split(",").filter(Boolean) ?? [],
  );
  const [platforms, setPlatforms] = useState<string[]>(
    searchParams.get("platform")?.split(",").filter(Boolean) ?? [],
  );
  const [chefVersion, setChefVersion] = useState(
    searchParams.get("chef_version") || "",
  );
  const [roles, setRoles] = useState<string[]>(
    searchParams.get("role")?.split(",").filter(Boolean) ?? [],
  );
  const [policyNames, setPolicyNames] = useState<string[]>(
    searchParams.get("policy_name")?.split(",").filter(Boolean) ?? [],
  );
  const [policyGroups, setPolicyGroups] = useState<string[]>(
    searchParams.get("policy_group")?.split(",").filter(Boolean) ?? [],
  );
  const [readinessFilter, setReadinessFilter] = useState<string[]>(
    searchParams.get("readiness")?.split(",").filter(Boolean) ?? [],
  );
  const [cookstyleFilter, setCookstyleFilter] = useState<string[]>([]);
  const [kitchenFilter, setKitchenFilter] = useState<string[]>([]);
  const [deploymentStateFilter, setDeploymentStateFilter] = useState<string[]>(
    searchParams.get("migration_state")?.split(",").filter(Boolean) ?? [],
  );
  const [convergeStatusFilter, setConvergeStatusFilter] = useState<string[]>(
    searchParams.get("target_converge_status")?.split(",").filter(Boolean) ?? [],
  );
  const [targetVersionFilter, setTargetVersionFilter] = useState<string[]>(
    searchParams.get("target_version")?.split(",").filter(Boolean) ?? [],
  );
  const [page, setPage] = useState(1);
  const perPage = DEFAULT_PAGE_SIZE;

  // Sort state — default to node_name ascending (backend default).
  const { sortField, sortOrder, handleSort } = useSort({
    defaultField: "node_name",
    defaultOrder: "asc",
    descendingFields: [
      "chef_version",
      "platform",
      "chef_environment",
      "ohai_time",
    ],
  });

  // Target Chef version for readiness filter and exports (from global context)
  const { selectedVersion: selectedTargetVersion } = useTargetChefVersion();

  // Filter option values loaded from the backend
  const [policyNameOptions, setPolicyNameOptions] = useState<string[]>([]);
  const [policyGroupOptions, setPolicyGroupOptions] = useState<string[]>([]);
  const [environmentOptions, setEnvironmentOptions] = useState<string[]>([]);
  const [platformOptions, setPlatformOptions] = useState<
    { value: string; label: string }[]
  >([]);

  // After the drill-down params have been read into local state, strip ONLY those
  // page-owned params from the URL so it stays clean. Crucially, preserve the
  // global filter params (target_chef_version, stale_tiers) — GlobalFilterContext
  // shares this same useSearchParams, and a blanket setSearchParams({}) would wipe
  // them, desyncing the global staleness/target state from the URL (the cause of
  // the "only fresh after clear / refresh needed / toggle no effect" bug).
  useEffect(() => {
    if (NODE_DRILLDOWN_PARAMS.some((p) => searchParams.has(p))) {
      setSearchParams((prev) => stripNodeDrilldownParams(prev), {
        replace: true,
      });
    }
  }, []); // run once on mount

  // Load filter option values whenever the selected org changes.
  useEffect(() => {
    const org = selectedOrg || undefined;

    fetchFilterPolicyNames(org)
      .then((res) => setPolicyNameOptions(res.data ?? []))
      .catch(() => setPolicyNameOptions([]));

    fetchFilterPolicyGroups(org)
      .then((res) => setPolicyGroupOptions(res.data ?? []))
      .catch(() => setPolicyGroupOptions([]));

    fetchFilterEnvironments(org)
      .then((res) => setEnvironmentOptions(res.data ?? []))
      .catch(() => setEnvironmentOptions([]));

    fetchFilterPlatforms(org)
      .then((res) => {
        const entries = (res.data ?? []).map((p) => ({
          value: p.value,
          label: p.display_name || p.value,
          group: p.group_display_name ?? "",
        }));
        // Sort by group then label for logical clustering.
        entries.sort((a, b) =>
          a.group !== b.group
            ? a.group.localeCompare(b.group)
            : a.label.localeCompare(b.label),
        );
        setPlatformOptions(entries);
      })
      .catch(() => setPlatformOptions([]));
  }, [selectedOrg]);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: NodeFilterQuery = {
      page,
      per_page: perPage,
    };
    if (selectedOrg) filters.organisation = selectedOrg;
    if (nodeName) filters.node_name = nodeName;
    if (environments.length > 0) filters.environment = environments.join(",");
    if (platforms.length > 0) filters.platform = platforms.join(",");
    if (chefVersion) filters.chef_version = chefVersion;
    if (roles.length > 0) filters.role = roles.join(",");
    if (policyNames.length > 0) filters.policy_name = policyNames.join(",");
    if (policyGroups.length > 0) filters.policy_group = policyGroups.join(",");
    if (staleTiers.length > 0) filters.stale = staleTiers.join(",");
    if (sortField) filters.sort = sortField;
    if (sortOrder) filters.order = sortOrder;
    if (selectedTargetVersion)
      filters.target_chef_version = selectedTargetVersion;
    if (readinessFilter.length > 0)
      filters.readiness_filter = readinessFilter.join(",");
    if (cookstyleFilter.length > 0)
      filters.cookstyle_status = cookstyleFilter.join(",");
    if (kitchenFilter.length > 0)
      filters.kitchen_status = kitchenFilter.join(",");
    if (deploymentStateFilter.length > 0)
      filters.migration_state = deploymentStateFilter.join(",");
    if (convergeStatusFilter.length > 0)
      filters.target_converge_status = convergeStatusFilter.join(",");
    if (targetVersionFilter.length > 0)
      filters.target_version = targetVersionFilter.join(",");

    fetchNodes(filters)
      .then((res) => {
        setNodes(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [
    selectedOrg,
    nodeName,
    environments,
    platforms,
    chefVersion,
    roles,
    policyNames,
    policyGroups,
    staleTiers,
    readinessFilter,
    cookstyleFilter,
    kitchenFilter,
    deploymentStateFilter,
    convergeStatusFilter,
    targetVersionFilter,
    selectedTargetVersion,
    page,
    sortField,
    sortOrder,
  ]);

  useEffect(() => {
    load();
  }, [load]);

  // Reset to page 1 when filters change.
  useEffect(() => {
    setPage(1);
  }, [
    selectedOrg,
    nodeName,
    environments,
    platforms,
    chefVersion,
    roles,
    policyNames,
    policyGroups,
    staleTiers,
    readinessFilter,
    cookstyleFilter,
    kitchenFilter,
    deploymentStateFilter,
    convergeStatusFilter,
    selectedTargetVersion,
    sortField,
    sortOrder,
  ]);

  // Count active filters for the clear button.
  const activeFilterCount =
    (nodeName ? 1 : 0) +
    (environments.length > 0 ? 1 : 0) +
    (platforms.length > 0 ? 1 : 0) +
    (chefVersion ? 1 : 0) +
    (roles.length > 0 ? 1 : 0) +
    (policyNames.length > 0 ? 1 : 0) +
    (policyGroups.length > 0 ? 1 : 0) +
    (readinessFilter.length > 0 ? 1 : 0) +
    (cookstyleFilter.length > 0 ? 1 : 0) +
    (kitchenFilter.length > 0 ? 1 : 0) +
    (deploymentStateFilter.length > 0 ? 1 : 0) +
    (convergeStatusFilter.length > 0 ? 1 : 0) +
    (targetVersionFilter.length > 0 ? 1 : 0);

  const clearFilters = () => {
    setNodeName("");
    setEnvironments([]);
    setPlatforms([]);
    setChefVersion("");
    setRoles([]);
    setPolicyNames([]);
    setPolicyGroups([]);
    setReadinessFilter([]);
    setCookstyleFilter([]);
    setKitchenFilter([]);
    setDeploymentStateFilter([]);
    setConvergeStatusFilter([]);
    setTargetVersionFilter([]);
  };

  // Readiness filtering is now handled server-side via readiness_filter and
  // target_chef_version query params. No client-side post-filtering needed.
  const displayNodes = nodes;

  // Build the current filter set for export buttons.
  const exportFilters: ExportFilters = {};
  if (selectedOrg) exportFilters.organisation = selectedOrg;
  if (nodeName) exportFilters.node_name = nodeName;
  if (environments.length > 0)
    exportFilters.environment = environments.join(",");
  if (platforms.length > 0) exportFilters.platform = platforms.join(",");
  if (chefVersion) exportFilters.chef_version = chefVersion;
  if (roles.length > 0) exportFilters.role = roles.join(",");
  if (policyNames.length > 0) exportFilters.policy_name = policyNames.join(",");
  if (policyGroups.length > 0)
    exportFilters.policy_group = policyGroups.join(",");
  if (staleTiers.length > 0) exportFilters.stale = staleTiers.join(",");

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-xl font-bold text-gray-800">Nodes</h2>
        <div className="flex items-center gap-3">
          <ExportButton
            exportType="ready_nodes"
            targetChefVersion={selectedTargetVersion || undefined}
            filters={exportFilters}
            label="Export Ready"
            formats={["csv", "json", "chef_search_query"]}
          />
          <ExportButton
            exportType="blocked_nodes"
            targetChefVersion={selectedTargetVersion || undefined}
            filters={exportFilters}
            label="Export Blocked"
          />
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <FilterInput
          label="Node Name"
          value={nodeName}
          onChange={setNodeName}
          placeholder="Filter by name"
          debounceMs={400}
        />
        <FilterMultiCheckbox
          label="Environment"
          selected={environments}
          onChange={setEnvironments}
          options={environmentOptions.map((o) => ({ value: o, label: o }))}
        />
        <FilterMultiCheckbox
          label="Platform"
          selected={platforms}
          onChange={setPlatforms}
          options={platformOptions}
        />
        <FilterInput
          label="Chef Version"
          value={chefVersion}
          onChange={setChefVersion}
          placeholder="e.g. 17.10.0"
          debounceMs={400}
        />
        <FilterTypeAhead
          label="Role"
          endpoint="/api/v1/filters/roles"
          selected={roles}
          onChange={setRoles}
        />
        <FilterMultiCheckbox
          label="Policy Name"
          selected={policyNames}
          onChange={setPolicyNames}
          options={policyNameOptions.map((o) => ({ value: o, label: o }))}
        />
        <FilterMultiCheckbox
          label="Policy Group"
          selected={policyGroups}
          onChange={setPolicyGroups}
          options={policyGroupOptions.map((o) => ({ value: o, label: o }))}
        />
        <FilterMultiCheckbox
          label="Readiness"
          selected={readinessFilter}
          onChange={setReadinessFilter}
          options={READINESS_OPTIONS}
        />
        <FilterMultiCheckbox
          label="CookStyle"
          selected={cookstyleFilter}
          onChange={setCookstyleFilter}
          options={[
            { value: "passed", label: "Passed" },
            { value: "failed", label: "Failed" },
            { value: "unknown", label: "Unknown" },
          ]}
        />
        <FilterMultiCheckbox
          label="Test Kitchen"
          selected={kitchenFilter}
          onChange={setKitchenFilter}
          options={[
            { value: "passed", label: "Passed" },
            { value: "failed", label: "Failed" },
            { value: "partial", label: "Partial" },
            { value: "unknown", label: "Unknown" },
          ]}
        />
        <FilterMultiCheckbox
          label="Deployment"
          selected={deploymentStateFilter}
          onChange={setDeploymentStateFilter}
          options={DEPLOYMENT_STATE_OPTIONS}
        />
        <FilterMultiCheckbox
          label="Converge"
          selected={convergeStatusFilter}
          onChange={setConvergeStatusFilter}
          options={CONVERGE_STATUS_OPTIONS}
        />
        {activeFilterCount > 0 && (
          <button
            onClick={clearFilters}
            className="mb-0.5 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-900"
            title="Clear all filters"
          >
            Clear ({activeFilterCount})
          </button>
        )}
      </div>

      {/* Table */}
      {loading && <LoadingSpinner message="Loading nodes…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {displayNodes.length === 0 ? (
            <EmptyState
              title="No nodes found"
              description="Adjust filters or wait for data collection."
            />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <SortableColumnHeader
                      label="Node Name"
                      field="node_name"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <th>Organisation</th>
                    <SortableColumnHeader
                      label="Environment"
                      field="chef_environment"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Chef Version"
                      field="chef_version"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <SortableColumnHeader
                      label="Platform"
                      field="platform"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <th>Status</th>
                    <th>Disk</th>
                    <th title="CookStyle — static analysis for Chef cookbook compatibility">CookStyle</th>
                    <th title="Test Kitchen — integration test results from matching Git repository">TK</th>
                    <th title="Deployment state — target version installation status">Deploy</th>
                    <th title="Speculative converge — nightly test run with staged version">Converge</th>
                    <SortableColumnHeader
                      label="Ohai Time"
                      field="ohai_time"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                  </tr>
                </thead>
                <tbody>
                  {displayNodes.map((node) => {
                    const readinessEntry = selectedTargetVersion
                      ? node.readiness?.find(
                          (r) =>
                            r.target_chef_version === selectedTargetVersion,
                        )
                      : node.readiness?.[0];
                    const csStatus = readinessEntry?.cookstyle_status ?? "unknown";
                    const csMapped = csStatus === "passed" ? "compatible" : csStatus === "failed" ? "incompatible" : "untested";
                    return (
                    <tr
                      key={node.id}
                      className={
                        node.ready_to_activate
                          ? "bg-green-50/60"
                          : node.is_stale
                            ? "bg-purple-50/50"
                            : ""
                      }
                    >
                      <td>
                        <Link
                          to={`/nodes/${encodeURIComponent(node.organisation_name || node.organisation_id)}/${encodeURIComponent(node.node_name)}`}
                          className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
                        >
                          {node.node_name}
                        </Link>
                      </td>
                      <td className="text-xs text-gray-500">
                        {node.organisation_name || node.organisation_id}
                      </td>
                      <td>{node.chef_environment || "—"}</td>
                      <td>
                        <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">
                          {node.chef_version || "—"}
                        </code>
                      </td>
                      <td>
                        <PlatformLabel
                          platform={node.platform}
                          platformVersion={node.platform_version}
                          platformDisplayName={node.platform_display_name}
                        />
                      </td>
                      <td>
                        <StaleBadge
                          isStale={node.is_stale}
                          stalenesTier={node.staleness_tier}
                          ageHours={node.ohai_time_age_hours}
                          size="sm"
                        />
                      </td>
                      <td>
                        {/* Version-invariant node-level disk verdict (from the
                            snapshot), correct even with no target/readiness rows. */}
                        <span title={node.disk_detail ?? "Disk: unknown"}>
                          <DiskBadge status={node.disk_status ?? "unknown"} size="sm" />
                        </span>
                      </td>
                      <td>
                        <span title={readinessEntry?.cookstyle_detail ?? "CookStyle: unknown"}>
                          <CookStyleBadge status={csMapped} size="sm" />
                        </span>
                      </td>
                      <td>
                        <span title={readinessEntry?.kitchen_detail ?? "Test Kitchen: unknown"}>
                          <TKBadge status={readinessEntry?.kitchen_status ?? "unknown"} size="sm" />
                        </span>
                      </td>
                      <td>
                        <DeploymentStateBadge state={node.migration_state} size="sm" />
                      </td>
                      <td>
                        <ConvergeBadge status={node.target_converge_status} size="sm" />
                      </td>
                      <td className="text-xs text-gray-400">
                        {formatOhaiTime(node.ohai_time)}
                      </td>
                    </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}

          {pagination && (
            <Pagination pagination={pagination} onPageChange={setPage} />
          )}
        </>
      )}
    </div>
  );
}
