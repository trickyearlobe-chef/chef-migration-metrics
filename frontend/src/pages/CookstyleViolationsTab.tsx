// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { useGlobalFilters } from "../context/GlobalFilterContext";
import { useOrg } from "../context/OrgContext";
import {
  fetchCookstyleViolations,
  type CookstyleViolationsQuery,
} from "../api";
import type {
  CookstyleViolationsResponse,
  CookstyleViolationItem,
  Pagination as PaginationType,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { StatusBadge } from "../components/StatusBadge";

// ---------------------------------------------------------------------------
// Namespace and severity options for filter dropdowns
// ---------------------------------------------------------------------------

const NAMESPACE_OPTIONS = [
  { value: "Chef/Deprecations/", label: "Deprecations" },
  { value: "Chef/Correctness/", label: "Correctness" },
  { value: "Chef/Style/", label: "Style" },
  { value: "Chef/Modernize/", label: "Modernize" },
];

const SEVERITY_OPTIONS = [
  { value: "convention", label: "Convention" },
  { value: "refactor", label: "Refactor" },
  { value: "warning", label: "Warning" },
  { value: "error", label: "Error" },
  { value: "fatal", label: "Fatal" },
];

const STATUS_OPTIONS = [
  { value: "", label: "All" },
  { value: "failed", label: "Failed" },
  { value: "passed", label: "Passed" },
  { value: "error", label: "Scan Error" },
];

// ---------------------------------------------------------------------------
// CookstyleViolationsTab — filterable, paginated violations browser
// ---------------------------------------------------------------------------

export function CookstyleViolationsTab() {
  const { targetChefVersion: selectedVersion, versionsLoading, targetVersions } =
    useGlobalFilters();
  const { selectedOrg } = useOrg();
  const [searchParams, setSearchParams] = useSearchParams();

  // Source toggle
  const sourceParam = searchParams.get("source");
  const [source, setSource] = useState<"server" | "git">(
    sourceParam === "git" ? "git" : "server",
  );

  // Filters from URL
  const [namespace, setNamespace] = useState(
    searchParams.get("namespace") || "",
  );
  const [severity, setSeverity] = useState(
    searchParams.get("severity") || "",
  );
  const [cop, setCop] = useState(searchParams.get("cop") || "");
  const [status, setStatus] = useState(searchParams.get("status") || "");

  // Sort and pagination
  const [sortField, setSortField] = useState(
    searchParams.get("sort") || "name",
  );
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">(
    (searchParams.get("order") as "asc" | "desc") || "asc",
  );
  const [page, setPage] = useState(
    Math.max(1, parseInt(searchParams.get("page") || "1", 10)),
  );
  const perPage = DEFAULT_PAGE_SIZE;

  // Data state
  const [data, setData] = useState<CookstyleViolationsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Debounce timer for cop typeahead
  const [copDebounced, setCopDebounced] = useState(cop);
  useEffect(() => {
    const timer = setTimeout(() => setCopDebounced(cop), 300);
    return () => clearTimeout(timer);
  }, [cop]);

  // Sync filter state to URL params (preserving tab param)
  useEffect(() => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        // Preserve tab
        if (source !== "server") next.set("source", source);
        else next.delete("source");
        if (namespace) next.set("namespace", namespace);
        else next.delete("namespace");
        if (severity) next.set("severity", severity);
        else next.delete("severity");
        if (copDebounced) next.set("cop", copDebounced);
        else next.delete("cop");
        if (status) next.set("status", status);
        else next.delete("status");
        if (sortField !== "name") next.set("sort", sortField);
        else next.delete("sort");
        if (sortOrder !== "asc") next.set("order", sortOrder);
        else next.delete("order");
        if (page > 1) next.set("page", String(page));
        else next.delete("page");
        return next;
      },
      { replace: true },
    );
  }, [source, namespace, severity, copDebounced, status, sortField, sortOrder, page, setSearchParams]);

  // Reset page when filters change
  useEffect(() => {
    setPage(1);
  }, [source, namespace, severity, copDebounced, status, sortField, sortOrder, selectedVersion, selectedOrg]);

  // Fetch data
  const loadData = useCallback(() => {
    if (!selectedVersion && targetVersions.length === 0 && versionsLoading)
      return;

    setLoading(true);
    setError(null);

    const params: CookstyleViolationsQuery = {
      source,
      target_chef_version: selectedVersion || undefined,
      namespace: namespace || undefined,
      severity: severity || undefined,
      cop: copDebounced || undefined,
      status: status || undefined,
      sort: sortField,
      order: sortOrder,
      page,
      per_page: perPage,
    };

    fetchCookstyleViolations(params)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [
    source,
    selectedVersion,
    namespace,
    severity,
    copDebounced,
    status,
    sortField,
    sortOrder,
    page,
    perPage,
    targetVersions.length,
    versionsLoading,
  ]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Sort handler
  const handleSort = (field: string) => {
    if (sortField === field) {
      setSortOrder((prev) => (prev === "desc" ? "asc" : "desc"));
    } else {
      setSortField(field);
      setSortOrder(field === "name" ? "asc" : "desc");
    }
  };

  const sortIndicator = (field: string) => {
    if (sortField !== field) return null;
    return sortOrder === "desc" ? " ↓" : " ↑";
  };

  // No target versions configured
  if (!versionsLoading && targetVersions.length === 0) {
    return (
      <EmptyState
        title="No target versions configured"
        description="Configure target Chef versions in Admin → Target Versions to use the violations browser."
      />
    );
  }

  return (
    <div className="space-y-4">
      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3">
        {/* Source toggle */}
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">
            Source
          </label>
          <div className="inline-flex rounded-md shadow-sm">
            <button
              type="button"
              onClick={() => setSource("server")}
              className={
                "rounded-l-md border px-3 py-1.5 text-sm font-medium " +
                (source === "server"
                  ? "border-blue-500 bg-blue-50 text-blue-700 z-10"
                  : "border-gray-300 bg-white text-gray-700 hover:bg-gray-50")
              }
            >
              Server Cookbooks
            </button>
            <button
              type="button"
              onClick={() => setSource("git")}
              className={
                "-ml-px rounded-r-md border px-3 py-1.5 text-sm font-medium " +
                (source === "git"
                  ? "border-blue-500 bg-blue-50 text-blue-700 z-10"
                  : "border-gray-300 bg-white text-gray-700 hover:bg-gray-50")
              }
            >
              Git Repos
            </button>
          </div>
        </div>

        {/* Namespace filter */}
        <div>
          <label htmlFor="violations-namespace" className="mb-1 block text-xs font-medium text-gray-500">
            Namespace
          </label>
          <select
            id="violations-namespace"
            value={namespace}
            onChange={(e) => setNamespace(e.target.value)}
            className="block w-40 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">All</option>
            {NAMESPACE_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>

        {/* Severity filter */}
        <div>
          <label htmlFor="violations-severity" className="mb-1 block text-xs font-medium text-gray-500">
            Severity
          </label>
          <select
            id="violations-severity"
            value={severity}
            onChange={(e) => setSeverity(e.target.value)}
            className="block w-32 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">All</option>
            {SEVERITY_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>

        {/* Cop name typeahead */}
        <div>
          <label htmlFor="violations-cop" className="mb-1 block text-xs font-medium text-gray-500">
            Cop Name
          </label>
          <input
            id="violations-cop"
            type="text"
            value={cop}
            onChange={(e) => setCop(e.target.value)}
            placeholder="e.g. Chef/Deprecations/"
            className="block w-52 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

        {/* Status filter */}
        <div>
          <label htmlFor="violations-status" className="mb-1 block text-xs font-medium text-gray-500">
            Status
          </label>
          <select
            id="violations-status"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="block w-28 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            {STATUS_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Results */}
      <ViolationsTable
        data={data}
        loading={loading}
        error={error}
        onRetry={loadData}
        source={source}
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
// ViolationsTable
// ---------------------------------------------------------------------------

function ViolationsTable({
  data,
  loading,
  error,
  onRetry,
  source,
  onSort,
  sortField,
  sortIndicator,
  page,
  onPageChange,
}: {
  data: CookstyleViolationsResponse | null;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  source: "server" | "git";
  onSort: (field: string) => void;
  sortField: string;
  sortIndicator: (field: string) => string | null;
  page: number;
  onPageChange: (page: number) => void;
}) {
  if (loading) return <LoadingSpinner message="Loading violations…" />;
  if (error) return <ErrorAlert message={error} onRetry={onRetry} />;
  if (!data) return null;

  const items: CookstyleViolationItem[] = data.data ?? [];
  const pagination: PaginationType | undefined = data.pagination;

  if (items.length === 0 && page === 1) {
    return (
      <EmptyState
        title="No violations match the current filters"
        description="Try adjusting the filters or selecting a different target Chef version."
      />
    );
  }

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
      <div className="table-container">
        <table className="table">
          <thead>
            <tr>
              {sortableHeader("name", source === "server" ? "Cookbook" : "Repo")}
              <th>{source === "server" ? "Version" : "Commit"}</th>
              {source === "server" && <th>Organisation</th>}
              <th>Status</th>
              {sortableHeader("offence_count", "Offences")}
              {sortableHeader("deprecation_count", "Deprecations")}
              <th>Top Cops</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item, idx) => (
              <ViolationRow key={`${item.name}-${item.version}-${idx}`} item={item} source={source} />
            ))}
          </tbody>
        </table>
      </div>

      {pagination && (
        <Pagination pagination={pagination} onPageChange={onPageChange} />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// ViolationRow
// ---------------------------------------------------------------------------

function ViolationRow({
  item,
  source,
}: {
  item: CookstyleViolationItem;
  source: "server" | "git";
}) {
  // Determine status badge
  const statusVariant = item.error_message
    ? "scan_error"
    : item.passed
      ? "cs_compatible"
      : "cs_incompatible";
  const statusLabel = item.error_message
    ? "Error"
    : item.passed
      ? "Passed"
      : "Failed";

  // Build link to detail/remediation page
  const detailLink =
    source === "server"
      ? `/cookbooks/${encodeURIComponent(item.name)}/${encodeURIComponent(item.version)}/remediation`
      : `/git-repos/${encodeURIComponent(item.name)}/${encodeURIComponent(item.version)}/remediation`;

  // Truncate top cops display
  const topCops = item.top_cops ?? [];
  const displayCops = topCops.slice(0, 3);
  const extraCops = topCops.length - displayCops.length;

  return (
    <tr>
      {/* Name with link */}
      <td>
        <Link
          to={detailLink}
          className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
        >
          {item.name}
        </Link>
      </td>

      {/* Version / Commit */}
      <td className="font-mono text-xs">
        {source === "git" && item.version.length > 8
          ? item.version.slice(0, 8)
          : item.version}
      </td>

      {/* Organisation (server only) */}
      {source === "server" && (
        <td className="text-gray-600">{item.organisation || "—"}</td>
      )}

      {/* Status badge */}
      <td>
        <StatusBadge variant={statusVariant} label={statusLabel} size="sm" />
      </td>

      {/* Offence count */}
      <td className="text-right tabular-nums">
        {item.offence_count.toLocaleString()}
      </td>

      {/* Deprecation count */}
      <td className="text-right tabular-nums">
        {item.deprecation_count.toLocaleString()}
      </td>

      {/* Top cops */}
      <td className="max-w-xs truncate text-xs text-gray-600">
        {displayCops.length > 0 ? (
          <span title={topCops.join(", ")}>
            {displayCops.join(", ")}
            {extraCops > 0 && (
              <span className="ml-1 text-gray-400">+{extraCops} more</span>
            )}
          </span>
        ) : (
          <span className="text-gray-400">—</span>
        )}
      </td>
    </tr>
  );
}
