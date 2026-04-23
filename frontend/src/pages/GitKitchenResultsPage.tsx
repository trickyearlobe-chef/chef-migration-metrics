// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import { listGitKitchenResults } from "../api";
import type { GitKitchenResult } from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

function resultStatus(r: GitKitchenResult): string {
  if (r.error_message) return "errored";
  if (r.timed_out) return "timed out";
  if (r.converge_passed === null) return "pending";
  if (r.converge_passed && r.tests_passed) return "passed";
  return "failed";
}

const RESULT_STATUS_COLORS: Record<string, string> = {
  passed: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
  "timed out": "bg-yellow-100 text-yellow-800",
  errored: "bg-orange-100 text-orange-800",
  pending: "bg-gray-100 text-gray-500",
};

const STATUS_OPTIONS = [
  "all",
  "passed",
  "failed",
  "timed out",
  "errored",
  "pending",
] as const;

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDate(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}

function shortSha(sha: string): string {
  return sha.slice(0, 8);
}

// ---------------------------------------------------------------------------
// Summary cards
// ---------------------------------------------------------------------------

function SummaryCards({ results }: { results: GitKitchenResult[] }) {
  const counts = {
    total: results.length,
    passed: 0,
    failed: 0,
    pending: 0,
    "timed out": 0,
    errored: 0,
  };
  for (const r of results) {
    const s = resultStatus(r);
    if (s in counts) counts[s as keyof typeof counts]++;
  }

  const cards: { label: string; value: number; color: string }[] = [
    { label: "Total", value: counts.total, color: "text-gray-800" },
    { label: "Passed", value: counts.passed, color: "text-green-700" },
    { label: "Failed", value: counts.failed, color: "text-red-700" },
    { label: "Pending", value: counts.pending, color: "text-gray-500" },
    {
      label: "Timed Out",
      value: counts["timed out"],
      color: "text-yellow-700",
    },
    { label: "Errored", value: counts.errored, color: "text-orange-700" },
  ];

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
      {cards.map((c) => (
        <div
          key={c.label}
          className="rounded-lg border border-gray-200 bg-white p-3 shadow-sm"
        >
          <p className="text-xs font-medium uppercase tracking-wider text-gray-500">
            {c.label}
          </p>
          <p className={`mt-1 text-2xl font-bold ${c.color}`}>{c.value}</p>
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Results table
// ---------------------------------------------------------------------------

function ResultsTable({ results }: { results: GitKitchenResult[] }) {
  if (results.length === 0) {
    return (
      <p className="text-sm text-gray-500">
        No results match the current filters.
      </p>
    );
  }
  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 shadow-sm">
      <table className="min-w-full text-sm">
        <thead>
          <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
            <th className="px-4 py-2">Cookbook</th>
            <th className="px-4 py-2">Platform</th>
            <th className="px-4 py-2">Suite</th>
            <th className="px-4 py-2">Chef Version</th>
            <th className="px-4 py-2">Commit</th>
            <th className="px-4 py-2">Status</th>
            <th className="px-4 py-2 text-right">Duration</th>
            <th className="px-4 py-2">Completed At</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {results.map((r) => {
            const status = resultStatus(r);
            const colorCls =
              RESULT_STATUS_COLORS[status] || "bg-gray-100 text-gray-500";
            return (
              <tr key={r.id} className="hover:bg-gray-50">
                <td className="whitespace-nowrap px-4 py-2 font-medium text-gray-800">
                  {r.git_repo_name}
                </td>
                <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                  {r.platform_name}
                </td>
                <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                  {r.suite_name}
                </td>
                <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                  {r.target_chef_version}
                </td>
                <td className="whitespace-nowrap px-4 py-2 font-mono text-xs text-gray-600">
                  {shortSha(r.commit_sha)}
                </td>
                <td className="whitespace-nowrap px-4 py-2">
                  <span
                    className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colorCls}`}
                  >
                    {status}
                  </span>
                </td>
                <td className="whitespace-nowrap px-4 py-2 text-right tabular-nums text-gray-600">
                  {r.duration_seconds != null ? `${r.duration_seconds}s` : "—"}
                </td>
                <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                  {formatDate(r.completed_at)}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Platform Matrix helpers
// ---------------------------------------------------------------------------

function statusPriority(status: string): number {
  switch (status) {
    case "errored":
      return 5;
    case "failed":
      return 4;
    case "timed out":
      return 3;
    case "pending":
      return 2;
    case "passed":
      return 1;
    default:
      return 0;
  }
}

function PlatformMatrix({ results }: { results: GitKitchenResult[] }) {
  const cookbooks = [...new Set(results.map((r) => r.git_repo_name))].sort();
  const platforms = [...new Set(results.map((r) => r.platform_name))].sort();

  if (cookbooks.length === 0 || platforms.length === 0) {
    return <p className="text-sm text-gray-500">No data for matrix view.</p>;
  }

  const lookup = new Map<string, string>();
  for (const r of results) {
    const key = `${r.git_repo_name}|${r.platform_name}`;
    const existing = lookup.get(key);
    const status = resultStatus(r);
    if (!existing || statusPriority(status) > statusPriority(existing)) {
      lookup.set(key, status);
    }
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 shadow-sm">
      <table className="min-w-full text-xs">
        <thead>
          <tr className="border-b border-gray-100 bg-gray-50">
            <th className="sticky left-0 z-10 bg-gray-50 px-3 py-2 text-left font-medium text-gray-500">
              Cookbook
            </th>
            {platforms.map((p) => (
              <th
                key={p}
                className="px-2 py-2 text-center font-medium text-gray-500"
                title={p}
              >
                {p.length > 15 ? p.slice(0, 12) + "…" : p}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {cookbooks.map((cb) => (
            <tr key={cb} className="hover:bg-gray-50">
              <td className="sticky left-0 z-10 bg-white whitespace-nowrap px-3 py-1.5 font-medium text-gray-700">
                {cb}
              </td>
              {platforms.map((p) => {
                const status = lookup.get(`${cb}|${p}`);
                let cellCls = "bg-gray-50 text-gray-300";
                let label = "—";
                if (status === "passed") {
                  cellCls = "bg-green-100 text-green-700";
                  label = "✓";
                } else if (status === "failed") {
                  cellCls = "bg-red-100 text-red-700";
                  label = "✗";
                } else if (status === "timed out") {
                  cellCls = "bg-yellow-100 text-yellow-700";
                  label = "⏱";
                } else if (status === "errored") {
                  cellCls = "bg-orange-100 text-orange-700";
                  label = "!";
                } else if (status === "pending") {
                  cellCls = "bg-blue-50 text-blue-400";
                  label = "…";
                }
                return (
                  <td
                    key={p}
                    className={`px-2 py-1.5 text-center ${cellCls}`}
                    title={`${cb} / ${p}: ${status || "untested"}`}
                  >
                    {label}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export function GitKitchenResultsPage() {
  const [results, setResults] = useState<GitKitchenResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [view, setView] = useState<"table" | "matrix">("table");
  const [cookbookFilter, setCookbookFilter] = useState("");
  const [platformFilter, setPlatformFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");

  const loadResults = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await listGitKitchenResults();
      setResults(data);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to load results");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadResults();
  }, [loadResults]);

  const filtered = results.filter((r) => {
    if (
      cookbookFilter &&
      !r.git_repo_name.toLowerCase().includes(cookbookFilter.toLowerCase())
    )
      return false;
    if (
      platformFilter &&
      !r.platform_name.toLowerCase().includes(platformFilter.toLowerCase())
    )
      return false;
    if (statusFilter !== "all" && resultStatus(r) !== statusFilter)
      return false;
    return true;
  });

  if (loading) return <LoadingSpinner message="Loading git kitchen results…" />;
  if (error) return <ErrorAlert message={error} onRetry={loadResults} />;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-bold text-gray-800">Git Kitchen Results</h2>
        <p className="text-sm text-gray-500">
          Per-instance Test Kitchen results across all batches.
        </p>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <div className="min-w-[180px] flex-1">
          <label className="mb-1 block text-xs font-medium text-gray-600">
            Cookbook
          </label>
          <input
            type="text"
            placeholder="Filter by cookbook name"
            className={INPUT_CLASS}
            value={cookbookFilter}
            onChange={(e) => setCookbookFilter(e.target.value)}
          />
        </div>
        <div className="min-w-[180px] flex-1">
          <label className="mb-1 block text-xs font-medium text-gray-600">
            Platform
          </label>
          <input
            type="text"
            placeholder="Filter by platform"
            className={INPUT_CLASS}
            value={platformFilter}
            onChange={(e) => setPlatformFilter(e.target.value)}
          />
        </div>
        <div className="min-w-[140px]">
          <label className="mb-1 block text-xs font-medium text-gray-600">
            Status
          </label>
          <select
            className={INPUT_CLASS}
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
          >
            {STATUS_OPTIONS.map((opt) => (
              <option key={opt} value={opt}>
                {opt === "all"
                  ? "All"
                  : opt.charAt(0).toUpperCase() + opt.slice(1)}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* View toggle */}
      <div className="flex items-center gap-2">
        <button
          className={`rounded px-3 py-1.5 text-sm font-medium ${view === "table" ? "bg-blue-100 text-blue-700" : "bg-gray-100 text-gray-600 hover:bg-gray-200"}`}
          onClick={() => setView("table")}
        >
          Table
        </button>
        <button
          className={`rounded px-3 py-1.5 text-sm font-medium ${view === "matrix" ? "bg-blue-100 text-blue-700" : "bg-gray-100 text-gray-600 hover:bg-gray-200"}`}
          onClick={() => setView("matrix")}
        >
          Platform Matrix
        </button>
      </div>

      {/* Summary stats */}
      <SummaryCards results={filtered} />

      {/* Results view */}
      {view === "table" ? (
        <ResultsTable results={filtered} />
      ) : (
        <PlatformMatrix results={filtered} />
      )}
    </div>
  );
}

export default GitKitchenResultsPage;
