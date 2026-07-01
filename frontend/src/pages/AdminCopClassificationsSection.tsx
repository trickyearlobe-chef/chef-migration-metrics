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
} from "../components/ClassificationBadge";

// ---------------------------------------------------------------------------
// AdminCopClassificationsSection — searchable list of all known cops with their
// resolved classification + source, and per-cop operator override. This is the
// dedicated management surface (reclassification is otherwise only reachable
// inline from the Cop Analysis drill-down).
// ---------------------------------------------------------------------------

const PER_PAGE = 200;

export function AdminCopClassificationsSection() {
  const [versions, setVersions] = useState<string[]>([]);
  const [target, setTarget] = useState<string>("");
  const [classFilter, setClassFilter] = useState<string>("");
  const [search, setSearch] = useState<string>("");
  const [triggeredOnly, setTriggeredOnly] = useState<boolean>(false);

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

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return items;
    return items.filter(
      (c) =>
        c.cop_name.toLowerCase().includes(q) ||
        (c.description ?? "").toLowerCase().includes(q),
    );
  }, [items, search]);

  const openEditor = (cop: CopAggregateItem) => {
    setEditCop(cop.cop_name);
    setEditValue(cop.classification === "unclassified" ? "blocker" : cop.classification);
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
          Every known cop (curated defaults, <code>RemovedIn</code> mappings, scanned,
          and custom) with its resolved classification for the selected target. Override
          any cop; overrides take effect immediately and re-evaluate affected cookbooks.
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

      {!loading && !error && visible.length === 0 && (
        <div className="rounded border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-400">
          No cops match the current filters.
        </div>
      )}

      {!loading && !error && visible.length > 0 && (
        <div className="overflow-x-auto rounded border border-gray-200">
          <table className="min-w-full divide-y divide-gray-200 text-sm">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-3 py-2 text-left font-medium text-gray-600">Cop</th>
                <th className="px-3 py-2 text-left font-medium text-gray-600">
                  Classification
                </th>
                <th className="px-3 py-2 text-left font-medium text-gray-600">Source</th>
                <th className="px-3 py-2 text-left font-medium text-gray-600">RemovedIn</th>
                <th className="px-3 py-2 text-center font-medium text-gray-600">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {visible.map((cop) => {
                const isEditing = editCop === cop.cop_name;
                const isOverride = cop.classification_source === "operator_override";
                return (
                  <tr key={cop.cop_name} className="align-top hover:bg-gray-50">
                    <td className="px-3 py-2">
                      <div className="font-mono text-xs text-gray-800">{cop.cop_name}</div>
                      {cop.description && (
                        <div className="mt-0.5 max-w-md truncate text-xs text-gray-400">
                          {cop.description}
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-2">
                      <ClassificationBadge classification={cop.classification} />
                    </td>
                    <td className="px-3 py-2 text-xs text-gray-500">
                      {cop.classification_source.replace(/_/g, " ")}
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
