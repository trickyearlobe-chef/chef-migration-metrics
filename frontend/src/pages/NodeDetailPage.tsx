import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { fetchNodeDetail } from "../api";
import type { NodeDetailResponse, NodeReadiness, BlockingCookbook, CookbookSourceVerdict } from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { StaleBadge, StatusBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Node detail page — shows full information for a single node including
// readiness status per target Chef version with per-source verdicts,
// a full cookbook compatibility table, and disk space analysis.
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

const STATUS_ICON: Record<string, { icon: string; color: string; bg: string }> = {
  compatible: { icon: "✓", color: "text-green-600", bg: "bg-green-50" },
  compatible_cookstyle_only: { icon: "✓", color: "text-green-500", bg: "bg-green-50" },
  incompatible: { icon: "✗", color: "text-red-600", bg: "bg-red-50" },
  untested: { icon: "?", color: "text-gray-400", bg: "bg-gray-50" },
  unknown: { icon: "—", color: "text-gray-300", bg: "bg-gray-50" },
};

function sourceLabel(source: string): string {
  return SOURCE_LABELS[source] || source;
}

function statusIcon(status: string) {
  return STATUS_ICON[status] || STATUS_ICON.unknown;
}

function statusLabel(status: string): string {
  switch (status) {
    case "compatible": return "Compatible";
    case "compatible_cookstyle_only": return "Compatible (CookStyle only)";
    case "incompatible": return "Incompatible";
    case "untested": return "Untested";
    default: return status || "Unknown";
  }
}

// ---------------------------------------------------------------------------
// Verdict row for a single source
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
// Blocking cookbook card with expandable verdicts
// ---------------------------------------------------------------------------

function BlockingCookbookCard({ bc }: { bc: BlockingCookbook }) {
  const [expanded, setExpanded] = useState(false);
  const hasVerdicts = bc.verdicts && bc.verdicts.length > 0;

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
      <div className="flex flex-wrap items-center gap-2">
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
            </Link>
            {" "}— upload to Chef Server to resolve.
          </span>
        </div>
      )}

      {/* Expandable verdicts */}
      {expanded && hasVerdicts && (
        <div className="mt-2 space-y-1 border-t border-red-100 pt-2">
          {bc.verdicts!.map((v, i) => (
            <div key={i} className="flex items-center gap-2">
              <VerdictRow verdict={v} />
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
// Disk space analysis panel
// ---------------------------------------------------------------------------

function DiskSpacePanel({ r }: { r: NodeReadiness }) {
  const available = r.available_disk_mb;
  const required = r.required_disk_mb;
  const sufficient = r.sufficient_disk_space;
  const stale = r.stale_data;

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
          <span className="rounded bg-gray-200 px-1.5 py-0.5 text-xs font-medium text-gray-600">Unknown</span>
        </div>
        <p className="mt-1 text-xs text-gray-500">{reason}</p>
        {required != null && (
          <p className="mt-0.5 text-xs text-gray-400">
            Minimum required: {formatMB(required)}
          </p>
        )}
      </div>
    );
  }

  // Known state
  const pct = available != null && required != null && required > 0
    ? Math.min(100, Math.round((available / required) * 100))
    : null;

  const barColor = sufficient ? "bg-green-500" : "bg-red-500";
  const borderColor = sufficient ? "border-green-200" : "border-red-200";
  const bgColor = sufficient ? "bg-green-50" : "bg-red-50";
  const badgeBg = sufficient ? "bg-green-100 text-green-700" : "bg-red-100 text-red-700";

  return (
    <div className={`rounded-lg border ${borderColor} ${bgColor} p-3`}>
      <div className="flex items-center gap-2">
        <span className="text-lg">{sufficient ? "✅" : "⚠️"}</span>
        <span className="text-sm font-medium text-gray-700">Disk Space</span>
        <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${badgeBg}`}>
          {sufficient ? "Sufficient" : "Insufficient"}
        </span>
      </div>

      {/* Bar chart */}
      {available != null && required != null && pct != null && (
        <div className="mt-2">
          <div className="flex justify-between text-xs text-gray-500">
            <span>Available: <strong className="text-gray-700">{formatMB(available)}</strong></span>
            <span>Required: <strong className="text-gray-700">{formatMB(required)}</strong></span>
          </div>
          <div className="mt-1 h-3 w-full overflow-hidden rounded-full bg-gray-200">
            <div
              className={`h-full rounded-full transition-all ${barColor}`}
              style={{ width: `${Math.min(pct, 100)}%` }}
            />
          </div>
          <div className="mt-0.5 text-right text-[10px] text-gray-400">
            {pct >= 100
              ? `${formatMB(available - required)} headroom`
              : `${formatMB(required - available)} short`}
          </div>
        </div>
      )}

      {available != null && required == null && (
        <p className="mt-1 text-xs text-gray-500">
          Available: <strong>{formatMB(available)}</strong> (no minimum configured)
        </p>
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
// Cookbook compatibility table — shows EVERY cookbook on the node with status
// ---------------------------------------------------------------------------

interface CookbookRow {
  name: string;
  version: string;
  status: string;       // "compatible", "incompatible", "untested", "unknown"
  source: string;
  blocking?: BlockingCookbook;
}

function buildCookbookRows(
  nodeCookbooks: Record<string, unknown> | null,
  blockingCookbooks: BlockingCookbook[] | null,
): CookbookRow[] {
  if (!nodeCookbooks) return [];

  // Index blocking cookbooks by name for O(1) lookup.
  const blockingByName = new Map<string, BlockingCookbook>();
  if (blockingCookbooks) {
    for (const bc of blockingCookbooks) {
      blockingByName.set(bc.name, bc);
    }
  }

  const rows: CookbookRow[] = [];
  for (const [name, info] of Object.entries(nodeCookbooks)) {
    const version =
      typeof info === "object" && info && "version" in info
        ? String((info as Record<string, string>).version)
        : "";

    const bc = blockingByName.get(name);
    if (bc) {
      rows.push({
        name,
        version: bc.version || version,
        status: bc.reason,      // "incompatible" or "untested"
        source: bc.source,
        blocking: bc,
      });
    } else {
      // Not in the blocking list → compatible (or at least not blocking).
      rows.push({
        name,
        version,
        status: "compatible",
        source: "",
      });
    }
  }

  // Sort: incompatible first, then untested, then compatible. Within each group, alphabetical.
  const ORDER: Record<string, number> = { incompatible: 0, untested: 1, compatible: 2, compatible_cookstyle_only: 2 };
  rows.sort((a, b) => {
    const oa = ORDER[a.status] ?? 3;
    const ob = ORDER[b.status] ?? 3;
    if (oa !== ob) return oa - ob;
    return a.name.localeCompare(b.name);
  });

  return rows;
}

function CookbookCompatibilityTable({
  nodeCookbooks,
  r,
}: {
  nodeCookbooks: Record<string, unknown> | null;
  r: NodeReadiness;
}) {
  const [showAll, setShowAll] = useState(false);
  const rows = buildCookbookRows(nodeCookbooks, r.blocking_cookbooks);

  if (rows.length === 0) {
    return (
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-3">
        <div className="flex items-center gap-2">
          <span className="text-lg">📦</span>
          <span className="text-sm font-medium text-gray-600">Cookbook Compatibility</span>
          <span className="rounded bg-gray-200 px-1.5 py-0.5 text-xs font-medium text-gray-600">No cookbooks</span>
        </div>
        <p className="mt-1 text-xs text-gray-500">This node has no cookbooks in its run list.</p>
      </div>
    );
  }

  const incompatible = rows.filter((r) => r.status === "incompatible").length;
  const untested = rows.filter((r) => r.status === "untested").length;
  const compatible = rows.length - incompatible - untested;

  const blocking = rows.filter((r) => r.status === "incompatible" || r.status === "untested");
  const nonBlocking = rows.filter((r) => r.status !== "incompatible" && r.status !== "untested");
  const displayedNonBlocking = showAll ? nonBlocking : [];

  const allCompatible = incompatible === 0 && untested === 0;
  const borderColor = allCompatible ? "border-green-200" : "border-red-200";
  const bgColor = allCompatible ? "bg-green-50" : "bg-red-50/30";

  return (
    <div className={`rounded-lg border ${borderColor} ${bgColor} p-3`}>
      {/* Summary header */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-lg">📦</span>
        <span className="text-sm font-medium text-gray-700">Cookbook Compatibility</span>
        {allCompatible ? (
          <span className="rounded bg-green-100 px-1.5 py-0.5 text-xs font-medium text-green-700">
            All {rows.length} compatible
          </span>
        ) : (
          <>
            {incompatible > 0 && (
              <span className="rounded bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700">
                {incompatible} incompatible
              </span>
            )}
            {untested > 0 && (
              <span className="rounded bg-gray-200 px-1.5 py-0.5 text-xs font-medium text-gray-600">
                {untested} untested
              </span>
            )}
            {compatible > 0 && (
              <span className="rounded bg-green-100 px-1.5 py-0.5 text-xs font-medium text-green-700">
                {compatible} compatible
              </span>
            )}
          </>
        )}
      </div>

      {/* Compatibility bar */}
      {rows.length > 0 && (
        <div className="mt-2 flex h-2 w-full overflow-hidden rounded-full bg-gray-200">
          {incompatible > 0 && (
            <div
              className="h-full bg-red-500"
              style={{ width: `${(incompatible / rows.length) * 100}%` }}
              title={`${incompatible} incompatible`}
            />
          )}
          {untested > 0 && (
            <div
              className="h-full bg-gray-400"
              style={{ width: `${(untested / rows.length) * 100}%` }}
              title={`${untested} untested`}
            />
          )}
          {compatible > 0 && (
            <div
              className="h-full bg-green-500"
              style={{ width: `${(compatible / rows.length) * 100}%` }}
              title={`${compatible} compatible`}
            />
          )}
        </div>
      )}

      {/* Blocking cookbooks — always shown */}
      {blocking.length > 0 && (
        <div className="mt-3 space-y-2">
          {blocking.map((row) =>
            row.blocking ? (
              <BlockingCookbookCard key={row.name} bc={row.blocking} />
            ) : (
              <div key={row.name} className="flex items-center gap-2 rounded border border-red-100 bg-red-50 px-2 py-1.5 text-xs">
                <span className={`font-bold ${statusIcon(row.status).color}`}>{statusIcon(row.status).icon}</span>
                <Link
                  to={`/cookbooks/${encodeURIComponent(row.name)}`}
                  className="font-medium text-gray-700 hover:text-blue-600 hover:underline"
                >
                  {row.name}
                </Link>
                {row.version && <span className="text-gray-400">@{row.version}</span>}
                <span className={`rounded px-1 py-0.5 text-[10px] ${statusIcon(row.status).bg} ${statusIcon(row.status).color}`}>
                  {row.status}
                </span>
              </div>
            )
          )}
        </div>
      )}

      {/* Compatible cookbooks — toggle */}
      {nonBlocking.length > 0 && (
        <div className="mt-2">
          <button
            onClick={() => setShowAll(!showAll)}
            className="text-xs text-blue-600 hover:text-blue-800 hover:underline"
          >
            {showAll
              ? `Hide ${nonBlocking.length} compatible cookbook${nonBlocking.length !== 1 ? "s" : ""}`
              : `Show ${nonBlocking.length} compatible cookbook${nonBlocking.length !== 1 ? "s" : ""}`}
          </button>

          {showAll && (
            <div className="mt-1.5 flex flex-wrap gap-1">
              {displayedNonBlocking.map((row) => (
                <Link
                  key={row.name}
                  to={`/cookbooks/${encodeURIComponent(row.name)}`}
                  className="flex items-center gap-1 rounded bg-green-50 px-1.5 py-0.5 text-xs text-green-700 hover:bg-green-100"
                  title={row.version ? `${row.name}@${row.version}` : row.name}
                >
                  <span className="font-bold text-green-500">✓</span>
                  {row.name}
                  {row.version && <span className="text-green-400">@{row.version}</span>}
                </Link>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Readiness card for one target version
// ---------------------------------------------------------------------------

function ReadinessCard({
  r,
  nodeCookbooks,
}: {
  r: NodeReadiness;
  nodeCookbooks: Record<string, unknown> | null;
}) {
  const ready = r.is_ready;

  return (
    <div className={`rounded-lg border p-4 ${ready ? "border-green-200 bg-green-50/30" : "border-red-200 bg-red-50/20"}`}>
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
        {r.stale_data && (
          <span className="ml-auto rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700">
            ⚠ Stale data
          </span>
        )}
      </div>

      {/* Overall verdict */}
      <div className={`mt-3 rounded-lg px-3 py-2 text-sm ${ready ? "bg-green-100 text-green-800" : "bg-red-100 text-red-800"}`}>
        {ready ? (
          <span className="flex items-center gap-2">
            <span className="text-base">🟢</span>
            This node is <strong>ready</strong> to upgrade — all cookbooks are compatible and disk space is sufficient.
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

      {/* Analysis panels */}
      <div className="mt-4 space-y-3">
        <CookbookCompatibilityTable nodeCookbooks={nodeCookbooks} r={r} />
        <DiskSpacePanel r={r} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Readiness section (container)
// ---------------------------------------------------------------------------

function ReadinessSection({ data }: { data: NodeDetailResponse }) {
  if (!data.readiness || data.readiness.length === 0) return null;

  return (
    <div className="card">
      <h3 className="card-header">Upgrade Readiness</h3>
      <div className="space-y-6">
        {data.readiness.map((r) => (
          <ReadinessCard
            key={r.id}
            r={r}
            nodeCookbooks={data.node.cookbooks}
          />
        ))}
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

      {/* Readiness — promoted above run list / roles / cookbooks for visibility */}
      <ReadinessSection data={data} />

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

      {/* Raw Cookbooks — kept as a secondary reference below readiness */}
      {node.cookbooks && Object.keys(node.cookbooks).length > 0 && (
        <div className="card">
          <h3 className="card-header">Cookbooks (Raw)</h3>
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
