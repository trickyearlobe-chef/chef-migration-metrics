import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import { fetchNodes, type NodeFilterQuery } from "../api";
import type { NodeListItem, Pagination as PaginationType } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StaleBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Nodes list page — paginated table from GET /api/v1/nodes with filter
// dropdowns for environment, platform, chef_version, and stale status.
// Each row links to node detail. Stale nodes are colour-coded.
// ---------------------------------------------------------------------------

export function NodesPage() {
  const { selectedOrg } = useOrg();
  const [nodes, setNodes] = useState<NodeListItem[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filter state
  const [environment, setEnvironment] = useState("");
  const [platform, setPlatform] = useState("");
  const [chefVersion, setChefVersion] = useState("");
  const [stale, setStale] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 50;

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: NodeFilterQuery = {
      page,
      per_page: perPage,
    };
    if (selectedOrg) filters.organisation = selectedOrg;
    if (environment) filters.environment = environment;
    if (platform) filters.platform = platform;
    if (chefVersion) filters.chef_version = chefVersion;
    if (stale) filters.stale = stale;

    fetchNodes(filters)
      .then((res) => {
        setNodes(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedOrg, environment, platform, chefVersion, stale, page]);

  useEffect(() => { load(); }, [load]);

  // Reset to page 1 when filters change.
  useEffect(() => { setPage(1); }, [selectedOrg, environment, platform, chefVersion, stale]);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-bold text-gray-800">Nodes</h2>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <FilterInput label="Environment" value={environment} onChange={setEnvironment} placeholder="e.g. production" />
        <FilterInput label="Platform" value={platform} onChange={setPlatform} placeholder="e.g. ubuntu" />
        <FilterInput label="Chef Version" value={chefVersion} onChange={setChefVersion} placeholder="e.g. 17.10.0" />
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
      </div>

      {/* Table */}
      {loading && <LoadingSpinner message="Loading nodes\u2026" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {nodes.length === 0 ? (
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
                    <th>Collected</th>
                  </tr>
                </thead>
                <tbody>
                  {nodes.map((node) => (
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
                      <td>{node.chef_environment || "\u2014"}</td>
                      <td>
                        <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">
                          {node.chef_version || "\u2014"}
                        </code>
                      </td>
                      <td>
                        {node.platform
                          ? `${node.platform} ${node.platform_version || ""}`
                          : "\u2014"}
                      </td>
                      <td>
                        <StaleBadge isStale={node.is_stale} size="sm" />
                      </td>
                      <td className="text-xs text-gray-400">
                        {new Date(node.collected_at).toLocaleString()}
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
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
}) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-gray-500">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="block w-32 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>{opt.label}</option>
        ))}
      </select>
    </div>
  );
}
