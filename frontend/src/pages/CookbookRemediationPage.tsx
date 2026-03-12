import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { fetchCookbookRemediation, fetchFilterTargetChefVersions } from "../api";
import type {
  CookbookRemediationResponse,
  OffenseGroup,
  RemediationOffense,
  AutocorrectPreview,
  RemediationStatistics,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { ComplexityBadge, StatusBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Cookbook Remediation Detail Page
//
// Shows the full remediation picture for a single cookbook version:
//   1. Header with cookbook identity, complexity badge, and scan date
//   2. Statistics cards — total offenses, correctable vs manual, by category
//   3. Offense groups — grouped by cop name, each with:
//        - Remediation guidance (description, migration URL, before/after)
//        - Individual offense locations
//   4. Auto-correct preview — unified diff output
//
// Route: /cookbooks/:name/:version/remediation
// API:   GET /api/v1/cookbooks/:name/:version/remediation
// ---------------------------------------------------------------------------

export function CookbookRemediationPage() {
  const { name, version } = useParams<{ name: string; version: string }>();
  const [data, setData] = useState<CookbookRemediationResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Target version selector
  const [targetVersions, setTargetVersions] = useState<string[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<string>("");
  const [versionsLoading, setVersionsLoading] = useState(true);

  // Load available target versions on mount.
  useEffect(() => {
    setVersionsLoading(true);
    fetchFilterTargetChefVersions()
      .then((res) => {
        const versions = res.data ?? [];
        setTargetVersions(versions);
        if (versions.length > 0 && !selectedVersion) {
          setSelectedVersion(versions[0]);
        }
      })
      .catch(() => {
        // Non-fatal — the page can still load with the default target version.
      })
      .finally(() => setVersionsLoading(false));
    // intentionally run only when params change
  }, []);

  const load = useCallback(() => {
    if (!name || !version || !selectedVersion) return;
    setLoading(true);
    setError(null);
    fetchCookbookRemediation(name, version, {
      target_chef_version: selectedVersion,
    })
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name, version, selectedVersion]);

  useEffect(() => {
    load();
  }, [load]);

  // Expand/collapse state for offense groups.
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const toggleGroup = (copName: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(copName)) {
        next.delete(copName);
      } else {
        next.add(copName);
      }
      return next;
    });
  };
  const expandAll = () => {
    if (data) {
      setExpandedGroups(new Set(data.offense_groups.map((g) => g.cop_name)));
    }
  };
  const collapseAll = () => setExpandedGroups(new Set());

  // Diff viewer expand/collapse.
  const [diffExpanded, setDiffExpanded] = useState(false);

  if (loading && !data) {
    return <LoadingSpinner message="Loading remediation detail…" />;
  }
  if (error) return <ErrorAlert message={error} onRetry={load} />;
  if (!data) return null;

  const stats = data.statistics;
  const hasOffenses = data.offense_groups.length > 0;
  const hasPreview = data.autocorrect_preview?.available;

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <Link
          to="/remediation"
          className="hover:text-blue-600 hover:underline"
        >
          Remediation
        </Link>
        <span className="mx-1">/</span>
        <Link
          to={`/cookbooks/${encodeURIComponent(data.cookbook_name)}`}
          className="hover:text-blue-600 hover:underline"
        >
          {data.cookbook_name}
        </Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">{data.cookbook_version}</span>
      </nav>

      {/* Page header */}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-gray-800">
            {data.cookbook_name}
            <code className="ml-2 rounded bg-gray-100 px-2 py-0.5 text-base font-normal">
              {data.cookbook_version}
            </code>
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Remediation detail for target Chef{" "}
            <strong>{data.target_chef_version}</strong>
            {data.scanned_at && (
              <>
                {" · "}Scanned{" "}
                {new Date(data.scanned_at).toLocaleString()}
              </>
            )}
          </p>
        </div>

        <div className="flex items-center gap-3">
          {/* Target version selector */}
          {!versionsLoading && targetVersions.length > 1 && (
            <select
              className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              value={selectedVersion}
              onChange={(e) => setSelectedVersion(e.target.value)}
            >
              {targetVersions.map((v) => (
                <option key={v} value={v}>
                  Chef {v}
                </option>
              ))}
            </select>
          )}

          {/* Badges */}
          <ComplexityBadge
            complexityLabel={data.complexity_label}
            score={data.complexity_score}
          />
          {data.cookstyle_passed !== null && (
            <StatusBadge
              variant={data.cookstyle_passed ? "compatible" : "incompatible"}
              label={
                data.cookstyle_passed ? "CookStyle Passed" : "CookStyle Failed"
              }
            />
          )}
        </div>
      </div>

      {/* Statistics cards */}
      <StatisticsCards stats={stats} />

      {/* Offense groups */}
      {hasOffenses ? (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold text-gray-800">
              Offense Groups
              <span className="ml-2 text-sm font-normal text-gray-500">
                ({data.offense_groups.length} cop
                {data.offense_groups.length !== 1 ? "s" : ""},{" "}
                {stats.total_offenses} offense
                {stats.total_offenses !== 1 ? "s" : ""})
              </span>
            </h3>
            <div className="flex gap-2">
              <button
                className="rounded-md px-2.5 py-1 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-100"
                onClick={expandAll}
              >
                Expand All
              </button>
              <button
                className="rounded-md px-2.5 py-1 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-100"
                onClick={collapseAll}
              >
                Collapse All
              </button>
            </div>
          </div>

          {data.offense_groups.map((group) => (
            <OffenseGroupCard
              key={group.cop_name}
              group={group}
              expanded={expandedGroups.has(group.cop_name)}
              onToggle={() => toggleGroup(group.cop_name)}
            />
          ))}
        </div>
      ) : (
        <EmptyState
          title="No offenses found"
          description={
            data.cookstyle_passed === null
              ? "No CookStyle scan results are available for this cookbook version and target."
              : "This cookbook version has no CookStyle offenses — it looks ready for the target Chef version!"
          }
        />
      )}

      {/* Auto-correct preview */}
      {hasPreview && (
        <AutocorrectPreviewCard
          preview={data.autocorrect_preview}
          expanded={diffExpanded}
          onToggle={() => setDiffExpanded((p) => !p)}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Statistics Cards
// ---------------------------------------------------------------------------

function StatisticsCards({ stats }: { stats: RemediationStatistics }) {
  const cards = [
    {
      label: "Total Offenses",
      value: stats.total_offenses,
      color: "text-gray-800",
      icon: (
        <svg
          className="h-5 w-5 text-gray-400"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={1.5}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 9v3.75m9-.75a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 3.75h.008v.008H12v-.008Z"
          />
        </svg>
      ),
    },
    {
      label: "Auto-Correctable",
      value: stats.correctable_offenses,
      sub: `${stats.total_offenses > 0 ? Math.round((stats.correctable_offenses / stats.total_offenses) * 100) : 0}% of total`,
      color: "text-green-700",
      icon: (
        <svg
          className="h-5 w-5 text-green-500"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={1.5}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
          />
        </svg>
      ),
    },
    {
      label: "Manual Fix Required",
      value: stats.remaining_offenses,
      sub:
        stats.manual_fix_count > 0
          ? `${stats.manual_fix_count} complexity-counted`
          : undefined,
      color: "text-amber-700",
      icon: (
        <svg
          className="h-5 w-5 text-amber-500"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={1.5}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M11.42 15.17 17.25 21A2.652 2.652 0 0 0 21 17.25l-5.877-5.877M11.42 15.17l2.496-3.03c.317-.384.74-.626 1.208-.766M11.42 15.17l-4.655 5.653a2.548 2.548 0 1 1-3.586-3.586l6.837-5.63m5.108-.233c.55-.164 1.163-.188 1.743-.14a4.5 4.5 0 0 0 4.486-6.336l-3.276 3.277a3.004 3.004 0 0 1-2.25-2.25l3.276-3.276a4.5 4.5 0 0 0-6.336 4.486c.091 1.076-.071 2.264-.904 2.95l-.102.085"
          />
        </svg>
      ),
    },
    {
      label: "Deprecations",
      value: stats.deprecation_count,
      sub:
        stats.error_count > 0
          ? `${stats.error_count} error${stats.error_count !== 1 ? "s" : ""}`
          : undefined,
      color: "text-purple-700",
      icon: (
        <svg
          className="h-5 w-5 text-purple-500"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={1.5}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
          />
        </svg>
      ),
    },
  ];

  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      {cards.map((card) => (
        <div key={card.label} className="stat-card">
          <div className="flex items-center gap-2">
            {card.icon}
            <span className="stat-label">{card.label}</span>
          </div>
          <span className={`stat-value ${card.color}`}>
            {card.value.toLocaleString()}
          </span>
          {card.sub && <span className="stat-sub">{card.sub}</span>}
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Offense Group Card
// ---------------------------------------------------------------------------

function OffenseGroupCard({
  group,
  expanded,
  onToggle,
}: {
  group: OffenseGroup;
  expanded: boolean;
  onToggle: () => void;
}) {
  const allCorrectable = group.correctable_count === group.count;
  const noneCorrectable = group.correctable_count === 0;

  const severityColor: Record<string, string> = {
    error: "border-red-300 bg-red-50/40",
    warning: "border-amber-300 bg-amber-50/40",
    convention: "border-blue-200 bg-blue-50/30",
    refactor: "border-purple-200 bg-purple-50/30",
  };
  const borderClass =
    severityColor[group.severity] ?? "border-gray-200 bg-white";

  return (
    <div className={`rounded-lg border ${borderClass} shadow-sm`}>
      {/* Clickable header */}
      <button
        type="button"
        className="flex w-full items-center gap-3 px-4 py-3 text-left"
        onClick={onToggle}
        aria-expanded={expanded}
      >
        {/* Chevron */}
        <svg
          className={`h-4 w-4 shrink-0 text-gray-400 transition-transform duration-200 ${expanded ? "rotate-90" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={2}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="m8.25 4.5 7.5 7.5-7.5 7.5"
          />
        </svg>

        {/* Cop name */}
        <span className="min-w-0 flex-1 truncate text-sm font-semibold text-gray-800">
          {group.cop_name}
        </span>

        {/* Badges */}
        <span className="shrink-0 rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
          {group.count} offense{group.count !== 1 ? "s" : ""}
        </span>
        {allCorrectable && (
          <span className="shrink-0 rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
            All auto-correctable
          </span>
        )}
        {!allCorrectable && !noneCorrectable && (
          <span className="shrink-0 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700">
            {group.correctable_count} auto-correctable
          </span>
        )}
        {noneCorrectable && (
          <span className="shrink-0 rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">
            Manual fix
          </span>
        )}
        <span className="shrink-0 text-xs capitalize text-gray-400">
          {group.severity}
        </span>
      </button>

      {/* Expanded content */}
      {expanded && (
        <div className="border-t border-gray-200 px-4 py-3 space-y-4">
          {/* Remediation guidance */}
          {group.remediation && (
            <div className="rounded-md bg-white/80 p-4 ring-1 ring-gray-200">
              <h4 className="mb-1 text-sm font-semibold text-gray-700">
                Remediation Guidance
              </h4>
              <p className="text-sm text-gray-600">
                {group.remediation.description}
              </p>

              {/* Migration URL */}
              {group.remediation.migration_url && (
                <a
                  href={group.remediation.migration_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="mt-2 inline-flex items-center gap-1 text-sm font-medium text-blue-600 hover:text-blue-800 hover:underline"
                >
                  <svg
                    className="h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    strokeWidth={1.5}
                    stroke="currentColor"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M13.5 6H5.25A2.25 2.25 0 0 0 3 8.25v10.5A2.25 2.25 0 0 0 5.25 21h10.5A2.25 2.25 0 0 0 18 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25"
                    />
                  </svg>
                  Migration Documentation
                </a>
              )}

              {/* Version lifecycle */}
              {(group.remediation.introduced_in ||
                group.remediation.removed_in) && (
                  <div className="mt-2 flex gap-4 text-xs text-gray-500">
                    {group.remediation.introduced_in && (
                      <span>
                        Introduced in Chef{" "}
                        <strong>{group.remediation.introduced_in}</strong>
                      </span>
                    )}
                    {group.remediation.removed_in && (
                      <span>
                        Removed in Chef{" "}
                        <strong>{group.remediation.removed_in}</strong>
                      </span>
                    )}
                  </div>
                )}

              {/* Replacement pattern (before/after) */}
              {group.remediation.replacement_pattern && (
                <div className="mt-3">
                  <h5 className="mb-1 text-xs font-medium text-gray-500">
                    Before / After
                  </h5>
                  <pre className="overflow-x-auto rounded-md bg-gray-900 p-3 text-xs leading-relaxed text-gray-100">
                    {group.remediation.replacement_pattern}
                  </pre>
                </div>
              )}
            </div>
          )}

          {/* Individual offense list */}
          <div>
            <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-500">
              Locations ({group.offenses.length})
            </h4>
            <div className="space-y-1">
              {group.offenses.map((offense, idx) => (
                <OffenseRow key={idx} offense={offense} />
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Single offense row
// ---------------------------------------------------------------------------

function OffenseRow({ offense }: { offense: RemediationOffense }) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 rounded px-2 py-1.5 text-sm hover:bg-gray-50">
      {/* File:line */}
      <span className="font-mono text-xs text-blue-700">
        {offense.location.file || "unknown"}:{offense.location.start_line}
        {offense.location.last_line !== offense.location.start_line &&
          `–${offense.location.last_line}`}
      </span>

      {/* Message */}
      <span className="text-gray-600">{offense.message}</span>

      {/* Correctable badge */}
      {offense.correctable && (
        <span className="rounded-full bg-green-100 px-1.5 py-0.5 text-[10px] font-medium text-green-700 ring-1 ring-inset ring-green-600/20">
          auto-correctable
        </span>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Auto-correct Preview Card
// ---------------------------------------------------------------------------

function AutocorrectPreviewCard({
  preview,
  expanded,
  onToggle,
}: {
  preview: AutocorrectPreview;
  expanded: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="card">
      <button
        type="button"
        className="flex w-full items-center justify-between text-left"
        onClick={onToggle}
        aria-expanded={expanded}
      >
        <div>
          <h3 className="text-lg font-semibold text-gray-800">
            Auto-Correct Preview
          </h3>
          <p className="mt-0.5 text-sm text-gray-500">
            {preview.correctable_offenses} of {preview.total_offenses} offenses
            correctable · {preview.files_modified} file
            {preview.files_modified !== 1 ? "s" : ""} modified
            {preview.remaining_offenses > 0 && (
              <> · {preview.remaining_offenses} remaining after auto-correct</>
            )}
            {preview.generated_at && (
              <>
                {" · "}Generated{" "}
                {new Date(preview.generated_at).toLocaleString()}
              </>
            )}
          </p>
        </div>
        <svg
          className={`h-5 w-5 shrink-0 text-gray-400 transition-transform duration-200 ${expanded ? "rotate-180" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={2}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="m19.5 8.25-7.5 7.5-7.5-7.5"
          />
        </svg>
      </button>

      {expanded && preview.diff_output && (
        <div className="mt-4 overflow-x-auto rounded-md bg-gray-900 text-xs leading-relaxed">
          <pre className="p-4">
            {preview.diff_output.split("\n").map((line, idx) => {
              let cls = "text-gray-300";
              if (line.startsWith("+++") || line.startsWith("---")) {
                cls = "text-gray-400 font-bold";
              } else if (line.startsWith("@@")) {
                cls = "text-cyan-400";
              } else if (line.startsWith("+")) {
                cls = "text-green-400";
              } else if (line.startsWith("-")) {
                cls = "text-red-400";
              }
              return (
                <span key={idx} className={cls}>
                  {line}
                  {"\n"}
                </span>
              );
            })}
          </pre>
        </div>
      )}
    </div>
  );
}
