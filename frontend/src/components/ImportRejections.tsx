// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import { fetchImportRejections, type ImportRejection } from "../api";
import { ErrorAlert, EmptyState, LoadingSpinner } from "./Feedback";

/**
 * The rows an import could not use — see journeys/ownership-intake.md:
 * "which row, and what was wrong with it — so I can get the source fixed rather
 * than silently importing three quarters of it."
 *
 * These have been recorded since imports existed and were reachable only by
 * taking an export, so an import that dropped a quarter of its rows told the
 * administrator nothing. Each row names where it came from, so it can be taken
 * back to whoever owns the source.
 *
 * Empty is the state worth reaching, so it says so plainly rather than showing
 * an empty table.
 */
export function ImportRejections() {
  const [rows, setRows] = useState<ImportRejection[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(false);

  const load = useCallback(() => {
    setError(null);
    fetchImportRejections({ page, per_page: 100 })
      .then((res) => {
        setRows(res.data ?? []);
        setHasMore((res.data?.length ?? 0) >= 100);
      })
      .catch((e: Error) => setError(e.message));
  }, [page]);

  useEffect(load, [load]);

  if (error) {
    return <ErrorAlert message={`Failed to load the rows the import could not use: ${error}`} />;
  }
  if (rows === null) return <LoadingSpinner />;

  if (rows.length === 0) {
    return (
      <EmptyState
        title="Every row was used"
        description="Nothing was rejected by the last run of any import. This is the state worth getting to — a row fixed at source stops being reported here."
      />
    );
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-gray-600">
        Rows the last run could not use. Each one names the import it came from and the row in
        the source, so it can be taken back to whoever owns that system. Fixing a row at source
        stops it being reported here on the next run.
      </p>

      <div className="overflow-x-auto rounded border border-gray-200">
        <table className="min-w-full divide-y divide-gray-200 text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-3 py-2 text-left font-medium text-gray-600">Import</th>
              <th className="px-3 py-2 text-right font-medium text-gray-600">Source row</th>
              <th className="px-3 py-2 text-left font-medium text-gray-600">What was wrong</th>
              <th className="px-3 py-2 text-left font-medium text-gray-600">Owner as written</th>
              <th className="px-3 py-2 text-left font-medium text-gray-600">What it was for</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {rows.map((r, i) => (
              <tr key={`${r.import_label}-${r.source_row}-${i}`}>
                <td className="px-3 py-2 text-xs text-gray-600">{r.import_label}</td>
                <td className="px-3 py-2 text-right tabular-nums">{r.source_row}</td>
                <td className="px-3 py-2">{r.reason}</td>
                <td className="px-3 py-2 font-mono text-xs text-gray-600">
                  {r.owner_raw || <span className="text-gray-300">—</span>}
                </td>
                <td className="px-3 py-2 text-xs text-gray-600">
                  {r.entity_key ? (
                    <>
                      {r.entity_type ? `${r.entity_type}: ` : ""}
                      {r.entity_key}
                    </>
                  ) : (
                    <span className="text-gray-300">—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {(page > 1 || hasMore) && (
        <div className="flex items-center gap-2">
          <button
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page === 1}
            className="rounded border border-gray-300 px-2 py-1 text-sm disabled:opacity-40"
          >
            Previous
          </button>
          <span className="text-sm text-gray-500">Page {page}</span>
          <button
            onClick={() => setPage((p) => p + 1)}
            disabled={!hasMore}
            className="rounded border border-gray-300 px-2 py-1 text-sm disabled:opacity-40"
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}
