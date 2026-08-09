// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import {
  fetchScanScope,
  saveScanScopeEntry,
  deleteScanScopeEntry,
  type ScanScopeEntry,
} from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";

// Saying how many verdicts moved is the point of recomputing on save: "none"
// is a real answer worth showing, because it means the decision was already
// true of everything scanned rather than that nothing happened.
function verdictMessage(changed: number): string {
  if (changed === 0) {
    return "Saved. No stored verdict changed — nothing scanned was affected by this.";
  }
  if (changed === 1) {
    return "Saved. 1 cookbook's verdict changed.";
  }
  return `Saved. ${changed} verdicts changed.`;
}

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

/**
 * AdminScanScopeSection edits which files a converge never executes.
 *
 * The seeded list reaches files with predictable names. It cannot reach a
 * script that only runs because a build job invokes it — that sits at a
 * different path in every estate, and nothing in the file says what runs it.
 * This screen is how somebody who knows the job says so.
 *
 * Both directions are edits: adding a pattern, and overturning a seeded one for
 * an estate where that directory really does ship code that runs. Both require
 * a reason, because an exclusion nobody can argue with is an exclusion nobody
 * can check.
 */
export function AdminScanScopeSection() {
  const [entries, setEntries] = useState<ScanScopeEntry[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [busyPattern, setBusyPattern] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const [recomputeMsg, setRecomputeMsg] = useState<string | null>(null);

  const [newPattern, setNewPattern] = useState("");
  const [newReason, setNewReason] = useState("");
  const [adding, setAdding] = useState(false);

  const load = useCallback(() => {
    setLoadError(null);
    fetchScanScope()
      .then((res) => setEntries(res.data ?? []))
      .catch((e: Error) => setLoadError(e.message));
  }, []);

  useEffect(load, [load]);

  const handleAdd = useCallback(() => {
    const pattern = newPattern.trim();
    const reason = newReason.trim();
    if (!pattern || !reason) return;
    setAdding(true);
    setActionError(null);
    saveScanScopeEntry({ pattern, excluded: true, reason })
      .then((res) => {
        setNewPattern("");
        setNewReason("");
        setRecomputeMsg(verdictMessage(res.verdicts_changed));
        load();
      })
      .catch((e: Error) => setActionError(e.message))
      .finally(() => setAdding(false));
  }, [newPattern, newReason, load]);

  const handleToggle = useCallback(
    (entry: ScanScopeEntry) => {
      const reason = window.prompt(
        entry.excluded
          ? `Why does ${entry.pattern} actually run during a converge?`
          : `Why does ${entry.pattern} not run during a converge?`,
        entry.source === "operator" ? entry.reason : "",
      );
      if (reason === null) return;
      if (!reason.trim()) {
        setActionError("A reason is required — it is what makes this checkable by somebody else.");
        return;
      }
      setBusyPattern(entry.pattern);
      setActionError(null);
      saveScanScopeEntry({
        pattern: entry.pattern,
        excluded: !entry.excluded,
        reason: reason.trim(),
      })
        .then((res) => {
          setRecomputeMsg(verdictMessage(res.verdicts_changed));
          return load();
        })
        .catch((e: Error) => setActionError(e.message))
        .finally(() => setBusyPattern(null));
    },
    [load],
  );

  const handleRevert = useCallback(
    (entry: ScanScopeEntry) => {
      setBusyPattern(entry.pattern);
      setActionError(null);
      deleteScanScopeEntry(entry.pattern)
        .then((res) => {
          setRecomputeMsg(verdictMessage(res.verdicts_changed));
          return load();
        })
        .catch((e: Error) => setActionError(e.message))
        .finally(() => setBusyPattern(null));
    },
    [load],
  );

  if (loadError) {
    return <ErrorAlert message={`Failed to load the scan scope: ${loadError}`} />;
  }
  if (entries === null) {
    return <LoadingSpinner />;
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-gray-600">
        A repository holds more than the cookbook. Findings in files listed here stay visible on
        the cookbook as work somebody has to do, but they do not decide whether the cookbook
        survives the upgrade. A pattern ending in <code className="font-mono">/*</code> covers
        that directory and everything under it.
      </p>

      {actionError && <ErrorAlert message={actionError} />}

      {recomputeMsg && (
        <p className="rounded bg-blue-50 px-3 py-2 text-sm text-blue-800">{recomputeMsg}</p>
      )}

      <div className="overflow-x-auto rounded border border-gray-200">
        <table className="min-w-full divide-y divide-gray-200 text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-3 py-2 text-left font-medium text-gray-600">Pattern</th>
              <th className="px-3 py-2 text-left font-medium text-gray-600">Runs on a node?</th>
              <th className="px-3 py-2 text-left font-medium text-gray-600">Reason</th>
              <th className="px-3 py-2 text-left font-medium text-gray-600">Decided by</th>
              <th className="px-3 py-2 text-center font-medium text-gray-600">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {entries.map((entry) => (
              <tr key={entry.pattern} className={entry.excluded ? "" : "bg-amber-50"}>
                <td className="px-3 py-2 font-mono text-xs">{entry.pattern}</td>
                <td className="px-3 py-2">
                  {entry.excluded ? (
                    <span className="text-gray-600">No — does not block</span>
                  ) : (
                    <span className="font-medium text-amber-800">Yes — counts as cookbook code</span>
                  )}
                </td>
                <td className="px-3 py-2 text-gray-600">{entry.reason}</td>
                <td className="px-3 py-2 text-xs text-gray-500">
                  {entry.source === "curated" ? (
                    <span className="text-gray-400">shipped default</span>
                  ) : (
                    entry.created_by || "an operator"
                  )}
                </td>
                <td className="px-3 py-2 text-center whitespace-nowrap">
                  {busyPattern === entry.pattern ? (
                    <InlineSpinner />
                  ) : (
                    <>
                      <button
                        onClick={() => handleToggle(entry)}
                        className="rounded px-2 py-0.5 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700"
                      >
                        {entry.excluded ? "It does run" : "It does not run"}
                      </button>
                      {entry.source === "operator" && (
                        <button
                          onClick={() => handleRevert(entry)}
                          className="ml-1 rounded px-2 py-0.5 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700"
                          title="Remove this decision. A shipped default returns to its original behaviour."
                        >
                          Undo
                        </button>
                      )}
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="rounded border border-gray-200 p-3">
        <h4 className="mb-2 text-sm font-semibold text-gray-700">Add a pattern</h4>
        <div className="grid gap-2 sm:grid-cols-2">
          <input
            className={INPUT_CLASS}
            placeholder="tooling/ci/*"
            aria-label="Pattern"
            value={newPattern}
            onChange={(e) => setNewPattern(e.target.value)}
            disabled={adding}
          />
          <input
            className={INPUT_CLASS}
            placeholder="Why this never runs during a converge"
            aria-label="Reason"
            value={newReason}
            onChange={(e) => setNewReason(e.target.value)}
            disabled={adding}
          />
        </div>
        <button
          onClick={handleAdd}
          disabled={adding || !newPattern.trim() || !newReason.trim()}
          className="mt-2 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:bg-gray-300"
        >
          {adding ? <InlineSpinner /> : "Add"}
        </button>
      </div>
    </div>
  );
}
