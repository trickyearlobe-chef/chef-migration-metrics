// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useMemo, useCallback, Fragment } from "react";
import { useNavigate } from "react-router-dom";
import {
  fetchRunEventNodes,
  fetchRunEventRuns,
  fetchFilterRunOrganisations,
  fetchFilterRunChefVersions,
} from "../api";
import type { RunEventFilterQuery } from "../api/client";
import type { RunEventItem } from "../api/run-events";
import type { Pagination as PaginationType } from "../types";
import { FilterInput, FilterSelect, FilterCombobox } from "../components/FilterInputs";
import { Pagination } from "../components/Pagination";
import { ExportButton } from "../components/ExportButton";
import type { ExportParams } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { SortableColumnHeader } from "../components/SortableColumnHeader";
import { useSort } from "../hooks/useSort";

type Tab = "nodes" | "runs";
type SortField = "end_time" | "node_name" | "status" | "chef_version" | "organisation";

// Convert a <input type="datetime-local"> value (local time, no zone) to RFC3339.
function localToRFC3339(v: string): string | undefined {
  if (!v) return undefined;
  const d = new Date(v);
  return isNaN(d.getTime()) ? undefined : d.toISOString();
}

export function RunEventsPage() {
  const navigate = useNavigate();

  const [tab, setTab] = useState<Tab>("nodes");

  // Filters (sourced from converge_runs — NOT the global org filter).
  const [organisation, setOrganisation] = useState("");
  const [status, setStatus] = useState("failure"); // default to failures
  const [node, setNode] = useState("");
  const [chefVersion, setChefVersion] = useState("");
  const [cookbook, setCookbook] = useState("");
  const [failureMessage, setFailureMessage] = useState("");
  const [since, setSince] = useState(""); // datetime-local

  // As-of anchor: pinned at load so live-appended rows don't skew paging.
  const [anchor, setAnchor] = useState<string>(() => new Date().toISOString());

  const [page, setPage] = useState(1);
  const perPage = 25;

  // Runs tab: which run's failure detail is expanded inline (run_id).
  const [expandedRun, setExpandedRun] = useState<string | null>(null);

  const { sortField, sortOrder, handleSort } = useSort<SortField>({
    defaultField: "end_time",
    defaultOrder: "desc",
    descendingFields: ["end_time"],
  });

  // Backend-populated filter options.
  const [orgOptions, setOrgOptions] = useState<string[]>([]);
  const [versionOptions, setVersionOptions] = useState<string[]>([]);

  const [rows, setRows] = useState<RunEventItem[]>([]);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchFilterRunOrganisations()
      .then((r) => setOrgOptions(r.data ?? []))
      .catch(() => setOrgOptions([]));
    fetchFilterRunChefVersions()
      .then((r) => setVersionOptions(r.data ?? []))
      .catch(() => setVersionOptions([]));
  }, []);

  const listQuery = useMemo<RunEventFilterQuery>(
    () => ({
      organisation: organisation || undefined,
      status: status || undefined,
      node: node || undefined,
      chef_version: chefVersion || undefined,
      cookbook: cookbook || undefined,
      failure_message: failureMessage || undefined,
      since: localToRFC3339(since),
      until: anchor, // as-of anchor
      sort: sortField,
      order: sortOrder,
    }),
    [organisation, status, node, chefVersion, cookbook, failureMessage, since, anchor, sortField, sortOrder],
  );

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    const fetcher = tab === "nodes" ? fetchRunEventNodes : fetchRunEventRuns;
    fetcher({ ...listQuery, page, per_page: perPage })
      .then((res) => {
        setRows(res.data ?? []);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [tab, listQuery, page]);

  useEffect(() => {
    load();
  }, [load]);

  const refresh = useCallback(() => {
    setAnchor(new Date().toISOString());
    setPage(1);
  }, []);

  const goToNode = useCallback(
    (org: string, nodeName: string) => {
      navigate(
        `/run-events/nodes/${encodeURIComponent(org)}/${encodeURIComponent(nodeName)}`,
      );
    },
    [navigate],
  );

  function changeTab(next: Tab) {
    if (next === tab) return;
    setTab(next);
    setPage(1);
    setExpandedRun(null);
  }

  const statusBadge = (s: string) => {
    const failed = s === "failure";
    return (
      <span
        className={`rounded-full px-2 py-0.5 text-xs font-medium ${
          failed ? "bg-red-50 text-red-700" : "bg-emerald-50 text-emerald-700"
        }`}
      >
        {s}
      </span>
    );
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-xl font-bold text-gray-800">Run events</h2>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-400" title="Data is a stable snapshot as of this time">
            as of {new Date(anchor).toLocaleTimeString()}
          </span>
          <button
            onClick={refresh}
            className="rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-900"
            title="Re-anchor to pull newer events"
          >
            Refresh
          </button>
          <ExportButton
            exportType="run_events"
            params={listQuery as ExportParams}
            label="Export"
          />
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-gray-200">
        {(["nodes", "runs"] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => changeTab(t)}
            className={`-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors ${
              tab === t
                ? "border-blue-600 text-blue-700"
                : "border-transparent text-gray-500 hover:text-gray-700"
            }`}
          >
            {t === "nodes" ? "Nodes" : "Runs"}
          </button>
        ))}
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3">
        <FilterCombobox
          label="Organisation"
          value={organisation}
          onChange={(v) => {
            setOrganisation(v);
            setPage(1);
          }}
          options={orgOptions}
        />
        <FilterSelect
          label="Status"
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={[
            { value: "", label: "All" },
            { value: "failure", label: "Failure" },
            { value: "success", label: "Success" },
          ]}
        />
        <FilterInput
          label="Node"
          value={node}
          onChange={(v) => {
            setNode(v);
            setPage(1);
          }}
          placeholder="Node name"
          debounceMs={300}
        />
        <FilterCombobox
          label="Chef version"
          value={chefVersion}
          onChange={(v) => {
            setChefVersion(v);
            setPage(1);
          }}
          options={versionOptions}
        />
        <FilterInput
          label="Cookbook"
          value={cookbook}
          onChange={(v) => {
            setCookbook(v);
            setPage(1);
          }}
          placeholder="Cookbook used"
          debounceMs={300}
        />
        <FilterInput
          label="Failure message"
          value={failureMessage}
          onChange={(v) => {
            setFailureMessage(v);
            setPage(1);
          }}
          placeholder="e.g. not enough space"
          debounceMs={300}
        />
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Since</label>
          <input
            type="datetime-local"
            value={since}
            onChange={(e) => {
              setSince(e.target.value);
              setPage(1);
            }}
            className="block rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>
      </div>

      {/* Table */}
      {loading && <LoadingSpinner message="Loading run events…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {rows.length === 0 ? (
            <EmptyState
              title="No run events found"
              description="Adjust filters, Refresh, or wait for ingest telemetry."
            />
          ) : (
            <div className="overflow-x-auto rounded-lg border border-gray-200">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 text-left text-xs text-gray-500">
                  <tr>
                    <SortableColumnHeader
                      label="Organisation"
                      field="organisation"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      className="px-3 py-2"
                    />
                    <SortableColumnHeader
                      label="Node"
                      field="node_name"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      className="px-3 py-2"
                    />
                    <SortableColumnHeader
                      label="Status"
                      field="status"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      className="px-3 py-2"
                    />
                    <SortableColumnHeader
                      label="Chef"
                      field="chef_version"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      className="px-3 py-2"
                    />
                    <SortableColumnHeader
                      label="Ended"
                      field="end_time"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      className="px-3 py-2"
                    />
                    <th className="px-3 py-2">Failure</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r, i) => {
                    // Runs tab: click expands failure detail inline. Nodes tab:
                    // click drills into the node's full run history.
                    const expandable = tab === "runs" && !!r.error;
                    const isOpen = expandable && expandedRun === r.run_id;
                    const clickable = tab === "nodes" || expandable;
                    return (
                      <Fragment
                        key={tab === "runs" ? r.run_id : `${r.organisation}/${r.node_name}/${i}`}
                      >
                        <tr
                          className={`border-t border-gray-100 ${
                            clickable ? "cursor-pointer hover:bg-blue-50/40" : ""
                          }`}
                          onClick={() => {
                            if (tab === "nodes") {
                              goToNode(r.organisation, r.node_name);
                            } else if (expandable) {
                              setExpandedRun(isOpen ? null : r.run_id);
                            }
                          }}
                        >
                          <td className="px-3 py-1.5 text-gray-600">{r.organisation}</td>
                          <td
                            className={`px-3 py-1.5 font-medium ${
                              tab === "nodes" ? "text-blue-700" : "text-gray-800"
                            }`}
                          >
                            {expandable && (
                              <span className="mr-1 text-gray-400">
                                {isOpen ? "▾" : "▸"}
                              </span>
                            )}
                            {r.node_name}
                          </td>
                          <td className="px-3 py-1.5">{statusBadge(r.status)}</td>
                          <td className="px-3 py-1.5">{r.chef_version || "—"}</td>
                          <td className="px-3 py-1.5 whitespace-nowrap text-gray-600">
                            {r.end_time ? new Date(r.end_time).toLocaleString() : "—"}
                          </td>
                          <td className="px-3 py-1.5 text-xs text-gray-600">
                            {r.error?.message ? (
                              <span>
                                <span className="font-medium text-red-700">
                                  {r.error.class}
                                </span>
                                : {r.error.message}
                                {r.failed_resource?.cookbook_name && (
                                  <span className="ml-1 text-gray-400">
                                    ({r.failed_resource.cookbook_name}::
                                    {r.failed_resource.recipe_name})
                                  </span>
                                )}
                              </span>
                            ) : (
                              "—"
                            )}
                          </td>
                        </tr>
                        {isOpen && r.error && (
                          <tr className="bg-red-50/40">
                            <td colSpan={6} className="px-3 py-2">
                              <div className="text-sm font-medium text-red-800">
                                {r.error.class}: {r.error.message}
                              </div>
                              {r.failed_resource?.type && (
                                <div className="mt-1 text-xs text-gray-600">
                                  Failed resource:{" "}
                                  <span className="font-mono">
                                    {r.failed_resource.type}[{r.failed_resource.name}]
                                  </span>{" "}
                                  in {r.failed_resource.cookbook_name}::
                                  {r.failed_resource.recipe_name}
                                </div>
                              )}
                              {r.error.backtrace && r.error.backtrace.length > 0 && (
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
          {pagination && (
            <Pagination pagination={pagination} onPageChange={setPage} />
          )}
        </>
      )}
    </div>
  );
}
