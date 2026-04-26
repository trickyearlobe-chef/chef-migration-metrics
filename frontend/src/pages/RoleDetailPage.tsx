import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { useTargetChefVersion } from "../hooks/useTargetChefVersion";
import { fetchRoleDetail } from "../api";
import type { RoleDetailResponse, RoleChainNode } from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { CompatibilityBadge } from "../components/StatusBadge";

function CompatibilitySummary({
  detail,
}: {
  detail: RoleDetailResponse;
}) {
  const total = detail.transitive_cookbooks?.length ?? 0;
  const blocking = detail.blocking_cookbooks?.length ?? 0;
  const compatible = total - blocking;

  return (
    <div className="grid grid-cols-3 gap-4">
      <div className="rounded-lg border border-green-200 bg-green-50 p-4 text-center">
        <div className="text-2xl font-bold text-green-700">{compatible}</div>
        <div className="text-xs text-green-600">Compatible</div>
      </div>
      <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-center">
        <div className="text-2xl font-bold text-red-700">{blocking}</div>
        <div className="text-xs text-red-600">Blocked</div>
      </div>
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-4 text-center">
        <div className="text-2xl font-bold text-gray-700">{total}</div>
        <div className="text-xs text-gray-600">Total Cookbooks</div>
      </div>
    </div>
  );
}

function BlockingCookbooksTable({
  detail,
}: {
  detail: RoleDetailResponse;
}) {
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

function BlastRadiusSection({
  detail,
}: {
  detail: RoleDetailResponse;
}) {
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

function RoleChainTree({ node, depth }: { node: RoleChainNode; depth: number }) {
  const indent = depth * 1.25;
  const isRole = node.type === "role";

  const statusColor =
    node.compatibility_status === "incompatible"
      ? "text-red-600"
      : node.compatibility_status === "untested"
        ? "text-gray-400"
        : "text-green-600";

  const linkTarget = isRole
    ? `/roles/${encodeURIComponent(node.name)}`
    : `/cookbooks/${encodeURIComponent(node.name)}`;

  return (
    <div>
      <div
        className="flex items-center gap-1.5 py-0.5"
        style={{ paddingLeft: `${indent}rem` }}
      >
        <span className="text-xs text-gray-400">
          {isRole ? "📁" : "📦"}
        </span>
        <Link
          to={linkTarget}
          className={`text-sm hover:underline ${isRole ? "font-medium text-blue-600" : statusColor}`}
        >
          {node.name}
        </Link>
        {!isRole && node.compatibility_status && (
          <CompatibilityBadge
            status={node.compatibility_status}
            size="sm"
          />
        )}
      </div>
      {node.children?.map((child, i) => (
        <RoleChainTree key={`${child.type}-${child.name}-${i}`} node={child} depth={depth + 1} />
      ))}
    </div>
  );
}

export function RoleDetailPage() {
  const { name } = useParams<{ name: string }>();
  const [detail, setDetail] = useState<RoleDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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
          <span>
            Organisations: {detail.organisations?.join(", ") || "—"}
          </span>
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
    </div>
  );
}
