// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { FilterSelect, FilterInput } from "../components/FilterInputs";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface KitchenAnalysisSummary {
  total_scanned: number;
  total_without_kitchen: number;
  total_with_local_override: number;
  total_with_conflicts: number;
  driver_counts: Record<string, number>;
  transport_counts: Record<string, number>;
  provisioner_counts: Record<string, number>;
  platform_count: number;
}

interface KitchenDiscoveredPlatform {
  platform_name: string;
  normalised_name: string;
  os_family: string;
  os_version: string;
  cookbook_count: number;
  has_extensions: boolean;
  common_extensions: unknown;
  transport_type: string;
  updated_at: string;
}

interface KitchenAnalysisResult {
  git_repo_name: string;
  git_repo_url: string;
  analysed_at: string;
  head_commit_sha: string;
  kitchen_files: string[];
  has_local_override: boolean;
  local_override_keys: string[];
  driver_name: string;
  provisioner_name: string;
  require_chef_omnibus: boolean | null;
  platforms: unknown[];
  suites: unknown[];
  transport_type: string;
  extensions: unknown;
  variant_files: string[];
  error_message: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type SortField =
  | "platform_name"
  | "normalised_name"
  | "os_family"
  | "os_version"
  | "cookbook_count"
  | "transport_type";

const OS_FAMILY_OPTIONS = [
  { value: "", label: "All" },
  { value: "rhel", label: "RHEL" },
  { value: "windows", label: "Windows" },
  { value: "debian", label: "Debian" },
  { value: "suse", label: "SUSE" },
  { value: "other", label: "Other" },
];

const OS_BADGE_COLORS: Record<string, string> = {
  rhel: "bg-red-100 text-red-700",
  windows: "bg-blue-100 text-blue-700",
  debian: "bg-orange-100 text-orange-700",
  suse: "bg-green-100 text-green-700",
};

function osBadgeClass(family: string): string {
  return OS_BADGE_COLORS[family.toLowerCase()] ?? "bg-gray-100 text-gray-600";
}

function sortPlatforms(
  data: KitchenDiscoveredPlatform[],
  field: SortField,
  dir: "asc" | "desc",
): KitchenDiscoveredPlatform[] {
  const sorted = [...data].sort((a, b) => {
    const av = a[field];
    const bv = b[field];
    if (typeof av === "number" && typeof bv === "number") return av - bv;
    return String(av).localeCompare(String(bv));
  });
  return dir === "desc" ? sorted.reverse() : sorted;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AdminKitchenAnalysisPage() {
  const [summary, setSummary] = useState<KitchenAnalysisSummary | null>(null);
  const [platforms, setPlatforms] = useState<KitchenDiscoveredPlatform[]>([]);
  const [conflicts, setConflicts] = useState<KitchenAnalysisResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [osFamily, setOsFamily] = useState("");
  const [minCount, setMinCount] = useState("");
  const [sortField, setSortField] = useState<SortField>("cookbook_count");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");

  // Trigger analysis
  const [triggering, setTriggering] = useState(false);
  const [triggerMsg, setTriggerMsg] = useState<string | null>(null);

  // Fetch summary on mount
  useEffect(() => {
    setLoading(true);
    fetch("/api/v1/kitchen/analysis/summary")
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json();
      })
      .then(setSummary)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  // Fetch platforms when filters change
  useEffect(() => {
    const params = new URLSearchParams();
    if (osFamily) params.set("os_family", osFamily);
    const mc = parseInt(minCount, 10);
    if (mc > 0) params.set("min_count", String(mc));
    const qs = params.toString();
    fetch(`/api/v1/kitchen/analysis/platforms${qs ? "?" + qs : ""}`)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json();
      })
      .then(setPlatforms)
      .catch((e) => setError(e.message));
  }, [osFamily, minCount]);

  // Fetch conflicts when summary is available and has conflicts
  useEffect(() => {
    if (summary && summary.total_with_conflicts > 0) {
      fetch("/api/v1/kitchen/analysis/cookbooks?has_local_override=true")
        .then((r) => {
          if (!r.ok) throw new Error(`HTTP ${r.status}`);
          return r.json();
        })
        .then(setConflicts)
        .catch((e) => setError(e.message));
    }
  }, [summary]);

  const handleSort = useCallback(
    (field: SortField) => {
      if (field === sortField) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
      } else {
        setSortField(field);
        setSortDir(field === "cookbook_count" ? "desc" : "asc");
      }
    },
    [sortField],
  );

  const handleTrigger = useCallback(async () => {
    setTriggering(true);
    setTriggerMsg(null);
    try {
      const r = await fetch("/api/v1/kitchen/analysis/trigger", {
        method: "POST",
      });
      const body = await r.json();
      setTriggerMsg(body.message || body.status || "Triggered");
    } catch (e: unknown) {
      setTriggerMsg(e instanceof Error ? e.message : "Failed to trigger");
    } finally {
      setTriggering(false);
    }
  }, []);

  const sortedPlatforms = sortPlatforms(platforms, sortField, sortDir);

  const sortIndicator = (field: SortField) =>
    sortField === field ? (sortDir === "asc" ? " ▲" : " ▼") : "";

  if (loading) return <LoadingSpinner message="Loading kitchen analysis…" />;
  if (error && !summary)
    return (
      <ErrorAlert message="Failed to load kitchen analysis" detail={error} />
    );

  // Driver chips from summary
  const driverChips = summary
    ? Object.entries(summary.driver_counts).sort(([, a], [, b]) => b - a)
    : [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-xl font-semibold text-gray-800">
            Kitchen Config Analysis
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Discovered Test Kitchen configurations across all git repositories.
          </p>
        </div>
        <button
          onClick={handleTrigger}
          disabled={triggering}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50"
        >
          {triggering ? "Triggering…" : "Re-analyse"}
        </button>
      </div>

      {triggerMsg && (
        <div className="rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-800">
          {triggerMsg}
        </div>
      )}

      {error && <ErrorAlert message="Error" detail={error} />}

      {/* Summary Cards */}
      {summary && (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
          <SummaryCard
            label="Cookbooks with TK"
            value={summary.total_scanned}
            color="green"
          />
          <SummaryCard
            label="Without TK"
            value={summary.total_without_kitchen}
            color="gray"
          />
          <SummaryCard
            label="Unique Platforms"
            value={summary.platform_count}
            color="blue"
          />
          <SummaryCard
            label="Local Override Conflicts"
            value={summary.total_with_conflicts}
            color={summary.total_with_conflicts > 0 ? "red" : "gray"}
          />
          <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
            <div className="text-xs font-medium uppercase tracking-wider text-gray-400">
              Driver Breakdown
            </div>
            <div className="mt-2 flex flex-wrap gap-1.5">
              {driverChips.length === 0 && (
                <span className="text-sm text-gray-400">—</span>
              )}
              {driverChips.map(([driver, count]) => (
                <span
                  key={driver}
                  className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700"
                >
                  {driver}: {count}
                </span>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Platform Discovery Table */}
      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="flex flex-wrap items-end gap-4 border-b border-gray-200 px-4 py-3">
          <h3 className="text-sm font-semibold text-gray-700">
            Platform Discovery
          </h3>
          <div className="ml-auto flex flex-wrap items-end gap-3">
            <FilterSelect
              label="OS Family"
              value={osFamily}
              onChange={setOsFamily}
              options={OS_FAMILY_OPTIONS}
            />
            <FilterInput
              label="Min Count"
              value={minCount}
              onChange={setMinCount}
              placeholder="0"
            />
          </div>
        </div>

        {sortedPlatforms.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-gray-400">
            No platform data available.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  <SortableHeader
                    label="Platform Name"
                    field="platform_name"
                    onClick={handleSort}
                    indicator={sortIndicator}
                  />
                  <SortableHeader
                    label="Normalised"
                    field="normalised_name"
                    onClick={handleSort}
                    indicator={sortIndicator}
                  />
                  <SortableHeader
                    label="OS Family"
                    field="os_family"
                    onClick={handleSort}
                    indicator={sortIndicator}
                  />
                  <SortableHeader
                    label="Version"
                    field="os_version"
                    onClick={handleSort}
                    indicator={sortIndicator}
                  />
                  <SortableHeader
                    label="Cookbooks"
                    field="cookbook_count"
                    onClick={handleSort}
                    indicator={sortIndicator}
                  />
                  <SortableHeader
                    label="Transport"
                    field="transport_type"
                    onClick={handleSort}
                    indicator={sortIndicator}
                  />
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {sortedPlatforms.map((p, i) => (
                  <tr
                    key={`${p.platform_name}-${p.os_version}-${i}`}
                    className="hover:bg-gray-50"
                  >
                    <td className="whitespace-nowrap px-4 py-2 font-medium text-gray-800">
                      {p.platform_name}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                      {p.normalised_name}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2">
                      <span
                        className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${osBadgeClass(p.os_family)}`}
                      >
                        {p.os_family}
                      </span>
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                      {p.os_version || "—"}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-right tabular-nums text-gray-800">
                      {p.cookbook_count}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                      {p.transport_type || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Conflict List */}
      {summary && summary.total_with_conflicts > 0 && (
        <div className="rounded-lg border border-red-200 bg-white shadow-sm">
          <div className="border-b border-red-200 bg-red-50 px-4 py-3">
            <h3 className="text-sm font-semibold text-red-800">
              ⚠️ Local Override Conflicts ({summary.total_with_conflicts})
            </h3>
          </div>
          {conflicts.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-gray-400">
              Loading conflicts…
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                    <th className="px-4 py-2">Cookbook</th>
                    <th className="px-4 py-2">Override Keys</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {conflicts.map((c) => (
                    <tr key={c.git_repo_name} className="hover:bg-gray-50">
                      <td className="whitespace-nowrap px-4 py-2">
                        <Link
                          to={`/git-repos/${encodeURIComponent(c.git_repo_name)}`}
                          className="font-medium text-blue-600 hover:underline"
                        >
                          {c.git_repo_name}
                        </Link>
                      </td>
                      <td className="px-4 py-2 text-gray-600">
                        {c.local_override_keys.length > 0
                          ? c.local_override_keys.join(", ")
                          : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

const CARD_COLORS: Record<string, string> = {
  green: "text-green-700",
  gray: "text-gray-500",
  blue: "text-blue-700",
  red: "text-red-700",
};

function SummaryCard({
  label,
  value,
  color,
}: {
  label: string;
  value: number;
  color: string;
}) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
      <div className="text-xs font-medium uppercase tracking-wider text-gray-400">
        {label}
      </div>
      <div
        className={`mt-1 text-2xl font-bold tabular-nums ${CARD_COLORS[color] ?? "text-gray-800"}`}
      >
        {value.toLocaleString()}
      </div>
    </div>
  );
}

function SortableHeader({
  label,
  field,
  onClick,
  indicator,
}: {
  label: string;
  field: SortField;
  onClick: (f: SortField) => void;
  indicator: (f: SortField) => string;
}) {
  return (
    <th
      className="cursor-pointer select-none px-4 py-2 hover:text-gray-700"
      onClick={() => onClick(field)}
    >
      {label}
      {indicator(field)}
    </th>
  );
}
