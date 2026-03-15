import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { fetchNodeDetail } from "../api";
import type { NodeDetailResponse, BlockingCookbook, CookbookSourceVerdict } from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { StaleBadge, StatusBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Node detail page — shows full information for a single node including
// readiness status per target Chef version with per-source verdicts.
// ---------------------------------------------------------------------------

// Human-readable labels for verdict sources.
const SOURCE_LABELS: Record<string, string> = {
  server_cookstyle: "Server CookStyle",
  git_cookstyle: "Git Repo CookStyle",
  git_test_kitchen: "Git Repo Test Kitchen",
  cookstyle: "CookStyle",
  test_kitchen: "Test Kitchen",
  none: "None",
};

// CSS classes for verdict status icons.
const STATUS_ICON: Record<string, { icon: string; color: string }> = {
  compatible: { icon: "✓", color: "text-green-600" },
  compatible_cookstyle_only: { icon: "✓", color: "text-green-500" },
  incompatible: { icon: "✗", color: "text-red-600" },
  untested: { icon: "?", color: "text-gray-400" },
};

function sourceLabel(source: string): string {
  return SOURCE_LABELS[source] || source;
}

function statusIcon(status: string): { icon: string; color: string } {
  return STATUS_ICON[status] || { icon: "•", color: "text-gray-400" };
}

function statusLabel(status: string): string {
  switch (status) {
    case "compatible":
      return "Compatible";
    case "compatible_cookstyle_only":
      return "Compatible (CookStyle only)";
    case "incompatible":
      return "Incompatible";
    case "untested":
      return "Untested";
    default:
      return status;
  }
}

// ---------------------------------------------------------------------------
// Verdict panel for a single source
// ---------------------------------------------------------------------------

function VerdictRow({ verdict }: { verdict: CookbookSourceVerdict }) {
  const si = statusIcon(verdict.status);
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className={`font-bold ${si.color}`}>{si.icon}</span>
      <span className="font-medium text-gray-700">{sourceLabel(verdict.source)}</span>
      <span className="text-gray-400">—</span>
      <span className={si.color}>{statusLabel(verdict.status)}</span>
      {verdict.version && (
        <span className="text-gray-400">
          v{verdict.version}
          {verdict.commit_sha && (
            <span className="ml-1 font-mono text-[10px]">({verdict.commit_sha.slice(0, 7)})</span>
          )}
        </span>
      )}
      {verdict.complexity_label && (
        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-500">
          complexity: {verdict.complexity_label}
          {verdict.complexity_score ? ` (${verdict.complexity_score})` : ""}
        </span>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Single blocking cookbook card with expandable verdicts
// ---------------------------------------------------------------------------

function BlockingCookbookCard({ bc }: { bc: BlockingCookbook }) {
  const [expanded, setExpanded] = useState(false);
  const hasVerdicts = bc.verdicts && bc.verdicts.length > 0;

  // Determine if a compatible version exists in git while server is incompatible.
  const serverIncompat = bc.verdicts?.find(
    (v) => v.source === "server_cookstyle" && v.status === "incompatible"
  );
  const gitCompat = bc.verdicts?.find(
    (v) =>
      (v.source === "git_cookstyle" || v.source === "git_test_kitchen") &&
      (v.status === "compatible" || v.status === "compatible_cookstyle_only")
  );
  const showActionHint = serverIncompat && gitCompat;

  return (
    <div className="rounded-lg border border-red-100 bg-red-50/30 p-3">
      {/* Header */}
      <div className="flex items-center gap-2">
        <Link
          to={`/cookbooks/${encodeURIComponent(bc.name)}`}
          className="font-medium text-red-700 hover:text-red-900 hover:underline"
        >
          {bc.name}
        </Link>
        <span className="text-xs text-gray-500">@{bc.version}</span>
        <span
          className={`rounded px-1.5 py-0.5 text-xs font-medium ${bc.reason === "incompatible"
              ? "bg-red-100 text-red-700"
              : "bg-gray-100 text-gray-600"
            }`}
        >
          {bc.reason}
        </span>
        {bc.complexity_label && (
          <span className="rounded bg-amber-50 px-1.5 py-0.5 text-xs text-amber-700">
            {bc.complexity_label}
            {bc.complexity_score ? ` (${bc.complexity_score})` : ""}
          </span>
        )}
        {hasVerdicts && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="ml-auto text-xs text-blue-600 hover:text-blue-800 hover:underline"
          >
            {expanded ? "Hide verdicts" : `Show verdicts (${bc.verdicts!.length})`}
          </button>
        )}
      </div>

      {/* Action hint */}
      {showActionHint && (
        <div className="mt-2 flex items-start gap-1.5 rounded border border-blue-100 bg-blue-50 px-2 py-1.5 text-xs text-blue-700">
          <span className="mt-0.5 shrink-0">💡</span>
          <span>
            A compatible version exists in{" "}
            <Link
              to={`/git-repos/${encodeURIComponent(bc.name)}`}
              className="font-medium underline hover:text-blue-900"
            >
              git
            </Link>{" "}
            — upload to Chef Server to resolve.
          </span>
        </div>
      )}

      {/* Expandable verdicts */}
      {expanded && hasVerdicts && (
        <div className="mt-2 space-y-1 border-t border-red-100 pt-2">
          {bc.verdicts!.map((v, i) => (
            <div key={i} className="flex items-center gap-2">
              <VerdictRow verdict={v} />
              {/* Link to source detail page */}
              {(v.source === "git_cookstyle" || v.source === "git_test_kitchen") && (
                <Link
                  to={`/git-repos/${encodeURIComponent(bc.name)}`}
                  className="text-[10px] text-blue-500 hover:text-blue-700 hover:underline"
                >
                  view repo →
                </Link>
              )}
              {v.source === "server_cookstyle" && (
                <Link
                  to={`/cookbooks/${encodeURIComponent(bc.name)}`}
                  className="text-[10px] text-blue-500 hover:text-blue-700 hover:underline"
                >
                  view cookbook →
                </Link>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Readiness section
// ---------------------------------------------------------------------------

function ReadinessSection({ data }: { data: NodeDetailResponse }) {
  if (!data.readiness || data.readiness.length === 0) return null;

  return (
    <div className="card">
      <h3 className="card-header">Upgrade Readiness</h3>
      <div className="space-y-4">
        {data.readiness.map((r) => {
          // Derive blocking reasons from the structured readiness record.
          const reasons: string[] = [];
          if (!r.all_cookbooks_compatible) {
            const count = r.blocking_cookbooks?.length ?? 0;
            reasons.push(
              count > 0
                ? `${count} blocking cookbook${count !== 1 ? "s" : ""}`
                : "Cookbooks not all compatible"
            );
          }
          if (r.sufficient_disk_space === false) {
            const avail = r.available_disk_mb != null ? `${r.available_disk_mb} MB` : "unknown";
            const req = r.required_disk_mb != null ? `${r.required_disk_mb} MB` : "unknown";
            reasons.push(`Insufficient disk space (available: ${avail}, required: ${req})`);
          } else if (r.sufficient_disk_space === null) {
            reasons.push("Disk space unknown");
          }
          if (r.stale_data) {
            reasons.push("Stale node data — disk space treated as unknown");
          }

          return (
            <div key={r.id} className="flex items-start gap-3 rounded-lg border border-gray-100 p-3">
              <StatusBadge variant={r.is_ready ? "ready" : "blocked"} />
              <div className="flex-1 space-y-2">
                <div className="text-sm font-medium text-gray-700">
                  Target: {r.target_chef_version}
                </div>

                {/* Blocking reasons */}
                {reasons.length > 0 && (
                  <ul className="space-y-0.5 text-xs text-gray-500">
                    {reasons.map((reason, i) => (
                      <li key={i} className="flex items-center gap-1">
                        <span className="text-red-400">•</span> {reason}
                      </li>
                    ))}
                  </ul>
                )}

                {/* Blocking cookbooks with per-source verdicts */}
                {r.blocking_cookbooks && r.blocking_cookbooks.length > 0 && (
                  <div className="space-y-2">
                    {r.blocking_cookbooks.map((bc) => (
                      <BlockingCookbookCard key={`${bc.name}@${bc.version}`} bc={bc} />
                    ))}
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
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
        <InfoCard label="Ohai Time" value={node.ohai_time ? new Date(Number(node.ohai_time) * 1000).toLocaleString() : "—"} />
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
      <ReadinessSection data={data} />
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
