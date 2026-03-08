import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { fetchNodeDetail } from "../api";
import type { NodeDetailResponse } from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { StaleBadge, StatusBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Node detail page — shows full information for a single node including
// readiness status per target Chef version.
// ---------------------------------------------------------------------------

export function NodeDetailPage() {
  const { org, name } = useParams<{ org: string; name: string }>();
  const [data, setData] = useState<NodeDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!org || !name) return;
    setLoading(true);
    setError(null);
    fetchNodeDetail(org, name)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [org, name]);

  useEffect(() => { load(); }, [load]);

  if (loading) return <LoadingSpinner message="Loading node detail…" />;
  if (error) return <ErrorAlert message={error} onRetry={load} />;
  if (!data) return null;

  const node = data.node;

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <Link to="/nodes" className="hover:text-blue-600 hover:underline">Nodes</Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">{node.node_name}</span>
      </nav>

      {/* Header */}
      <div className="flex items-center gap-3">
        <h2 className="text-xl font-bold text-gray-800">{node.node_name}</h2>
        <StaleBadge isStale={node.is_stale} />
      </div>

      {/* Info grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <InfoCard label="Organisation" value={data.organisation_name || node.organisation_id} />
        <InfoCard label="Environment" value={node.chef_environment || "—"} />
        <InfoCard label="Chef Version" value={node.chef_version || "—"} />
        <InfoCard label="Platform" value={`${node.platform || "—"} ${node.platform_version || ""}`} />
        <InfoCard label="Platform Family" value={node.platform_family || "—"} />
        <InfoCard label="Policy" value={node.policy_name ? `${node.policy_name} / ${node.policy_group}` : "—"} />
        <InfoCard label="Last Collected" value={new Date(node.collected_at).toLocaleString()} />
        <InfoCard label="Ohai Time" value={node.ohai_time ? new Date(node.ohai_time).toLocaleString() : "—"} />
      </div>

      {/* Run list */}
      {node.run_list && node.run_list.length > 0 && (
        <div className="card">
          <h3 className="card-header">Run List</h3>
          <div className="flex flex-wrap gap-2">
            {node.run_list.map((item, i) => (
              <code key={i} className="rounded bg-gray-100 px-2 py-1 text-xs text-gray-700">
                {item}
              </code>
            ))}
          </div>
        </div>
      )}

      {/* Roles */}
      {node.roles && node.roles.length > 0 && (
        <div className="card">
          <h3 className="card-header">Roles</h3>
          <div className="flex flex-wrap gap-2">
            {node.roles.map((role) => (
              <span key={role} className="badge badge-compatible">{role}</span>
            ))}
          </div>
        </div>
      )}

      {/* Cookbooks */}
      {node.cookbooks && Object.keys(node.cookbooks).length > 0 && (
        <div className="card">
          <h3 className="card-header">Cookbooks</h3>
          <div className="flex flex-wrap gap-2">
            {Object.entries(node.cookbooks).map(([name, info]) => (
              <Link
                key={name}
                to={`/cookbooks/${encodeURIComponent(name)}`}
                className="rounded bg-blue-50 px-2 py-1 text-xs text-blue-700 hover:bg-blue-100"
              >
                {name} {typeof info === "object" && info && "version" in info ? `@${(info as Record<string, string>).version}` : ""}
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* Readiness */}
      {data.readiness && data.readiness.length > 0 && (
        <div className="card">
          <h3 className="card-header">Upgrade Readiness</h3>
          <div className="space-y-3">
            {data.readiness.map((r) => (
              <div key={r.id} className="flex items-start gap-3 rounded-lg border border-gray-100 p-3">
                <StatusBadge variant={r.ready ? "ready" : "blocked"} />
                <div className="flex-1">
                  <div className="text-sm font-medium text-gray-700">
                    Target: {r.target_chef_version}
                  </div>
                  {r.blocking_reasons && r.blocking_reasons.length > 0 && (
                    <ul className="mt-1 space-y-0.5 text-xs text-gray-500">
                      {r.blocking_reasons.map((reason, i) => (
                        <li key={i} className="flex items-center gap-1">
                          <span className="text-red-400">•</span> {reason}
                        </li>
                      ))}
                    </ul>
                  )}
                  {r.blocking_cookbooks && r.blocking_cookbooks.length > 0 && (
                    <div className="mt-1 flex flex-wrap gap-1">
                      {r.blocking_cookbooks.map((cb) => (
                        <Link
                          key={cb}
                          to={`/cookbooks/${encodeURIComponent(cb)}`}
                          className="rounded bg-red-50 px-1.5 py-0.5 text-xs text-red-700 hover:bg-red-100"
                        >
                          {cb}
                        </Link>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="stat-card">
      <span className="stat-label">{label}</span>
      <span className="mt-1 text-sm font-medium text-gray-800">{value}</span>
    </div>
  );
}
