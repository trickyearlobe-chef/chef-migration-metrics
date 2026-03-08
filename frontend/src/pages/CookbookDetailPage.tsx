import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { fetchCookbookDetail, requestCookbookRescan } from "../api";
import type { CookbookDetailResponse } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { StatusBadge, ComplexityBadge } from "../components/StatusBadge";

export function CookbookDetailPage() {
  const { name } = useParams<{ name: string }>();
  const [data, setData] = useState<CookbookDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [rescanning, setRescanning] = useState(false);
  const [rescanMsg, setRescanMsg] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!name) return;
    setLoading(true);
    setError(null);
    fetchCookbookDetail(name)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name]);

  const handleRescan = useCallback(() => {
    if (!name) return;
    setRescanning(true);
    setRescanMsg(null);
    requestCookbookRescan(name)
      .then((res) => {
        setRescanMsg(res.message);
        load();
      })
      .catch((e: Error) => setRescanMsg(`Rescan failed: ${e.message}`))
      .finally(() => setRescanning(false));
  }, [name, load]);

  useEffect(() => { load(); }, [load]);

  if (loading) return <LoadingSpinner message="Loading cookbook detail…" />;
  if (error) return <ErrorAlert message={error} onRetry={load} />;
  if (!data) return null;

  return (
    <div className="space-y-6">
      <nav className="text-sm text-gray-500">
        <Link to="/cookbooks" className="hover:text-blue-600 hover:underline">Cookbooks</Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">{data.name}</span>
      </nav>

      <div className="flex items-center gap-4">
        <h2 className="text-xl font-bold text-gray-800">{data.name}</h2>
        <button
          onClick={handleRescan}
          disabled={rescanning}
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
          title="Invalidate cached CookStyle results and rescan on the next collection cycle"
        >
          {rescanning ? "Requesting…" : "Rescan CookStyle"}
        </button>
      </div>

      {rescanMsg && (
        <div className="rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
          {rescanMsg}
        </div>
      )}

      {data.data.length === 0 ? (
        <EmptyState title="No versions found" />
      ) : (
        <div className="space-y-4">
          {data.data.map((vd, idx) => {
            const cb = vd.cookbook;
            return (
              <div key={idx} className="card">
                <div className="mb-4 flex flex-wrap items-center gap-3">
                  <h3 className="text-base font-semibold text-gray-800">
                    {cb.name}
                    {cb.version && (
                      <code className="ml-2 rounded bg-gray-100 px-1.5 py-0.5 text-sm font-normal">
                        {cb.version}
                      </code>
                    )}
                  </h3>
                  <span className={`badge ${cb.source === "git" ? "badge-compatible" : "badge-cookstyle"}`}>
                    {cb.source === "git" ? "Git" : "Chef Server"}
                  </span>
                  <StatusBadge variant={cb.is_active ? "active" : "inactive"} size="sm" />
                  {cb.is_stale_cookbook && <StatusBadge variant="stale" label="Stale" size="sm" />}
                  {cb.has_test_suite ? (
                    <StatusBadge variant="compatible" label="Has Test Suite" size="sm" />
                  ) : (
                    <StatusBadge variant="untested" label="No Test Suite" size="sm" />
                  )}
                </div>

                {/* Complexity scores */}
                {vd.complexity && vd.complexity.length > 0 && (
                  <div className="mb-4">
                    <h4 className="mb-2 text-sm font-medium text-gray-600">Complexity</h4>
                    <div className="space-y-2">
                      {vd.complexity.map((c) => (
                        <div key={c.id} className="flex flex-wrap items-center gap-3 rounded-lg border border-gray-100 p-3">
                          <span className="text-xs text-gray-500">Target: {c.target_chef_version}</span>
                          <ComplexityBadge complexityLabel={c.complexity_label} score={c.complexity_score} size="sm" />
                          <span className="text-xs text-gray-500">
                            Auto: {c.auto_correctable_count} | Manual: {c.manual_fix_count}
                          </span>
                          <span className="text-xs text-gray-400">
                            Errors: {c.error_count} | Deprecations: {c.deprecation_count}
                          </span>
                          {cb.version && (
                            <Link
                              to={`/cookbooks/${encodeURIComponent(cb.name)}/${encodeURIComponent(cb.version)}/remediation?target_chef_version=${encodeURIComponent(c.target_chef_version)}`}
                              className="ml-auto text-xs font-medium text-blue-600 hover:text-blue-800 hover:underline"
                            >
                              View Remediation Detail →
                            </Link>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Cookstyle results */}
                {vd.cookstyle && vd.cookstyle.length > 0 && (
                  <div>
                    <h4 className="mb-2 text-sm font-medium text-gray-600">CookStyle Results</h4>
                    <div className="space-y-2">
                      {vd.cookstyle.map((cs) => (
                        <div key={cs.id} className="flex flex-wrap items-center gap-3 rounded-lg border border-gray-100 p-3">
                          <span className="text-xs text-gray-500">Target: {cs.target_chef_version}</span>
                          <StatusBadge
                            variant={cs.passed ? "compatible" : "incompatible"}
                            label={cs.passed ? "Passed" : "Failed"}
                            size="sm"
                          />
                          <span className="text-xs text-gray-500">
                            Offences: {cs.offence_count} | Deprecations: {cs.deprecation_count}
                          </span>
                          <span className="text-xs text-gray-400">
                            Scanned: {new Date(cs.scanned_at).toLocaleString()}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
