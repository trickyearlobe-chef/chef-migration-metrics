import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import {
  fetchRemediationSummary,
  fetchRemediationPriority,
  fetchFilterTargetChefVersions,
  fetchFilterComplexityLabels,
  type RemediationQuery,
} from "../api";
import type {
  RemediationSummaryResponse,
  RemediationPriorityResponse,
  RemediationPriorityItem,
  Pagination as PaginationType,
  ExportFilters,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { ComplexityBadge } from "../components/StatusBadge";
import { ExportButton } from "../components/ExportButton";

// ---------------------------------------------------------------------------
// Remediation Priority page
//
// Shows what to fix first and how:
//   1. Effort summary header — total cookbooks needing remediation, quick
//      wins, manual fixes, blocked nodes
//   2. Target Chef version selector
//   3. Priority-sorted table with complexity badges, blast radius,
//      auto-correct vs manual counts
//   4. Links from each row to the cookbook detail page
// ---------------------------------------------------------------------------

export function RemediationPage() {
  const { selectedOrg } = useOrg();
  const org = selectedOrg || undefined;

  // Target Chef version state
  const [targetVersions, setTargetVersions] = useState<string[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<string>("");
  const [versionsLoading, setVersionsLoading] = useState(true);

  // Complexity label filter state
  const [complexityLabels, setComplexityLabels] = useState<string[]>([]);
  const [selectedComplexity, setSelectedComplexity] = useState<string>("");

  // Summary state
  const [summary, setSummary] = useState<RemediationSummaryResponse | null>(null);
  const [summaryLoading, setSummaryLoading] = useState(false);
  const [summaryError, setSummaryError] = useState<string | null>(null);

  // Priority table state
  const [priority, setPriority] = useState<RemediationPriorityResponse | null>(null);
  const [priorityLoading, setPriorityLoading] = useState(false);
  const [priorityError, setPriorityError] = useState<string | null>(null);

  // Sorting and pagination
  const [sortField, setSortField] = useState<string>("priority_score");
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">("desc");
  const [page, setPage] = useState(1);
  const perPage = 50;

  // -----------------------------------------------------------------------
  // Load available target Chef versions and complexity labels on mount
  // -----------------------------------------------------------------------
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
        // If we can't load versions, the summary/priority calls will use
        // the backend default. Leave the selector empty.
      })
      .finally(() => setVersionsLoading(false));

    fetchFilterComplexityLabels()
      .then((res) => setComplexityLabels(res.data ?? []))
      .catch(() => setComplexityLabels([]));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // -----------------------------------------------------------------------
  // Load summary whenever org or target version changes
  // -----------------------------------------------------------------------
  const loadSummary = useCallback(() => {
    if (!selectedVersion && targetVersions.length === 0 && versionsLoading) return;

    setSummaryLoading(true);
    setSummaryError(null);

    fetchRemediationSummary({
      organisation: org,
      target_chef_version: selectedVersion || undefined,
    })
      .then(setSummary)
      .catch((e: Error) => setSummaryError(e.message))
      .finally(() => setSummaryLoading(false));
  }, [org, selectedVersion, targetVersions.length, versionsLoading]);

  useEffect(() => {
    loadSummary();
  }, [loadSummary]);

  // -----------------------------------------------------------------------
  // Load priority table whenever org, version, sort, or page changes
  // -----------------------------------------------------------------------
  const loadPriority = useCallback(() => {
    if (!selectedVersion && targetVersions.length === 0 && versionsLoading) return;

    setPriorityLoading(true);
    setPriorityError(null);

    const filters: RemediationQuery = {
      page,
      per_page: perPage,
      sort: sortField,
      order: sortOrder,
      organisation: org,
      target_chef_version: selectedVersion || undefined,
      complexity_label: selectedComplexity || undefined,
    };

    fetchRemediationPriority(filters)
      .then(setPriority)
      .catch((e: Error) => setPriorityError(e.message))
      .finally(() => setPriorityLoading(false));
  }, [org, selectedVersion, selectedComplexity, sortField, sortOrder, page, targetVersions.length, versionsLoading]);

  useEffect(() => {
    loadPriority();
  }, [loadPriority]);

  // Reset page when filters change
  useEffect(() => {
    setPage(1);
  }, [org, selectedVersion, selectedComplexity, sortField, sortOrder]);

  // -----------------------------------------------------------------------
  // Sort handler — toggle direction if clicking the same column
  // -----------------------------------------------------------------------
  const handleSort = (field: string) => {
    if (sortField === field) {
      setSortOrder((prev) => (prev === "desc" ? "asc" : "desc"));
    } else {
      setSortField(field);
      // Default to descending for numeric fields, ascending for name
      setSortOrder(field === "name" ? "asc" : "desc");
    }
  };

  const sortIndicator = (field: string) => {
    if (sortField !== field) return null;
    return sortOrder === "desc" ? " ↓" : " ↑";
  };

  return (
    <div className="space-y-6">
      {/* Page header with version selector */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-xl font-bold text-gray-800">Remediation Priority</h2>
        <div className="flex items-center gap-4">
          {/* Target Chef version selector */}
          <div className="flex items-center gap-2">
            <label className="text-sm font-medium text-gray-500">Target Chef Version</label>
            {versionsLoading ? (
              <span className="text-xs text-gray-400">Loading…</span>
            ) : targetVersions.length === 0 ? (
              <span className="text-xs text-gray-400">No target versions configured</span>
            ) : (
              <select
                value={selectedVersion}
                onChange={(e) => setSelectedVersion(e.target.value)}
                className="block w-36 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                {targetVersions.map((v) => (
                  <option key={v} value={v}>{v}</option>
                ))}
              </select>
            )}
          </div>

          {/* Complexity label filter */}
          {complexityLabels.length > 0 && (
            <div className="flex items-center gap-2">
              <label className="text-sm font-medium text-gray-500">Complexity</label>
              <select
                value={selectedComplexity}
                onChange={(e) => setSelectedComplexity(e.target.value)}
                className="block w-32 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                <option value="">All</option>
                {complexityLabels.map((label) => (
                  <option key={label} value={label}>
                    {label.charAt(0).toUpperCase() + label.slice(1)}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Export cookbook remediation data */}
          <ExportButton
            exportType="cookbook_remediation"
            targetChefVersion={selectedVersion || undefined}
            filters={
              {
                ...(org ? { organisation: org } : {}),
                ...(selectedVersion ? { target_chef_version: selectedVersion } : {}),
                ...(selectedComplexity ? { complexity_label: selectedComplexity } : {}),
              } as ExportFilters
            }
            label="Export"
          />
        </div>
      </div>

      {/* Summary cards */}
      <SummaryHeader
        summary={summary}
        loading={summaryLoading}
        error={summaryError}
        onRetry={loadSummary}
      />

      {/* Priority table */}
      <PriorityTable
        data={priority}
        loading={priorityLoading}
        error={priorityError}
        onRetry={loadPriority}
        onSort={handleSort}
        sortField={sortField}
        sortIndicator={sortIndicator}
        page={page}
        onPageChange={setPage}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Summary Header — stat cards row
// ---------------------------------------------------------------------------

function SummaryHeader({
  summary,
  loading,
  error,
  onRetry,
}: {
  summary: RemediationSummaryResponse | null;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
}) {
  if (loading) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {[1, 2, 3, 4].map((i) => (
          <div key={i} className="stat-card animate-pulse">
            <div className="h-4 w-24 rounded bg-gray-200" />
            <div className="mt-3 h-7 w-16 rounded bg-gray-200" />
            <div className="mt-2 h-3 w-32 rounded bg-gray-100" />
          </div>
        ))}
      </div>
    );
  }

  if (error) {
    return <ErrorAlert message={error} onRetry={onRetry} />;
  }

  if (!summary) return null;

  const cards = [
    {
      label: "Need Remediation",
      value: summary.total_needing_remediation,
      sub: `of ${summary.total_cookbooks_evaluated} evaluated`,
      color: summary.total_needing_remediation > 0 ? "text-red-600" : "text-green-600",
      icon: "M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z",
    },
    {
      label: "Quick Wins",
      value: summary.quick_wins,
      sub: "auto-correctable only — no manual fixes",
      color: summary.quick_wins > 0 ? "text-green-600" : "text-gray-500",
      icon: "M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z",
    },
    {
      label: "Manual Fixes",
      value: summary.manual_fixes,
      sub: `${summary.total_auto_correctable.toLocaleString()} auto + ${summary.total_manual_fix.toLocaleString()} manual issues`,
      color: summary.manual_fixes > 0 ? "text-amber-600" : "text-gray-500",
      icon: "M11.42 15.17 17.25 21A2.652 2.652 0 0 0 21 17.25l-5.877-5.877M11.42 15.17l2.496-3.03c.317-.384.74-.626 1.208-.766M11.42 15.17l-4.655 5.653a2.548 2.548 0 1 1-3.586-3.586l6.837-5.63m5.108-.233c.55-.164 1.163-.188 1.743-.14a4.5 4.5 0 0 0 4.486-6.336l-3.276 3.277a3.004 3.004 0 0 1-2.25-2.25l3.276-3.276a4.5 4.5 0 0 0-6.336 4.486c.049.58.025 1.193-.14 1.743",
    },
    {
      label: "Blocked Nodes",
      value: summary.blocked_nodes_by_readiness,
      sub: `${summary.blocked_nodes_by_complexity.toLocaleString()} by cookbook complexity`,
      color: summary.blocked_nodes_by_readiness > 0 ? "text-red-600" : "text-green-600",
      icon: "M18.364 18.364A9 9 0 0 0 5.636 5.636m12.728 12.728A9 9 0 0 1 5.636 5.636m12.728 12.728L5.636 5.636",
    },
  ];

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      {cards.map((card) => (
        <div key={card.label} className="stat-card">
          <div className="flex items-center gap-2">
            <svg
              className="h-4 w-4 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
              aria-hidden="true"
            >
              <path strokeLinecap="round" strokeLinejoin="round" d={card.icon} />
            </svg>
            <span className="stat-label">{card.label}</span>
          </div>
          <span className={`stat-value ${card.color}`}>
            {card.value.toLocaleString()}
          </span>
          <span className="stat-sub">{card.sub}</span>
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Priority Table
// ---------------------------------------------------------------------------

function PriorityTable({
  data,
  loading,
  error,
  onRetry,
  onSort,
  sortField,
  sortIndicator,
  page,
  onPageChange,
}: {
  data: RemediationPriorityResponse | null;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  onSort: (field: string) => void;
  sortField: string;
  sortIndicator: (field: string) => string | null;
  page: number;
  onPageChange: (page: number) => void;
}) {
  if (loading) return <LoadingSpinner message="Loading remediation priorities…" />;
  if (error) return <ErrorAlert message={error} onRetry={onRetry} />;
  if (!data) return null;

  const items: RemediationPriorityItem[] = data.data ?? [];
  const pagination: PaginationType | undefined = data.pagination;

  if (items.length === 0 && page === 1) {
    return (
      <EmptyState
        title="No cookbooks need remediation"
        description="All cookbooks are compatible with the target Chef version, or no analysis data is available yet."
      />
    );
  }

  // Aggregate row at the top of the table
  const totalRow = {
    auto: data.total_auto_correctable,
    manual: data.total_manual_fix,
    deprecations: data.total_deprecations,
    errors: data.total_errors,
    cookbooks: data.total_cookbooks,
  };

  const sortableHeader = (field: string, label: string) => (
    <th
      className="cursor-pointer select-none hover:text-gray-700"
      onClick={() => onSort(field)}
      aria-sort={
        sortField === field
          ? sortIndicator(field) === " ↓"
            ? "descending"
            : "ascending"
          : "none"
      }
    >
      <span className="inline-flex items-center gap-1">
        {label}
        <span className="text-blue-500">{sortIndicator(field)}</span>
      </span>
    </th>
  );

  return (
    <div className="space-y-3">
      {/* Aggregate banner */}
      <div className="flex flex-wrap items-center gap-4 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 text-sm">
        <span className="font-medium text-gray-700">
          {totalRow.cookbooks.toLocaleString()} cookbook{totalRow.cookbooks !== 1 ? "s" : ""}
        </span>
        <span className="text-gray-400">|</span>
        <span className="text-green-700">
          <svg className="mr-1 inline h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          {totalRow.auto.toLocaleString()} auto-correctable
        </span>
        <span className="text-amber-700">
          <svg className="mr-1 inline h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M11.42 15.17 17.25 21A2.652 2.652 0 0 0 21 17.25l-5.877-5.877M11.42 15.17l2.496-3.03c.317-.384.74-.626 1.208-.766M11.42 15.17l-4.655 5.653a2.548 2.548 0 1 1-3.586-3.586l6.837-5.63m5.108-.233c.55-.164 1.163-.188 1.743-.14a4.5 4.5 0 0 0 4.486-6.336l-3.276 3.277a3.004 3.004 0 0 1-2.25-2.25l3.276-3.276a4.5 4.5 0 0 0-6.336 4.486c.049.58.025 1.193-.14 1.743" />
          </svg>
          {totalRow.manual.toLocaleString()} manual fixes
        </span>
        <span className="text-purple-700">
          {totalRow.deprecations.toLocaleString()} deprecations
        </span>
        {totalRow.errors > 0 && (
          <span className="text-red-700">
            {totalRow.errors.toLocaleString()} errors
          </span>
        )}
      </div>

      {/* Table */}
      <div className="table-container">
        <table className="table">
          <thead>
            <tr>
              {sortableHeader("name", "Cookbook")}
              <th>Version</th>
              {sortableHeader("complexity_score", "Complexity")}
              {sortableHeader("affected_nodes", "Blast Radius")}
              {sortableHeader("priority_score", "Priority")}
              <th>Auto-correct</th>
              <th>Manual</th>
              <th>Deprecations</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item, idx) => (
              <PriorityRow key={`${item.cookbook_id}-${idx}`} item={item} />
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {pagination && (
        <Pagination pagination={pagination} onPageChange={onPageChange} />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Priority Table Row
// ---------------------------------------------------------------------------

function PriorityRow({ item }: { item: RemediationPriorityItem }) {
  // Determine if this is a "quick win" — only auto-correctable issues, no manual
  const isQuickWin = item.auto_correctable_count > 0 && item.manual_fix_count === 0;

  return (
    <tr className={isQuickWin ? "bg-green-50/40" : ""}>
      {/* Cookbook name — link to remediation detail page (or cookbook detail if no version) */}
      <td>
        <div className="flex items-center gap-2">
          <Link
            to={
              item.cookbook_version
                ? `/cookbooks/${encodeURIComponent(item.cookbook_name)}/${encodeURIComponent(item.cookbook_version)}/remediation`
                : `/cookbooks/${encodeURIComponent(item.cookbook_name)}`
            }
            className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
          >
            {item.cookbook_name}
          </Link>
          {isQuickWin && (
            <span
              className="inline-flex items-center rounded-full bg-green-100 px-1.5 py-0.5 text-[10px] font-medium text-green-700 ring-1 ring-inset ring-green-600/20"
              title="Only auto-correctable issues — can be fixed automatically"
            >
              Quick Win
            </span>
          )}
        </div>
      </td>

      {/* Version */}
      <td>
        {item.cookbook_version ? (
          <code className="rounded bg-gray-100 px-1.5 py-0.5 text-xs">
            {item.cookbook_version}
          </code>
        ) : (
          <span className="text-gray-400">—</span>
        )}
      </td>

      {/* Complexity badge */}
      <td>
        <ComplexityBadge
          complexityLabel={item.complexity_label}
          score={item.complexity_score}
          size="sm"
        />
      </td>

      {/* Blast radius — affected nodes & roles */}
      <td>
        <div className="flex flex-col">
          <span className="text-sm font-medium text-gray-800">
            {item.affected_node_count.toLocaleString()} node{item.affected_node_count !== 1 ? "s" : ""}
          </span>
          {item.affected_role_count > 0 && (
            <span className="text-xs text-gray-400">
              {item.affected_role_count} role{item.affected_role_count !== 1 ? "s" : ""}
            </span>
          )}
        </div>
      </td>

      {/* Priority score — displayed as a bar */}
      <td>
        <PriorityScoreBar score={item.priority_score} />
      </td>

      {/* Auto-correctable count */}
      <td>
        {item.auto_correctable_count > 0 ? (
          <span className="inline-flex items-center gap-1 text-sm font-medium text-green-700">
            <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor" aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
            </svg>
            {item.auto_correctable_count}
          </span>
        ) : (
          <span className="text-gray-400">0</span>
        )}
      </td>

      {/* Manual fix count */}
      <td>
        {item.manual_fix_count > 0 ? (
          <span className="text-sm font-medium text-amber-700">
            {item.manual_fix_count}
          </span>
        ) : (
          <span className="text-gray-400">0</span>
        )}
      </td>

      {/* Deprecation count */}
      <td>
        {item.deprecation_count > 0 ? (
          <span className="text-sm text-purple-700">
            {item.deprecation_count}
          </span>
        ) : (
          <span className="text-gray-400">0</span>
        )}
      </td>
    </tr>
  );
}

// ---------------------------------------------------------------------------
// Priority Score Bar — small inline bar to visualise relative priority
// ---------------------------------------------------------------------------

function PriorityScoreBar({ score }: { score: number }) {
  // Priority scores are unbounded; we normalise against a reasonable max
  // for visual display. Anything above the cap renders as a full bar.
  const cap = 500;
  const pct = Math.min((score / cap) * 100, 100);

  let barColor = "bg-gray-300";
  if (score >= 200) barColor = "bg-red-500";
  else if (score >= 50) barColor = "bg-amber-400";
  else if (score > 0) barColor = "bg-green-400";

  return (
    <div className="flex items-center gap-2">
      <div className="h-2 w-16 overflow-hidden rounded-full bg-gray-100">
        <div
          className={`h-full rounded-full transition-all duration-300 ${barColor}`}
          style={{ width: `${Math.max(pct, score > 0 ? 4 : 0)}%` }}
        />
      </div>
      <span className="text-xs font-medium text-gray-600">{score.toLocaleString()}</span>
    </div>
  );
}
