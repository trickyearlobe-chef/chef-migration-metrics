// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  fetchTargetVersions,
  fetchCookstyleCops,
  setCopClassification,
  deleteCopClassification,
  type CopAggregationQuery,
} from "../api";
import type { CopAggregateItem } from "../types";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";
import {
  ClassificationBadge,
  ClassificationFilterBar,
  classificationSourceLabel,
} from "../components/ClassificationBadge";

// ---------------------------------------------------------------------------
// AdminCopClassificationsSection — searchable list of all known cops with their
// resolved classification + source, and per-cop operator override. This is the
// dedicated management surface (reclassification is otherwise only reachable
// inline from the Cop Analysis drill-down).
// ---------------------------------------------------------------------------

const PER_PAGE = 200;

// Client-side sort over the fetched page. Columns like classification/source have
// no server sort field, and the fetched set is the whole (paginated) list, so
// sorting locally keeps every column sortable and the UI instant.
type SortKey =
  | "cop_name"
  | "classification"
  | "classification_source"
  | "cookbooks_affected"
  | "removed_in";
type SortDir = "asc" | "desc";

// Impact order for classification: blockers first so the must-curate work sorts
// to the top in ascending order.
const CLASS_RANK: Record<string, number> = {
  blocker: 0,
  review: 1,
  noise: 2,
};

// Numeric-aware compare for Chef version strings ("16.0" < "18.0").
function compareVersion(a: string, b: string): number {
  const pa = a.split(".").map((n) => parseInt(n, 10) || 0);
  const pb = b.split(".").map((n) => parseInt(n, 10) || 0);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d !== 0) return d;
  }
  return 0;
}

function makeCopComparator(
  key: SortKey,
  dir: SortDir,
): (a: CopAggregateItem, b: CopAggregateItem) => number {
  const mul = dir === "asc" ? 1 : -1;
  return (a, b) => {
    switch (key) {
      case "classification":
        return (
          mul *
          ((CLASS_RANK[a.classification] ?? 9) - (CLASS_RANK[b.classification] ?? 9))
        );
      case "classification_source":
        return mul * a.classification_source.localeCompare(b.classification_source);
      case "cookbooks_affected":
        return mul * (a.cookbooks_affected - b.cookbooks_affected);
      case "removed_in": {
        // Cops with no RemovedIn always sort last, regardless of direction.
        const ae = !a.removed_in;
        const be = !b.removed_in;
        if (ae && be) return 0;
        if (ae) return 1;
        if (be) return -1;
        return mul * compareVersion(a.removed_in ?? "", b.removed_in ?? "");
      }
      default:
        return mul * a.cop_name.localeCompare(b.cop_name);
    }
  };
}

export function AdminCopClassificationsSection() {
  const [versions, setVersions] = useState<string[]>([]);
  const [target, setTarget] = useState<string>("");
  const [classFilter, setClassFilter] = useState<string>("");
  const [search, setSearch] = useState<string>("");
  const [triggeredOnly, setTriggeredOnly] = useState<boolean>(false);
  const [sourceFilter, setSourceFilter] = useState<string>("");
  const [sortKey, setSortKey] = useState<SortKey>("cop_name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  const [items, setItems] = useState<CopAggregateItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [truncated, setTruncated] = useState(false);

  // Inline override editor state.
  const [editCop, setEditCop] = useState<string | null>(null);
  const [editValue, setEditValue] = useState<string>("blocker");
  const [editReason, setEditReason] = useState<string>("");
  const [busyCop, setBusyCop] = useState<string | null>(null);

  // Load the available target versions once; default to the first.
  useEffect(() => {
    let cancelled = false;
    fetchTargetVersions()
      .then((v) => {
        if (cancelled) return;
        setVersions(v);
        setTarget((cur) => cur || v[0] || "");
      })
      .catch((e: unknown) => {
        if (!cancelled)
          setError(e instanceof Error ? e.message : "Failed to load target versions");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const loadCops = useCallback(async () => {
    if (!target) return;
    setLoading(true);
    setError(null);
    try {
      const params: CopAggregationQuery = {
        target_chef_version: target,
        sort: "cop_name",
        order: "asc",
        per_page: PER_PAGE,
        page: 1,
      };
      if (classFilter) params.classification = classFilter;
      if (triggeredOnly) params.triggered_only = true;
      const resp = await fetchCookstyleCops(params);
      setItems(resp.data ?? []);
      setTruncated((resp.pagination?.total_items ?? 0) > (resp.data?.length ?? 0));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load cops");
    } finally {
      setLoading(false);
    }
  }, [target, classFilter, triggeredOnly]);

  useEffect(() => {
    loadCops();
  }, [loadCops]);

  const rows = useMemo(() => {
    const q = search.trim().toLowerCase();
    let out = items;
    if (q) {
      out = out.filter(
        (c) =>
          c.cop_name.toLowerCase().includes(q) ||
          (c.description ?? "").toLowerCase().includes(q),
      );
    }
    if (sourceFilter) {
      out = out.filter((c) => c.classification_source === sourceFilter);
    }
    return [...out].sort(makeCopComparator(sortKey, sortDir));
  }, [items, search, sourceFilter, sortKey, sortDir]);

  const toggleSort = (key: SortKey) => {
    if (key === sortKey) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("asc");
    }
  };

  const openEditor = (cop: CopAggregateItem) => {
    setEditCop(cop.cop_name);
    setEditValue(cop.classification || "blocker");
    setEditReason("");
  };

  const saveOverride = async () => {
    if (!editCop || !target) return;
    setBusyCop(editCop);
    try {
      await setCopClassification(editCop, {
        target_chef_version: target,
        classification: editValue,
        reason: editReason || undefined,
      });
      setEditCop(null);
      setEditReason("");
      await loadCops();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save classification");
    } finally {
      setBusyCop(null);
    }
  };

  const resetOverride = async (copName: string) => {
    if (!target) return;
    setBusyCop(copName);
    try {
      await deleteCopClassification(copName, target);
      await loadCops();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to reset classification");
    } finally {
      setBusyCop(null);
    }
  };

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-lg font-medium text-gray-900">Cop Classifications</h3>
        <p className="mt-1 text-sm text-gray-500">
          Every known cop with its resolved classification and source (operator override,
          custom cop, verified removal, structural noise, or the review default) for the
          selected target. Override any cop; overrides take effect immediately and
          re-evaluate affected cookbooks.
        </p>
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <span className="font-medium">Target Chef version</span>
          <select
            aria-label="Target Chef version"
            className="rounded border border-gray-300 px-2 py-1 text-sm"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          >
            {versions.map((v) => (
              <option key={v} value={v}>
                {v}
              </option>
            ))}
          </select>
        </label>

        <input
          type="search"
          placeholder="Search cops…"
          className="rounded border border-gray-300 px-3 py-1 text-sm"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />

        <ClassificationFilterBar activeFilter={classFilter} onFilterChange={setClassFilter} />

        <label className="flex items-center gap-2 text-sm text-gray-700">
          <span className="font-medium">Source</span>
          <select
            aria-label="Filter by source"
            className="rounded border border-gray-300 px-2 py-1 text-sm"
            value={sourceFilter}
            onChange={(e) => setSourceFilter(e.target.value)}
          >
            <option value="">All sources</option>
            <option value="operator_override">Operator override</option>
            <option value="custom_cop">Custom cop</option>
            <option value="verified_removal">Verified removal</option>
            <option value="structural_noise">Structural noise</option>
            <option value="review_default">Review (default)</option>
          </select>
        </label>

        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            className="rounded border-gray-300"
            checked={triggeredOnly}
            onChange={(e) => setTriggeredOnly(e.target.checked)}
          />
          <span>Only cops that have triggered</span>
        </label>
      </div>

      {error && <ErrorAlert message={error} />}
      {loading && <LoadingSpinner />}

      {!loading && !error && rows.length === 0 && (
        <div className="rounded border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-400">
          No cops match the current filters.
        </div>
      )}

      {!loading && !error && rows.length > 0 && (
        <div className="overflow-x-auto rounded border border-gray-200">
          <table className="min-w-full divide-y divide-gray-200 text-sm">
            <thead className="bg-gray-50">
              <tr>
                <SortHeader label="Cop" sortKeyName="cop_name" activeKey={sortKey} dir={sortDir} onSort={toggleSort} />
                <SortHeader label="Classification" sortKeyName="classification" activeKey={sortKey} dir={sortDir} onSort={toggleSort} />
                <SortHeader label="Source" sortKeyName="classification_source" activeKey={sortKey} dir={sortDir} onSort={toggleSort} />
                <SortHeader label="Cookbooks" sortKeyName="cookbooks_affected" activeKey={sortKey} dir={sortDir} onSort={toggleSort} />
                <SortHeader label="RemovedIn" sortKeyName="removed_in" activeKey={sortKey} dir={sortDir} onSort={toggleSort} />
                <th className="px-3 py-2 text-center font-medium text-gray-600">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {rows.map((cop) => {
                const isEditing = editCop === cop.cop_name;
                const isOverride = cop.classification_source === "operator_override";
                return (
                  <tr key={cop.cop_name} className="align-top hover:bg-gray-50">
                    <td className="px-3 py-2">
                      <div
                        className="font-mono text-xs text-gray-800"
                        title={cop.cop_name}
                      >
                        {cop.cop_name}
                      </div>
                      {cop.description && (
                        <div
                          className="mt-0.5 max-w-md truncate text-xs text-gray-400"
                          title={cop.description}
                        >
                          {cop.description}
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-2">
                      <ClassificationBadge classification={cop.classification} />
                    </td>
                    <td className="px-3 py-2 text-xs text-gray-500">
                      {classificationSourceLabel(cop.classification_source)}
                    </td>
                    <td className="px-3 py-2 text-xs text-gray-600">
                      {cop.cookbooks_affected > 0 ? (
                        cop.cookbooks_affected
                      ) : (
                        <span className="text-gray-300">—</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-xs text-gray-600">
                      {cop.removed_in ? (
                        <span className="font-mono text-red-600">Chef {cop.removed_in}</span>
                      ) : (
                        <span className="text-gray-300">—</span>
                      )}
                    </td>
                    <td className="px-3 py-2">
                      {isEditing ? (
                        <OverrideEditor
                          value={editValue}
                          reason={editReason}
                          busy={busyCop === cop.cop_name}
                          onValue={setEditValue}
                          onReason={setEditReason}
                          onSave={saveOverride}
                          onCancel={() => setEditCop(null)}
                        />
                      ) : (
                        <div className="flex items-center justify-center gap-2">
                          <button
                            type="button"
                            aria-label={`Override ${cop.cop_name}`}
                            onClick={() => openEditor(cop)}
                            className="rounded px-2 py-0.5 text-xs text-blue-600 hover:bg-blue-50 hover:underline"
                          >
                            Override
                          </button>
                          {isOverride && (
                            <button
                              type="button"
                              aria-label={`Reset ${cop.cop_name}`}
                              disabled={busyCop === cop.cop_name}
                              onClick={() => resetOverride(cop.cop_name)}
                              className="rounded px-2 py-0.5 text-xs text-gray-500 hover:bg-gray-100 hover:underline disabled:opacity-50"
                            >
                              Reset
                            </button>
                          )}
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {truncated && (
        <p className="text-xs text-gray-400">
          Showing the first {PER_PAGE} cops. Narrow with the search or classification
          filter to find a specific cop.
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sortable column header
// ---------------------------------------------------------------------------

function SortHeader({
  label,
  sortKeyName,
  activeKey,
  dir,
  onSort,
}: {
  label: string;
  sortKeyName: SortKey;
  activeKey: SortKey;
  dir: SortDir;
  onSort: (key: SortKey) => void;
}) {
  const active = activeKey === sortKeyName;
  return (
    <th
      scope="col"
      aria-sort={active ? (dir === "asc" ? "ascending" : "descending") : "none"}
      className="px-3 py-2 text-left font-medium text-gray-600"
    >
      <button
        type="button"
        aria-label={`Sort by ${label}`}
        onClick={() => onSort(sortKeyName)}
        className="inline-flex items-center gap-1 hover:text-gray-900"
      >
        <span>{label}</span>
        <span aria-hidden="true" className={active ? "text-gray-700" : "text-gray-300"}>
          {active ? (dir === "asc" ? "▲" : "▼") : "⇅"}
        </span>
      </button>
    </th>
  );
}

// ---------------------------------------------------------------------------
// Inline override editor
// ---------------------------------------------------------------------------

function OverrideEditor({
  value,
  reason,
  busy,
  onValue,
  onReason,
  onSave,
  onCancel,
}: {
  value: string;
  reason: string;
  busy: boolean;
  onValue: (v: string) => void;
  onReason: (r: string) => void;
  onSave: () => void;
  onCancel: () => void;
}) {
  return (
    <div className="space-y-2 rounded border border-blue-200 bg-blue-50/50 p-2">
      <select
        aria-label="Classification"
        className="w-full rounded border border-gray-300 px-2 py-1 text-xs"
        value={value}
        onChange={(e) => onValue(e.target.value)}
      >
        <option value="blocker">Blocker</option>
        <option value="review">Review</option>
        <option value="noise">Noise</option>
      </select>
      <input
        type="text"
        aria-label="Reason"
        placeholder="Reason (optional)"
        className="w-full rounded border border-gray-300 px-2 py-1 text-xs"
        value={reason}
        onChange={(e) => onReason(e.target.value)}
      />
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="rounded px-2 py-0.5 text-xs text-gray-600 hover:bg-gray-100"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={onSave}
          disabled={busy}
          className="rounded bg-blue-600 px-2 py-0.5 text-xs text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {busy ? "Saving…" : "Save"}
        </button>
      </div>
    </div>
  );
}
