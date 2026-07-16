// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback, Fragment } from "react";
import { fetchNodeRuns } from "../api";
import type { ConvergeRun } from "../types";

// NodeRunsSection renders recent converge runs for a node (event-ingest telemetry),
// most-recent first. Failure rows expand to show the error class/message, the
// failing cookbook·recipe, and the backtrace. Reuses the .card panel idiom.
export function NodeRunsSection({
  org,
  nodeName,
}: {
  org: string;
  nodeName: string;
}) {
  const [runs, setRuns] = useState<ConvergeRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const resp = await fetchNodeRuns(org, nodeName);
      setRuns(resp.data);
    } catch {
      /* panel stays empty on error — ingest is optional telemetry */
    } finally {
      setLoading(false);
    }
  }, [org, nodeName]);

  useEffect(() => {
    load();
  }, [load]);

  const cookbookSummary = (cb: Record<string, string>): string => {
    const entries = Object.entries(cb || {});
    if (entries.length === 0) return "—";
    const shown = entries
      .slice(0, 4)
      .map(([n, v]) => `${n} ${v}`)
      .join(", ");
    return entries.length > 4 ? `${shown} +${entries.length - 4}` : shown;
  };

  return (
    <div className="card">
      <h3 className="card-header">Converge Runs</h3>
      {loading ? (
        <p className="text-sm text-gray-400">Loading runs…</p>
      ) : runs.length === 0 ? (
        <p className="text-sm text-gray-400">
          No converge runs ingested for this node yet.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left text-xs text-gray-500">
                <th className="py-1 pr-3">Status</th>
                <th className="py-1 pr-3">Ended</th>
                <th className="py-1 pr-3">Chef</th>
                <th className="py-1 pr-3">Run List</th>
                <th className="py-1 pr-3">Cookbooks</th>
                <th className="py-1 pr-3">Resources</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => {
                const failed = r.status === "failure";
                const isOpen = expanded === r.run_id;
                return (
                  <Fragment key={r.run_id}>
                    <tr
                      className={`border-b ${failed ? "cursor-pointer hover:bg-red-50" : ""}`}
                      onClick={() =>
                        failed && setExpanded(isOpen ? null : r.run_id)
                      }
                    >
                      <td className="py-1 pr-3">
                        <span
                          className={`rounded-full px-2 py-0.5 text-xs font-medium ${
                            failed
                              ? "bg-red-50 text-red-700"
                              : "bg-emerald-50 text-emerald-700"
                          }`}
                        >
                          {failed ? "▸ failure" : "success"}
                        </span>
                      </td>
                      <td className="py-1 pr-3 whitespace-nowrap">
                        {r.end_time
                          ? new Date(r.end_time).toLocaleString()
                          : "—"}
                      </td>
                      <td className="py-1 pr-3">{r.chef_version || "—"}</td>
                      <td className="py-1 pr-3 font-mono text-xs">
                        {(r.run_list || []).join(", ") || "—"}
                      </td>
                      <td className="py-1 pr-3 text-xs text-gray-600">
                        {cookbookSummary(r.cookbooks)}
                      </td>
                      <td className="py-1 pr-3 text-xs text-gray-600">
                        {r.updated_resource_count ?? 0}/
                        {r.total_resource_count ?? 0}
                      </td>
                    </tr>
                    {failed && isOpen && r.error && (
                      <tr className="border-b bg-red-50/40">
                        <td colSpan={6} className="px-3 py-2">
                          <div className="text-sm font-medium text-red-800">
                            {r.error.class}: {r.error.message}
                          </div>
                          {r.failed_resource && r.failed_resource.type && (
                            <div className="mt-1 text-xs text-gray-600">
                              Failed resource:{" "}
                              <span className="font-mono">
                                {r.failed_resource.type}[
                                {r.failed_resource.name}]
                              </span>{" "}
                              in {r.failed_resource.cookbook_name}::
                              {r.failed_resource.recipe_name}
                            </div>
                          )}
                          {r.error.backtrace &&
                            r.error.backtrace.length > 0 && (
                              <pre className="mt-2 max-h-64 overflow-auto rounded bg-gray-900 p-2 text-xs text-gray-100">
                                {r.error.backtrace.join("\n")}
                              </pre>
                            )}
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
