import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { fetchCookbookDetail, requestCookbookRescan } from "../api";
import type { CookbookDetailResponse } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { StatusBadge } from "../components/StatusBadge";

/** Small helper – renders a label/value row in the metadata grid. */
function MetaRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-2 py-1.5 text-sm">
      <dt className="w-36 shrink-0 font-medium text-gray-500">{label}</dt>
      <dd className="text-gray-800 break-words min-w-0">{children}</dd>
    </div>
  );
}

/** Render a JSON-style map (platforms or dependencies) as inline badges. */
function MapBadges({ map }: { map?: Record<string, string> }) {
  if (!map || Object.keys(map).length === 0) return <span className="text-gray-400 italic">None</span>;
  return (
    <div className="flex flex-wrap gap-1.5">
      {Object.entries(map).map(([k, v]) => (
        <span
          key={k}
          className="inline-flex items-center rounded-full border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs text-gray-700"
        >
          {k}{v ? ` ${v}` : ""}
        </span>
      ))}
    </div>
  );
}


export function CookbookDetailPage() {
  const [expandedMeta, setExpandedMeta] = useState<Record<string, boolean>>({});

  const toggleMeta = (key: string) =>
    setExpandedMeta((prev) => ({ ...prev, [key]: !prev[key] }));

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

  const hasGitRepos = data.git_repos && data.git_repos.length > 0;
  const hasServerCookbooks = data.server_cookbooks && data.server_cookbooks.length > 0;

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
          title="Invalidate cached CookStyle results for server cookbook versions and rescan on the next collection cycle"
        >
          {rescanning ? "Requesting…" : "Rescan CookStyle"}
        </button>
      </div>

      {rescanMsg && (
        <div className="rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
          {rescanMsg}
        </div>
      )}

      {/* Link to the git repo detail page when a git repo exists for this cookbook */}
      {hasGitRepos && (
        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <h4 className="text-sm font-medium text-gray-600">Git Repository</h4>
              <p className="mt-1 text-sm text-gray-500">
                This cookbook also has a git repository source.
                View CookStyle results, Test Kitchen results, committers, and remediation detail on the git repo page.
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {data.git_repos.map((gd, idx) => (
                  <span
                    key={idx}
                    className="text-xs text-gray-400 truncate max-w-sm"
                    title={gd.git_repo.git_repo_url}
                  >
                    {gd.git_repo.git_repo_url}
                  </span>
                ))}
              </div>
            </div>
            <Link
              to={`/git-repos/${encodeURIComponent(data.name)}`}
              className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-blue-600 shadow-sm hover:bg-gray-50"
            >
              View Git Repo →
            </Link>
          </div>
        </div>
      )}

      {!hasServerCookbooks && !hasGitRepos ? (
        <EmptyState title="No versions found" />
      ) : !hasServerCookbooks ? (
        /* When there are only git repos and no server cookbooks, direct the user to the git repo page */
        <div className="rounded-md border border-gray-200 bg-gray-50 px-4 py-6 text-center text-sm text-gray-500">
          No Chef Server versions found for this cookbook.
          {hasGitRepos && (
            <span>
              {" "}See the{" "}
              <Link
                to={`/git-repos/${encodeURIComponent(data.name)}`}
                className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
              >
                git repo page
              </Link>
              {" "}for source repository details.
            </span>
          )}
        </div>
      ) : (
        <div className="space-y-4">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-gray-500">Chef Server Versions</h3>
          {data.server_cookbooks.map((vd, idx) => {
            const cb = vd.cookbook;
            return (
              <div key={`sc-${idx}`} className="card">
                <div className="mb-4 flex flex-wrap items-center gap-3">
                  <h3 className="text-base font-semibold text-gray-800">
                    {cb.name}
                    {cb.version && (
                      <code className="ml-2 rounded bg-gray-100 px-1.5 py-0.5 text-sm font-normal">
                        {cb.version}
                      </code>
                    )}
                  </h3>
                  <span className="badge badge-cookstyle">Chef Server</span>
                  <StatusBadge variant={cb.is_active ? "active" : "inactive"} size="sm" />
                  {cb.is_stale_cookbook && <StatusBadge variant="stale" label="Stale" size="sm" />}
                  {cb.is_frozen && (
                    <span className="inline-flex items-center rounded-full border border-blue-200 bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700">
                      Frozen
                    </span>
                  )}
                </div>

                {/* Metadata toggle */}
                <div className="mb-4">
                  <button
                    onClick={() => toggleMeta(`sc-${idx}`)}
                    className="inline-flex items-center gap-1 text-sm font-medium text-blue-600 hover:text-blue-800"
                  >
                    <span className="text-xs">{expandedMeta[`sc-${idx}`] ? "▼" : "▶"}</span>
                    {expandedMeta[`sc-${idx}`] ? "Hide" : "Show"} Metadata
                  </button>

                  {expandedMeta[`sc-${idx}`] && (
                    <dl className="mt-3 rounded-lg border border-gray-100 bg-gray-50 px-4 py-2 divide-y divide-gray-100">
                      {cb.description && (
                        <MetaRow label="Description">{cb.description}</MetaRow>
                      )}
                      {cb.maintainer && (
                        <MetaRow label="Maintainer">{cb.maintainer}</MetaRow>
                      )}
                      {cb.license && (
                        <MetaRow label="License">{cb.license}</MetaRow>
                      )}
                      <MetaRow label="Download">
                        <span className={cb.download_status === "ok" ? "text-green-700" : "text-amber-600"}>
                          {cb.download_status}
                        </span>
                        {cb.download_error && (
                          <span className="ml-2 text-xs text-red-500">({cb.download_error})</span>
                        )}
                      </MetaRow>
                      <MetaRow label="Platforms">
                        <MapBadges map={cb.platforms} />
                      </MetaRow>
                      <MetaRow label="Dependencies">
                        <MapBadges map={cb.dependencies} />
                      </MetaRow>
                      {cb.first_seen_at && (
                        <MetaRow label="First Seen">{new Date(cb.first_seen_at).toLocaleString()}</MetaRow>
                      )}
                      {cb.last_fetched_at && (
                        <MetaRow label="Last Fetched">{new Date(cb.last_fetched_at).toLocaleString()}</MetaRow>
                      )}
                      {cb.updated_at && (
                        <MetaRow label="Updated At">{new Date(cb.updated_at).toLocaleString()}</MetaRow>
                      )}
                    </dl>
                  )}
                </div>

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
                          {cb.version && cs.target_chef_version && (
                            <Link
                              to={`/cookbooks/${encodeURIComponent(cb.name)}/${encodeURIComponent(cb.version)}/remediation?target_chef_version=${encodeURIComponent(cs.target_chef_version)}`}
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
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
