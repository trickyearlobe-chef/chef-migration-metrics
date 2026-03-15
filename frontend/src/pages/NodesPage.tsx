import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import {
  fetchNodes,
  fetchFilterRoles,
  fetchFilterPolicyNames,
  fetchFilterPolicyGroups,
  fetchFilterEnvironments,
  fetchFilterPlatforms,
  fetchFilterTargetChefVersions,
  type NodeFilterQuery,
} from "../api";
import type { NodeListItem, Pagination as PaginationType, ExportFilters } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StaleBadge } from "../components/StatusBadge";
import { ExportButton } from "../components/ExportButton";

// ---------------------------------------------------------------------------
// Readiness filter values
// ---------------------------------------------------------------------------
type ReadinessFilter = "" | "ready" | "blocked" | "cookbooks_blocked" | "disk_blocked" | "disk_unknown";

function matchesReadinessFilter(
  node: NodeListItem,
  targetVersion: string,
  filter: ReadinessFilter,
): boolean {
  if (!filter) return true; // "All" — no filtering

  // Find the readiness entry for the selected target version.
  const entry = (node.readiness ?? []).find(
    (r) => r.target_chef_version === targetVersion,
  );

  // If there's no readiness data for this target version, only show in "unknown".
  if (!entry) {
    return filter === "blocked"; // nodes with no data are implicitly not ready
  }

  switch (filter) {
    case "ready":
      return entry.is_ready;
    case "blocked":
      return !entry.is_ready;
    case "cookbooks_blocked":
      return !entry.all_cookbooks_compatible;
    case "disk_blocked":
      return entry.sufficient_disk_space === false;
    case "disk_unknown":
      return entry.sufficient_disk_space === null || entry.sufficient_disk_space === undefined;
    default:
      return true;
  }
}

function formatOhaiTime(ohaiTime?: number): string {
  if (!ohaiTime) return "—";
  try {
    return new Date(ohaiTime * 1000).toLocaleString();
  } catch {
    return "—";
  }
}

// ---------------------------------------------------------------------------
// Nodes list page — paginated table from GET /api/v1/nodes with filter
// dropdowns for environment, platform, chef_version, role, policy name,
// policy group, stale status, and readiness (for a selected target version).
// Each row links to node detail. Stale nodes are colour-coded.
// ---------------------------------------------------------------------------

export function NodesPage() {
  const { selectedOrg } = useOrg();
  const [nodes, setNodes] = useState<NodeListItem[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filter state
  const [nodeName, setNodeName] = useState("");
  const [environment, setEnvironment] = useState("");
  const [platform, setPlatform] = useState("");
  const [chefVersion, setChefVersion] = useState("");
  const [role, setRole] = useState("");
  const [policyName, setPolicyName] = useState("");
  const [policyGroup, setPolicyGroup] = useState("");
  const [stale, setStale] = useState("");
  const [readinessFilter, setReadinessFilter] = useState<ReadinessFilter>("");
  const [page, setPage] = useState(1);
  const perPage = 50;

  // Target Chef version for readiness filter and exports (loaded from backend config)
  const [targetVersions, setTargetVersions] = useState<string[]>([]);
  const [selectedTargetVersion, setSelectedTargetVersion] = useState<string>("");

  // Filter option values loaded from the backend
  const [roleOptions, setRoleOptions] = useState<string[]>([]);
  const [policyNameOptions, setPolicyNameOptions] = useState<string[]>([]);
  const [policyGroupOptions, setPolicyGroupOptions] = useState<string[]>([]);
  const [environmentOptions, setEnvironmentOptions] = useState<string[]>([]);
  const [platformOptions, setPlatformOptions] = useState<string[]>([]);

  // Load target Chef versions once on mount.
  useEffect(() => {
    fetchFilterTargetChefVersions()
      .then((res) => {
        const versions = res.data ?? [];
        setTargetVersions(versions);
        if (versions.length > 0 && !selectedTargetVersion) {
          setSelectedTargetVersion(versions[0]);
        }
      })
      .catch(() => setTargetVersions([]));
  }, []); // intentionally run only on mount

  // Load filter option values whenever the selected org changes.
  useEffect(() => {
    const org = selectedOrg || undefined;

    fetchFilterRoles(org)
      .then((res) => setRoleOptions(res.data ?? []))
      .catch(() => setRoleOptions([]));

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
        // Flatten platform objects to just the platform name strings.
        const names = (res.data ?? []).map((p) => p.platform);
        setPlatformOptions(names);
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
    if (environment) filters.environment = environment;
    if (platform) filters.platform = platform;
    if (chefVersion) filters.chef_version = chefVersion;
    if (role) filters.role = role;
    if (policyName) filters.policy_name = policyName;
    if (policyGroup) filters.policy_group = policyGroup;
    if (stale) filters.stale = stale;

    fetchNodes(filters)
      .then((res) => {
        setNodes(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedOrg, nodeName, environment, platform, chefVersion, role, policyName, policyGroup, stale, page]);

  useEffect(() => { load(); }, [load]);

  // Reset to page 1 when filters change.
  useEffect(() => { setPage(1); }, [selectedOrg, nodeName, environment, platform, chefVersion, role, policyName, policyGroup, stale]);

  // Count active filters for the clear button.
  // Readiness filter is counted only when set (target version selector is not counted as a filter).
  const activeFilterCount = [nodeName, environment, platform, chefVersion, role, policyName, policyGroup, stale, readinessFilter].filter(Boolean).length;

  const clearFilters = () => {
    setNodeName("");
    setEnvironment("");
    setPlatform("");
    setChefVersion("");
    setRole("");
    setPolicyName("");
    setPolicyGroup("");
    setStale("");
    setReadinessFilter("");
  };

  // Apply client-side readiness filter. The backend doesn't support readiness
  // filtering directly, so we filter the already-fetched page of nodes.
  // This means the displayed count may be less than the page size when a
  // readiness filter is active, which is an acceptable trade-off for now.
  const displayNodes = readinessFilter && selectedTargetVersion
    ? nodes.filter((n) => matchesReadinessFilter(n, selectedTargetVersion, readinessFilter))
    : nodes;

  // Build the current filter set for export buttons.
  const exportFilters: ExportFilters = {};
  if (selectedOrg) exportFilters.organisation = selectedOrg;
  if (nodeName) exportFilters.node_name = nodeName;
  if (environment) exportFilters.environment = environment;
  if (platform) exportFilters.platform = platform;
  if (chefVersion) exportFilters.chef_version = chefVersion;
  if (role) exportFilters.role = role;
  if (policyName) exportFilters.policy_name = policyName;
  if (policyGroup) exportFilters.policy_group = policyGroup;
  if (stale) exportFilters.stale = stale;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-xl font-bold text-gray-800">Nodes</h2>
        <div className="flex items-center gap-3">
          {/* Target version selector for readiness filter + exports */}
          {targetVersions.length > 0 && (
            <div className="flex items-center gap-2">
              <label className="text-xs font-medium text-gray-500">Target Version</label>
              <select
                value={selectedTargetVersion}
                onChange={(e) => setSelectedTargetVersion(e.target.value)}
                className="block w-28 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                {targetVersions.map((v) => (
                  <option key={v} value={v}>{v}</option>
                ))}
              </select>
            </div>
          )}
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
        <FilterInput label="Node Name" value={nodeName} onChange={setNodeName} placeholder="Filter by name" />
        <FilterCombobox
          label="Environment"
          value={environment}
          onChange={setEnvironment}
          options={environmentOptions}
          placeholder="All environments"
        />
        <FilterCombobox
          label="Platform"
          value={platform}
          onChange={setPlatform}
          options={platformOptions}
          placeholder="All platforms"
        />
        <FilterInput label="Chef Version" value={chefVersion} onChange={setChefVersion} placeholder="e.g. 17.10.0" />
        <FilterCombobox
          label="Role"
          value={role}
          onChange={setRole}
          options={roleOptions}
          placeholder="All roles"
        />
        <FilterCombobox
          label="Policy Name"
          value={policyName}
          onChange={setPolicyName}
          options={policyNameOptions}
          placeholder="All policies"
        />
        <FilterCombobox
          label="Policy Group"
          value={policyGroup}
          onChange={setPolicyGroup}
          options={policyGroupOptions}
          placeholder="All groups"
        />
        <FilterSelect
          label="Stale Status"
          value={stale}
          onChange={setStale}
          options={[
            { value: "", label: "All" },
            { value: "true", label: "Stale" },
            { value: "false", label: "Fresh" },
          ]}
        />
        <FilterSelect
          label="Readiness"
          value={readinessFilter}
          onChange={(v) => setReadinessFilter(v as ReadinessFilter)}
          options={[
            { value: "", label: "All" },
            { value: "ready", label: "✓ Ready" },
            { value: "blocked", label: "✗ Blocked" },
            { value: "cookbooks_blocked", label: "📦 Cookbooks Blocked" },
            { value: "disk_blocked", label: "💾 Disk Blocked" },
            { value: "disk_unknown", label: "💾 Disk Unknown" },
          ]}
          wide
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
            <EmptyState title="No nodes found" description="Adjust filters or wait for data collection." />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th>Node Name</th>
                    <th>Organisation</th>
                    <th>Environment</th>
                    <th>Chef Version</th>
                    <th>Platform</th>
                    <th>Status</th>
                    <th>Ohai Time</th>
                  </tr>
                </thead>
                <tbody>
                  {displayNodes.map((node) => (
                    <tr
                      key={node.id}
                      className={node.is_stale ? "bg-purple-50/50" : ""}
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
                        {node.platform
                          ? `${node.platform} ${node.platform_version || ""}`
                          : "—"}
                      </td>
                      <td>
                        <StaleBadge isStale={node.is_stale} size="sm" />
                      </td>
                      <td className="text-xs text-gray-400">
                        {formatOhaiTime(node.ohai_time)}
                      </td>
                    </tr>
                  ))}
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

// ---------------------------------------------------------------------------
// Filter helpers
// ---------------------------------------------------------------------------

function FilterInput({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-gray-500">{label}</label>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="block w-40 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      />
    </div>
  );
}

function FilterSelect({
  label,
  value,
  onChange,
  options,
  wide,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
  wide?: boolean;
}) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-gray-500">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={`block rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 ${wide ? "w-48" : "w-32"}`}
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>{opt.label}</option>
        ))}
      </select>
    </div>
  );
}

/**
 * FilterCombobox — a select dropdown populated from backend filter options.
 * Shows a "placeholder" option as the first entry (value="") representing
 * "all / no filter". Falls back to a text input if no options are loaded.
 */
function FilterCombobox({
  label,
  value,
  onChange,
  options,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: string[];
  placeholder?: string;
}) {
  // If no backend options have loaded yet, render a plain text input so
  // the user can still type a filter value manually.
  if (options.length === 0) {
    return (
      <FilterInput
        label={label}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
      />
    );
  }

  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-gray-500">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="block w-40 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      >
        <option value="">{placeholder || `All ${label.toLowerCase()}s`}</option>
        {options.map((opt) => (
          <option key={opt} value={opt}>{opt}</option>
        ))}
      </select>
    </div>
  );
}
