// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { useGlobalFilters } from "../context/GlobalFilterContext";
import { useIsAdmin } from "../context/AuthContext";
import {
  fetchCookstyleCops,
  fetchCookstyleCopCookbooks,
  setCopClassification,
  type CopAggregationQuery,
} from "../api";
import type {
  CopAggregateItem,
  CopAggregationSummary,
  CopCookbookItem,
  Pagination as PaginationType,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { ClassificationBadge, CLASSIFICATION_FILTERS } from "../components/ClassificationBadge";

// ---------------------------------------------------------------------------
// CopAnalysisTab — classification-aware cop aggregation view
// ---------------------------------------------------------------------------

export function CopAnalysisTab() {
  const { targetChefVersion } = useGlobalFilters();
  const isAdmin = useIsAdmin();
  const [searchParams, setSearchParams] = useSearchParams();

  // State
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [items, setItems] = useState<CopAggregateItem[]>([]);
  const [summary, setSummary] = useState<CopAggregationSummary | null>(null);
  const [pagination, setPagination] = useState<PaginationType | null>(null);

  // Drill-down state
  const [drillCop, setDrillCop] = useState<string | null>(null);
  const [drillItems, setDrillItems] = useState<CopCookbookItem[]>([]);
  const [drillLoading, setDrillLoading] = useState(false);

  // Reclassification state
  const [reclassifyCop, setReclassifyCop] = useState<string | null>(null);
  const [reclassifyValue, setReclassifyValue] = useState<string>("blocker");
  const [reclassifyReason, setReclassifyReason] = useState("");
  const [reclassifySaving, setReclassifySaving] = useState(false);

  // Filters from URL params
  const classFilter = searchParams.get("classification") ?? "";
  const source = searchParams.get("source") ?? "";
  const sortField = searchParams.get("sort") ?? "cookbooks_affected";
  const sortOrder = searchParams.get("order") ?? "desc";
  const page = parseInt(searchParams.get("page") ?? "1", 10);

  const setParam = useCallback(
    (key: string, value: string) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        if (value) {
          next.set(key, value);
        } else {
          next.delete(key);
        }
        if (key !== "page") next.delete("page");
        return next;
      });
    },
    [setSearchParams],
  );

  // Fetch cop data
  const loadData = useCallback(async () => {
    if (!targetChefVersion) return;
    setLoading(true);
    setError(null);
    try {
      const params: CopAggregationQuery = {
        target_chef_version: targetChefVersion,
        page,
        per_page: DEFAULT_PAGE_SIZE,
        sort: sortField,
        order: sortOrder,
      };
      if (classFilter) params.classification = classFilter;
      if (source === "server" || source === "git") params.source = source;

      const resp = await fetchCookstyleCops(params);
      setItems(resp.data ?? []);
      setSummary(resp.summary);
      setPagination(resp.pagination);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load cop data");
    } finally {
      setLoading(false);
    }
  }, [targetChefVersion, classFilter, source, sortField, sortOrder, page]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Drill-down handler
  const openDrillDown = async (copName: string) => {
    if (drillCop === copName) {
      setDrillCop(null);
      return;
    }
    setDrillCop(copName);
    setDrillLoading(true);
    try {
      const resp = await fetchCookstyleCopCookbooks(copName, {
        target_chef_version: targetChefVersion,
        source: source || undefined,
        per_page: 20,
      });
      setDrillItems(resp.data ?? []);
    } catch {
      setDrillItems([]);
    } finally {
      setDrillLoading(false);
    }
  };

  // Reclassification handler
  const handleReclassify = async () => {
    if (!reclassifyCop || !targetChefVersion) return;
    setReclassifySaving(true);
    try {
      await setCopClassification(reclassifyCop, {
        target_chef_version: targetChefVersion,
        classification: reclassifyValue,
        reason: reclassifyReason,
      });
      setReclassifyCop(null);
      setReclassifyReason("");
      loadData(); // refresh
    } catch {
      // silently fail for now
    } finally {
      setReclassifySaving(false);
    }
  };

  // Sort handler
  const toggleSort = (field: string) => {
    if (sortField === field) {
      setParam("order", sortOrder === "desc" ? "asc" : "desc");
    } else {
      setParam("sort", field);
      setParam("order", "desc");
    }
  };

  const sortIndicator = (field: string) => {
    if (sortField !== field) return "";
    return sortOrder === "desc" ? " ↓" : " ↑";
  };

  if (!targetChefVersion) {
    return <EmptyState title="No target Chef version configured." />;
  }

  return (
    <div className="space-y-4">
      {/* Summary cards */}
      {summary && <SummaryCards summary={summary} />}

      {/* Filters row */}
      <div className="flex flex-wrap items-center gap-3">
        {/* Classification filter */}
        <div className="flex gap-1">
          {CLASSIFICATION_FILTERS.map((f) => (
            <button
              key={f.value}
              onClick={() => setParam("classification", f.value)}
              className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
                classFilter === f.value
                  ? f.colour + " ring-2 ring-offset-1 ring-blue-400"
                  : "bg-white text-gray-600 border border-gray-200 hover:bg-gray-50"
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>

        {/* Source filter */}
        <select
          className="rounded border border-gray-300 px-2 py-1 text-sm"
          value={source}
          onChange={(e) => setParam("source", e.target.value)}
        >
          <option value="">All sources</option>
          <option value="server">Server</option>
          <option value="git">Git</option>
        </select>
      </div>

      {/* Main content */}
      {loading && <LoadingSpinner />}
      {error && <ErrorAlert message={error} />}
      {!loading && !error && items.length === 0 && (
        <EmptyState title="No cops found for the current filters." />
      )}

      {!loading && !error && items.length > 0 && (
        <>
          <div className="overflow-x-auto rounded border border-gray-200">
            <table className="min-w-full divide-y divide-gray-200 text-sm">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-3 py-2 text-left font-medium text-gray-600">
                    Cop
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-gray-600">
                    Classification
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-gray-600">
                    RemovedIn
                  </th>
                  <th
                    className="px-3 py-2 text-right font-medium text-gray-600 cursor-pointer select-none"
                    onClick={() => toggleSort("cookbooks_affected")}
                  >
                    Cookbooks{sortIndicator("cookbooks_affected")}
                  </th>
                  <th
                    className="px-3 py-2 text-right font-medium text-gray-600 cursor-pointer select-none"
                    onClick={() => toggleSort("total_offences")}
                  >
                    Offences{sortIndicator("total_offences")}
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-gray-600">
                    Auto-fix
                  </th>
                  <th
                    className="px-3 py-2 text-right font-medium text-gray-600 cursor-pointer select-none"
                    onClick={() => toggleSort("unblocks")}
                  >
                    Unblocks{sortIndicator("unblocks")}
                  </th>
                  {isAdmin && (
                    <th className="px-3 py-2 text-center font-medium text-gray-600">
                      Actions
                    </th>
                  )}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {items.map((cop) => (
                  <CopRow
                    key={cop.cop_name}
                    cop={cop}
                    isExpanded={drillCop === cop.cop_name}
                    drillItems={drillCop === cop.cop_name ? drillItems : []}
                    drillLoading={drillCop === cop.cop_name && drillLoading}
                    canReclassify={isAdmin}
                    onExpand={() => openDrillDown(cop.cop_name)}
                    onReclassify={() => {
                      setReclassifyCop(cop.cop_name);
                      setReclassifyValue(cop.classification === "unclassified" ? "blocker" : cop.classification);
                    }}
                  />
                ))}
              </tbody>
            </table>
          </div>

          {pagination && (
            <Pagination
              pagination={pagination}
              onPageChange={(p) => setParam("page", String(p))}
            />
          )}
        </>
      )}

      {/* Reclassification modal */}
      {reclassifyCop && (
        <ReclassifyModal
          copName={reclassifyCop}
          value={reclassifyValue}
          reason={reclassifyReason}
          saving={reclassifySaving}
          onValueChange={setReclassifyValue}
          onReasonChange={setReclassifyReason}
          onSave={handleReclassify}
          onCancel={() => setReclassifyCop(null)}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Summary cards
// ---------------------------------------------------------------------------

function SummaryCards({ summary }: { summary: CopAggregationSummary }) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <SummaryCard
        label="Blocker cops"
        value={summary.blocker_cops}
        subLabel={`${summary.blocker_cookbooks} cookbooks`}
        colour="text-red-700 bg-red-50 border-red-200"
      />
      <SummaryCard
        label="Review cops"
        value={summary.review_cops}
        subLabel={`${summary.review_cookbooks} cookbooks`}
        colour="text-amber-700 bg-amber-50 border-amber-200"
      />
      <SummaryCard
        label="Noise cops"
        value={summary.noise_cops}
        colour="text-gray-500 bg-gray-50 border-gray-200"
      />
      <SummaryCard
        label="Unclassified"
        value={summary.unclassified_cops}
        colour="text-blue-600 bg-blue-50 border-blue-200"
      />
    </div>
  );
}

function SummaryCard({
  label,
  value,
  subLabel,
  colour,
}: {
  label: string;
  value: number;
  subLabel?: string;
  colour: string;
}) {
  return (
    <div className={`rounded-lg border p-3 ${colour}`}>
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-xs font-medium">{label}</div>
      {subLabel && <div className="mt-0.5 text-xs opacity-75">{subLabel}</div>}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Cop row with expandable drill-down
// ---------------------------------------------------------------------------

function CopRow({
  cop,
  isExpanded,
  drillItems,
  drillLoading,
  canReclassify,
  onExpand,
  onReclassify,
}: {
  cop: CopAggregateItem;
  isExpanded: boolean;
  drillItems: CopCookbookItem[];
  drillLoading: boolean;
  canReclassify: boolean;
  onExpand: () => void;
  onReclassify: () => void;
}) {
  return (
    <>
      <tr className="hover:bg-gray-50">
        <td className="px-3 py-2">
          <button
            onClick={onExpand}
            className="text-left font-mono text-xs text-blue-700 hover:underline"
            title={cop.description || cop.cop_name}
          >
            {cop.cop_name}
          </button>
          {cop.description && (
            <div className="mt-0.5 text-xs text-gray-400 truncate max-w-xs">
              {cop.description}
            </div>
          )}
        </td>
        <td className="px-3 py-2">
          <ClassificationBadge
            classification={cop.classification}
            source={cop.classification_source}
          />
        </td>
        <td className="px-3 py-2 text-xs text-gray-600">
          {cop.removed_in ? (
            <span className="font-mono text-red-600">Chef {cop.removed_in}</span>
          ) : (
            <span className="text-gray-300">—</span>
          )}
        </td>
        <td className="px-3 py-2 text-right tabular-nums">{cop.cookbooks_affected}</td>
        <td className="px-3 py-2 text-right tabular-nums">{cop.total_offences}</td>
        <td className="px-3 py-2 text-right tabular-nums">
          {cop.auto_correctable_pct > 0 ? (
            <span className="text-green-700">{Math.round(cop.auto_correctable_pct)}%</span>
          ) : (
            <span className="text-gray-300">—</span>
          )}
        </td>
        <td className="px-3 py-2 text-right tabular-nums">
          {cop.unblocks > 0 ? (
            <span className="font-medium text-emerald-700">{cop.unblocks}</span>
          ) : (
            <span className="text-gray-300">—</span>
          )}
        </td>
        {canReclassify && (
          <td className="px-3 py-2 text-center">
            <button
              onClick={onReclassify}
              className="rounded px-2 py-0.5 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700"
              title="Change classification"
            >
              ✏️
            </button>
          </td>
        )}
      </tr>

      {/* Drill-down row */}
      {isExpanded && (
        <tr>
          <td colSpan={canReclassify ? 8 : 7} className="bg-gray-50 px-6 py-3">
            {drillLoading ? (
              <div className="text-xs text-gray-400">Loading affected cookbooks…</div>
            ) : drillItems.length === 0 ? (
              <div className="text-xs text-gray-400">No cookbooks found.</div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="text-gray-500">
                      <th className="text-left pb-1">Cookbook</th>
                      <th className="text-left pb-1">Source</th>
                      <th className="text-right pb-1">Offences</th>
                      <th className="text-right pb-1">Auto-fix</th>
                      <th className="text-center pb-1">Would pass</th>
                    </tr>
                  </thead>
                  <tbody>
                    {drillItems.map((cb, i) => (
                      <tr key={`${cb.source}-${cb.name}-${i}`} className="border-t border-gray-100">
                        <td className="py-1 font-mono">{cb.name}</td>
                        <td className="py-1 text-gray-500">{cb.source}</td>
                        <td className="py-1 text-right">{cb.offence_count}</td>
                        <td className="py-1 text-right">{cb.auto_correctable}</td>
                        <td className="py-1 text-center">
                          {cb.would_pass_without ? (
                            <span className="text-green-600">✓</span>
                          ) : (
                            <span className="text-gray-300">—</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Reclassification modal
// ---------------------------------------------------------------------------

function ReclassifyModal({
  copName,
  value,
  reason,
  saving,
  onValueChange,
  onReasonChange,
  onSave,
  onCancel,
}: {
  copName: string;
  value: string;
  reason: string;
  saving: boolean;
  onValueChange: (v: string) => void;
  onReasonChange: (r: string) => void;
  onSave: () => void;
  onCancel: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30">
      <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
        <h3 className="text-sm font-semibold text-gray-800">
          Reclassify: <span className="font-mono">{copName}</span>
        </h3>
        <div className="mt-4 space-y-3">
          <div>
            <label className="block text-xs font-medium text-gray-600">Classification</label>
            <select
              className="mt-1 w-full rounded border border-gray-300 px-3 py-1.5 text-sm"
              value={value}
              onChange={(e) => onValueChange(e.target.value)}
            >
              <option value="blocker">Blocker</option>
              <option value="review">Review</option>
              <option value="noise">Noise</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600">Reason (optional)</label>
            <input
              type="text"
              className="mt-1 w-full rounded border border-gray-300 px-3 py-1.5 text-sm"
              placeholder="Why this classification?"
              value={reason}
              onChange={(e) => onReasonChange(e.target.value)}
            />
          </div>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            onClick={onCancel}
            className="rounded px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100"
          >
            Cancel
          </button>
          <button
            onClick={onSave}
            disabled={saving}
            className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
