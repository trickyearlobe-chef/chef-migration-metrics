// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { useGlobalFilters } from "../context/GlobalFilterContext";
import { useIsAdmin } from "../context/AuthContext";
import {
  fetchCookstyleCops,
  fetchCookstyleCopCookbooks,
  fetchCookstyleServerCopCookbooks,
  setCopClassification,
  type CopAggregationQuery,
} from "../api";
import type {
  CopAggregateItem,
  CopAggregationSummary,
  CopCookbookItem,
  CopCookbookGroup,
  Pagination as PaginationType,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { ClassificationBadge, CLASSIFICATION_FILTERS } from "../components/ClassificationBadge";

// ---------------------------------------------------------------------------
// CopAnalysisTab — classification-aware cop aggregation for a single source.
//
// The source is fixed per tab (Server or Git) rather than chosen from a
// dropdown, so each tab uses its natural grain: a server cookbook has many
// versions across orgs (grouped by name in the drill-down), while a git repo is
// 1:1 with a cookbook (a flat list). Fixing the source per tab also removes the
// old "All sources" double-count, and keeps the header count equal to the
// drill-down total within a tab.
// ---------------------------------------------------------------------------

const DRILL_PAGE_SIZE = 20;

export function CopAnalysisTab({ source }: { source: "server" | "git" }) {
  const { targetChefVersion } = useGlobalFilters();
  const isAdmin = useIsAdmin();
  const [searchParams, setSearchParams] = useSearchParams();

  // State
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [items, setItems] = useState<CopAggregateItem[]>([]);
  const [summary, setSummary] = useState<CopAggregationSummary | null>(null);
  const [pagination, setPagination] = useState<PaginationType | null>(null);

  // Drill-down state. Server uses grouped rows (drillGroups); git uses the flat
  // list (drillItems). Only the one matching `source` is populated at a time.
  const [drillCop, setDrillCop] = useState<string | null>(null);
  const [drillItems, setDrillItems] = useState<CopCookbookItem[]>([]);
  const [drillGroups, setDrillGroups] = useState<CopCookbookGroup[]>([]);
  const [drillPagination, setDrillPagination] = useState<PaginationType | null>(null);
  const [drillLoading, setDrillLoading] = useState(false);

  // Reclassification state
  const [reclassifyCop, setReclassifyCop] = useState<string | null>(null);
  const [reclassifyValue, setReclassifyValue] = useState<string>("blocker");
  const [reclassifyReason, setReclassifyReason] = useState("");
  const [reclassifySaving, setReclassifySaving] = useState(false);

  // Filters from URL params
  const classFilter = searchParams.get("classification") ?? "";
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
        source,
        page,
        per_page: DEFAULT_PAGE_SIZE,
        sort: sortField,
        order: sortOrder,
        // Cop Analysis is about cops that have triggered (cookbooks affected /
        // fix effort). Restrict to triggered cops so the endpoint's full
        // known-cop universe doesn't flood this view with zero-cookbook cops.
        triggered_only: true,
      };
      if (classFilter) params.classification = classFilter;

      const resp = await fetchCookstyleCops(params);
      setItems(resp.data ?? []);
      setSummary(resp.summary);
      setPagination(resp.pagination);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load cop data");
    } finally {
      setLoading(false);
    }
  }, [targetChefVersion, source, classFilter, sortField, sortOrder, page]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Reset the open drill-down when a list-level filter, sort, or target version
  // changes. Without this the expanded panel would show stale rows selected
  // under the previous filter (issue A: stale drill-down).
  useEffect(() => {
    setDrillCop(null);
    setDrillItems([]);
    setDrillGroups([]);
    setDrillPagination(null);
  }, [classFilter, sortField, sortOrder, targetChefVersion, source]);

  // Load one page of the drill-down for a cop. Server → grouped-by-name;
  // git → flat repo list. Both surface pagination (issue C).
  const loadDrill = useCallback(
    async (copName: string, drillPage: number) => {
      setDrillLoading(true);
      try {
        if (source === "server") {
          const resp = await fetchCookstyleServerCopCookbooks(copName, {
            target_chef_version: targetChefVersion,
            page: drillPage,
            per_page: DRILL_PAGE_SIZE,
          });
          setDrillGroups(resp.data ?? []);
          setDrillItems([]);
          setDrillPagination(resp.pagination);
        } else {
          const resp = await fetchCookstyleCopCookbooks(copName, {
            target_chef_version: targetChefVersion,
            source: "git",
            page: drillPage,
            per_page: DRILL_PAGE_SIZE,
          });
          setDrillItems(resp.data ?? []);
          setDrillGroups([]);
          setDrillPagination(resp.pagination);
        }
      } catch {
        setDrillItems([]);
        setDrillGroups([]);
        setDrillPagination(null);
      } finally {
        setDrillLoading(false);
      }
    },
    [source, targetChefVersion],
  );

  const openDrillDown = (copName: string) => {
    if (drillCop === copName) {
      setDrillCop(null);
      return;
    }
    setDrillCop(copName);
    loadDrill(copName, 1);
  };

  const changeDrillPage = (drillPage: number) => {
    if (drillCop) loadDrill(drillCop, drillPage);
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
                    title="Cookbooks where this appears in code that runs during a converge — the ones it can block."
                  >
                    Cookbooks{sortIndicator("cookbooks_affected")}
                  </th>
                  <th
                    className="px-3 py-2 text-right font-medium text-gray-600 cursor-pointer select-none"
                    onClick={() => toggleSort("cookbooks_excluded_only")}
                    title="Cookbooks where this appears only in files a converge never executes — a helper task, a pipeline, a test suite. Real work, but it does not block the cookbook."
                  >
                    Outside cookbook{sortIndicator("cookbooks_excluded_only")}
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
                    source={source}
                    isExpanded={drillCop === cop.cop_name}
                    drillItems={drillCop === cop.cop_name ? drillItems : []}
                    drillGroups={drillCop === cop.cop_name ? drillGroups : []}
                    drillPagination={drillCop === cop.cop_name ? drillPagination : null}
                    drillLoading={drillCop === cop.cop_name && drillLoading}
                    canReclassify={isAdmin}
                    onExpand={() => openDrillDown(cop.cop_name)}
                    onDrillPageChange={changeDrillPage}
                    onReclassify={() => {
                      setReclassifyCop(cop.cop_name);
                      setReclassifyValue(cop.classification || "blocker");
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
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
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
  source,
  isExpanded,
  drillItems,
  drillGroups,
  drillPagination,
  drillLoading,
  canReclassify,
  onExpand,
  onDrillPageChange,
  onReclassify,
}: {
  cop: CopAggregateItem;
  source: "server" | "git";
  isExpanded: boolean;
  drillItems: CopCookbookItem[];
  drillGroups: CopCookbookGroup[];
  drillPagination: PaginationType | null;
  drillLoading: boolean;
  canReclassify: boolean;
  onExpand: () => void;
  onDrillPageChange: (page: number) => void;
  onReclassify: () => void;
}) {
  const isEmpty =
    source === "server" ? drillGroups.length === 0 : drillItems.length === 0;

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
            <div
              className="mt-0.5 text-xs text-gray-400 truncate max-w-xs"
              title={cop.description}
            >
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
        <td className="px-3 py-2 text-right tabular-nums">
          {cop.cookbooks_excluded_only > 0 ? (
            <span className="text-gray-600">{cop.cookbooks_excluded_only}</span>
          ) : (
            <span className="text-gray-300">—</span>
          )}
        </td>
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
            ) : isEmpty ? (
              <div className="text-xs text-gray-400">No cookbooks found.</div>
            ) : (
              <div className="space-y-2">
                {source === "server" ? (
                  <ServerDrillDown groups={drillGroups} />
                ) : (
                  <GitDrillDown items={drillItems} />
                )}
                {drillPagination && (
                  <Pagination
                    pagination={drillPagination}
                    onPageChange={onDrillPageChange}
                  />
                )}
              </div>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Server drill-down: one row per cookbook name, expandable to version/org detail
// ---------------------------------------------------------------------------

function ServerDrillDown({ groups }: { groups: CopCookbookGroup[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-xs">
        <thead>
          <tr className="text-gray-500">
            <th className="text-left pb-1">Cookbook</th>
            <th className="text-right pb-1">Versions</th>
            <th className="text-right pb-1">Offences</th>
            <th className="text-right pb-1">Auto-fix</th>
            <th className="text-center pb-1">Would pass</th>
          </tr>
        </thead>
        <tbody>
          {groups.map((g) => (
            <ServerGroupRow key={g.name} group={g} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ServerGroupRow({ group }: { group: CopCookbookGroup }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <tr className="border-t border-gray-100">
        <td className="py-1 font-mono">
          {/* Caret toggles the version detail; the name links to the cookbook. */}
          <span className="inline-flex items-center gap-1">
            <button
              onClick={() => setOpen((v) => !v)}
              className="text-gray-400 hover:text-gray-600"
              aria-expanded={open}
              aria-label={open ? "Collapse versions" : "Expand versions"}
            >
              <span className="inline-block w-3">{open ? "▾" : "▸"}</span>
            </button>
            <Link
              to={`/cookbooks/${encodeURIComponent(group.name)}`}
              className="text-blue-700 hover:underline"
            >
              {group.name}
            </Link>
          </span>
        </td>
        <td className="py-1 text-right tabular-nums">{group.version_count}</td>
        <td className="py-1 text-right tabular-nums">{group.offence_count}</td>
        <td className="py-1 text-right tabular-nums">{group.auto_correctable}</td>
        <td className="py-1 text-center">
          {group.would_pass_without ? (
            <span className="text-green-600">✓</span>
          ) : (
            <span className="text-gray-300">—</span>
          )}
        </td>
      </tr>
      {open &&
        group.versions.map((v, i) => (
          <tr
            key={`${v.version}-${v.organisation ?? ""}-${i}`}
            className="border-t border-gray-50 bg-white/60"
          >
            <td className="py-1 pl-6 text-gray-600">
              <Link
                to={`/cookbooks/${encodeURIComponent(v.name)}/${encodeURIComponent(v.version)}/remediation`}
                className="font-mono text-blue-700 hover:underline"
              >
                {v.version}
              </Link>
              {v.organisation && (
                <span className="ml-2 text-gray-400">{v.organisation}</span>
              )}
            </td>
            <td className="py-1 text-right text-gray-300">—</td>
            <td className="py-1 text-right tabular-nums">{v.offence_count}</td>
            <td className="py-1 text-right tabular-nums">{v.auto_correctable}</td>
            <td className="py-1 text-center">
              {v.would_pass_without ? (
                <span className="text-green-600">✓</span>
              ) : (
                <span className="text-gray-300">—</span>
              )}
            </td>
          </tr>
        ))}
    </>
  );
}

// ---------------------------------------------------------------------------
// Git drill-down: flat repo list (1:1 repo == cookbook)
// ---------------------------------------------------------------------------

function GitDrillDown({ items }: { items: CopCookbookItem[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-xs">
        <thead>
          <tr className="text-gray-500">
            <th className="text-left pb-1">Repository</th>
            <th className="text-right pb-1">Offences</th>
            <th className="text-right pb-1">Auto-fix</th>
            <th className="text-center pb-1">Would pass</th>
          </tr>
        </thead>
        <tbody>
          {items.map((cb, i) => (
            <tr key={`${cb.name}-${i}`} className="border-t border-gray-100">
              <td className="py-1 font-mono">
                <Link
                  to={`/git-repos/${encodeURIComponent(cb.name)}`}
                  className="text-blue-700 hover:underline"
                >
                  {cb.name}
                </Link>
              </td>
              <td className="py-1 text-right tabular-nums">{cb.offence_count}</td>
              <td className="py-1 text-right tabular-nums">{cb.auto_correctable}</td>
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
