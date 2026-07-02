// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useMemo, useState } from "react";
import { fetchTargetVersions, fetchCopDrift } from "../api";
import type { CopDriftReport } from "../types";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";

// ---------------------------------------------------------------------------
// AdminCopInventorySection — surfaces classification-table drift against the
// live `cookstyle --show-cops` inventory:
//   • Coverage gaps: Chef/* cops the binary emits that resolve to unclassified
//     (must be curated so they participate in pass/fail decisions).
//   • Stale entries: curated/mapping rows for a cop the binary no longer emits
//     (safe to prune).
// Read-only: it is the worklist that feeds the Cop Classifications editor above.
// ---------------------------------------------------------------------------

// Human labels for the stale-entry source (which static table owns the row).
const STALE_SOURCE_LABEL: Record<string, string> = {
  curated_default: "Curated default",
  removed_in_mapping: "RemovedIn mapping",
};

export function AdminCopInventorySection() {
  const [versions, setVersions] = useState<string[]>([]);
  const [target, setTarget] = useState<string>("");
  const [report, setReport] = useState<CopDriftReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Load target versions once; default to the first.
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

  const loadDrift = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await fetchCopDrift(target ? { target_chef_version: target } : undefined);
      setReport(resp);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load cop drift report");
    } finally {
      setLoading(false);
    }
  }, [target]);

  useEffect(() => {
    loadDrift();
  }, [loadDrift]);

  const gaps = useMemo(() => report?.coverage_gaps ?? [], [report]);
  const stale = useMemo(() => report?.stale ?? [], [report]);

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-lg font-medium text-gray-900">Cop Inventory &amp; Drift</h3>
        <p className="mt-1 text-sm text-gray-500">
          Cross-references the live <code>cookstyle --show-cops</code> inventory against the
          classification tables. Curate the coverage gaps so new Chef cops participate in
          pass/fail; prune stale rows the binary has dropped.
        </p>
      </div>

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
        {report?.registry_available && report.registry_version && (
          <span className="text-xs text-gray-500">
            cookstyle {report.registry_version}
          </span>
        )}
      </div>

      {error && <ErrorAlert message={error} onRetry={loadDrift} />}
      {loading && <LoadingSpinner message="Loading cop drift…" />}

      {!loading && !error && report && !report.registry_available && (
        <div
          className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800"
          role="status"
        >
          The cookstyle binary is unavailable, so live drift cannot be computed. The static
          classification tables still apply. Drift will populate once cookstyle is available.
        </div>
      )}

      {!loading && !error && report?.registry_available && (
        <div className="space-y-6">
          <section aria-labelledby="coverage-gaps-heading">
            <h4 id="coverage-gaps-heading" className="text-sm font-semibold text-gray-800">
              Coverage gaps{" "}
              <span className="font-normal text-gray-500">({gaps.length})</span>
            </h4>
            <p className="mt-0.5 text-xs text-gray-500">
              Chef cops the binary emits with no classification — resolve to unclassified.
            </p>
            {gaps.length === 0 ? (
              <p className="mt-2 text-sm text-gray-500">
                No coverage gaps — every live Chef cop is classified. 🎉
              </p>
            ) : (
              <table className="mt-2 w-full border-collapse text-sm">
                <thead>
                  <tr className="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500">
                    <th className="py-1.5 pr-4">Cop</th>
                    <th className="py-1.5 pr-4">Department</th>
                    <th className="py-1.5 pr-4">Enabled</th>
                  </tr>
                </thead>
                <tbody>
                  {gaps.map((g) => (
                    <tr key={g.cop_name} className="border-b border-gray-100">
                      <td className="py-1.5 pr-4 font-mono text-xs text-gray-800">{g.cop_name}</td>
                      <td className="py-1.5 pr-4 text-gray-600">{g.department}</td>
                      <td className="py-1.5 pr-4 text-gray-600">{g.enabled ? "yes" : "no"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>

          <section aria-labelledby="stale-heading">
            <h4 id="stale-heading" className="text-sm font-semibold text-gray-800">
              Stale entries{" "}
              <span className="font-normal text-gray-500">({stale.length})</span>
            </h4>
            <p className="mt-0.5 text-xs text-gray-500">
              Classification-table rows for a cop this binary no longer emits — safe to prune.
            </p>
            {stale.length === 0 ? (
              <p className="mt-2 text-sm text-gray-500">
                No stale entries — every classified cop still exists in the binary.
              </p>
            ) : (
              <table className="mt-2 w-full border-collapse text-sm">
                <thead>
                  <tr className="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500">
                    <th className="py-1.5 pr-4">Cop</th>
                    <th className="py-1.5 pr-4">Source table</th>
                  </tr>
                </thead>
                <tbody>
                  {stale.map((s) => (
                    <tr key={s.cop_name} className="border-b border-gray-100">
                      <td className="py-1.5 pr-4 font-mono text-xs text-gray-800">{s.cop_name}</td>
                      <td className="py-1.5 pr-4 text-gray-600">
                        {STALE_SOURCE_LABEL[s.source] ?? s.source}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
