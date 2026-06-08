// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback, useRef } from "react";
import {
  listKitchenBatches,
  createKitchenBatch,
  getKitchenBatch,
  runKitchenBatch,
  cancelKitchenBatch,
  deleteKitchenBatch,
  listExcludedGitRepos,
  excludeGitRepo,
  clearGitRepoExclusion,
  fetchBatchProgress,
  fetchBatchInstances,
  fetchTestKitchenConfig,
} from "../api";
import type {
  KitchenBatch,
  KitchenBatchDetail,
  KitchenBatchRequest,
  BatchFilters,
  ResolvedCookbook,
  GitRepoListItem,
  BatchProgress,
  KitchenBatchInstance,
} from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { useWebSocket } from "../hooks/useWebSocket";

/** Extract the batch_id from a WebSocket event payload, if present. */
function eventBatchId(data: unknown): string | undefined {
  if (data && typeof data === "object" && "batch_id" in data) {
    const v = (data as Record<string, unknown>).batch_id;
    return typeof v === "string" ? v : undefined;
  }
  return undefined;
}

// ---------------------------------------------------------------------------
// Status badge helpers
// ---------------------------------------------------------------------------

const STATUS_COLORS: Record<string, string> = {
  draft: "bg-gray-100 text-gray-700",
  previewing: "bg-blue-100 text-blue-700",
  preparing: "bg-indigo-100 text-indigo-700",
  running: "bg-yellow-100 text-yellow-800",
  completed: "bg-green-100 text-green-700",
  cancelled: "bg-red-100 text-red-700",
  failed: "bg-red-200 text-red-800",
};

const INSTANCE_STATUS_COLORS: Record<string, string> = {
  pending: "bg-gray-100 text-gray-600",
  running: "bg-yellow-100 text-yellow-800",
  passed: "bg-green-100 text-green-700",
  failed: "bg-red-200 text-red-800",
  errored: "bg-orange-100 text-orange-700",
  timed_out: "bg-yellow-100 text-yellow-800",
  network_timeout: "bg-violet-100 text-violet-700",
  cancelled: "bg-red-100 text-red-700",
};

function InstanceStatusBadge({ status }: { status: string }) {
  const cls = INSTANCE_STATUS_COLORS[status] ?? "bg-gray-100 text-gray-600";
  return (
    <span
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}
    >
      {status}
    </span>
  );
}

function BatchProgressBar({ progress }: { progress: BatchProgress }) {
  const total = progress.total || 1;
  const pct = (n: number) => `${((n / total) * 100).toFixed(1)}%`;
  return (
    <div className="space-y-2">
      <div className="flex h-4 w-full overflow-hidden rounded-full bg-gray-200">
        {progress.passed > 0 && (
          <div
            className="bg-green-500"
            style={{ width: pct(progress.passed) }}
            title={`Passed: ${progress.passed}`}
          />
        )}
        {progress.failed > 0 && (
          <div
            className="bg-red-500"
            style={{ width: pct(progress.failed) }}
            title={`Failed: ${progress.failed}`}
          />
        )}
        {progress.timed_out > 0 && (
          <div
            className="bg-yellow-400"
            style={{ width: pct(progress.timed_out) }}
            title={`Timed out: ${progress.timed_out}`}
          />
        )}
        {progress.network_timeout > 0 && (
          <div
            className="bg-violet-400"
            style={{ width: pct(progress.network_timeout) }}
            title={`Network timeout: ${progress.network_timeout}`}
          />
        )}
        {progress.errored > 0 && (
          <div
            className="bg-orange-400"
            style={{ width: pct(progress.errored) }}
            title={`Errored: ${progress.errored}`}
          />
        )}
        {progress.pending > 0 && (
          <div
            className="bg-gray-300"
            style={{ width: pct(progress.pending) }}
            title={`Pending: ${progress.pending}`}
          />
        )}
      </div>
      <div className="flex flex-wrap gap-4 text-xs text-gray-600">
        <span>✅ {progress.passed} passed</span>
        <span>❌ {progress.failed} failed</span>
        <span>⏱ {progress.timed_out} timed out</span>
        <span>🔌 {progress.network_timeout} network timeout</span>
        <span>⚠️ {progress.errored} errored</span>
        <span>⏳ {progress.pending} pending</span>
        <span className="font-medium">Total: {progress.total}</span>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const cls = STATUS_COLORS[status] ?? "bg-gray-100 text-gray-600";
  return (
    <span
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}
    >
      {status}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

const BTN_PRIMARY =
  "inline-flex items-center rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50";

const BTN_SECONDARY =
  "inline-flex items-center rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-50";

const BTN_DANGER =
  "inline-flex items-center rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-red-700 disabled:opacity-50";

const BTN_SUCCESS =
  "inline-flex items-center rounded-md bg-green-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-green-700 disabled:opacity-50";

const PREVIOUS_STATUS_OPTIONS = ["any", "passed", "failed", "untested"];

type HasTestSuiteFilter = "any" | "yes" | "no";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function splitComma(val: string): string[] {
  return val
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

function joinComma(arr?: string[]): string {
  return arr?.join(", ") ?? "";
}

function formatDate(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface BatchFormData {
  name: string;
  cookbookNames: string;
  excludeCookbooks: string;
  hasTestSuite: HasTestSuiteFilter;
  previousStatus: string;
  maxCount: string;
  dryRun: boolean;
}

function emptyForm(): BatchFormData {
  return {
    name: "",
    cookbookNames: "",
    excludeCookbooks: "",
    hasTestSuite: "any",
    previousStatus: "any",
    maxCount: "",
    dryRun: true,
  };
}

function formToRequest(form: BatchFormData): KitchenBatchRequest {
  const filters: BatchFilters = {};
  const names = splitComma(form.cookbookNames);
  if (names.length > 0) filters.cookbook_names = names;
  const excl = splitComma(form.excludeCookbooks);
  if (excl.length > 0) filters.exclude_cookbooks = excl;
  if (form.hasTestSuite === "yes") filters.has_test_suite = true;
  if (form.hasTestSuite === "no") filters.has_test_suite = false;
  if (form.previousStatus !== "any")
    filters.previous_status = form.previousStatus;
  return {
    name: form.name,
    filters,
    max_count: form.maxCount ? parseInt(form.maxCount, 10) : null,
    dry_run: form.dryRun,
  };
}

export function batchToForm(b: KitchenBatch): BatchFormData {
  return {
    name: b.name,
    cookbookNames: joinComma(b.filters.cookbook_names),
    excludeCookbooks: joinComma(b.filters.exclude_cookbooks),
    hasTestSuite:
      b.filters.has_test_suite === true
        ? "yes"
        : b.filters.has_test_suite === false
          ? "no"
          : "any",
    previousStatus: b.filters.previous_status ?? "any",
    maxCount: b.max_count != null ? String(b.max_count) : "",
    dryRun: b.dry_run,
  };
}

// ---------------------------------------------------------------------------
// Batch Form
// ---------------------------------------------------------------------------

function BatchForm({
  initial,
  saving,
  runEnabled,
  onSave,
  onSaveAndRun,
  onCancel,
}: {
  initial: BatchFormData;
  saving: boolean;
  runEnabled: boolean;
  onSave: (form: BatchFormData) => void;
  onSaveAndRun: (form: BatchFormData) => void;
  onCancel: () => void;
}) {
  const [form, setForm] = useState<BatchFormData>(initial);

  function update(partial: Partial<BatchFormData>) {
    setForm((prev) => ({ ...prev, ...partial }));
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-gray-500">
        {initial.name ? "Edit Batch" : "New Batch"}
      </h3>

      <div className="space-y-4">
        {/* Name */}
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">
            Batch Name
          </label>
          <input
            type="text"
            value={form.name}
            onChange={(e) => update({ name: e.target.value })}
            disabled={saving}
            className={INPUT_CLASS}
            placeholder="e.g. Phase 1 — Linux cookbooks"
          />
        </div>

        {/* Filters */}
        <fieldset className="rounded-md border border-gray-200 p-4">
          <legend className="px-1 text-xs font-medium uppercase tracking-wider text-gray-400">
            Filters
          </legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700">
                Cookbook Names
              </label>
              <input
                type="text"
                value={form.cookbookNames}
                onChange={(e) => update({ cookbookNames: e.target.value })}
                disabled={saving}
                className={INPUT_CLASS}
                placeholder="supports * wildcards, comma-separated"
              />
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700">
                Exclude Cookbooks
              </label>
              <input
                type="text"
                value={form.excludeCookbooks}
                onChange={(e) => update({ excludeCookbooks: e.target.value })}
                disabled={saving}
                className={INPUT_CLASS}
                placeholder="comma-separated"
              />
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700">
                Has Test Suite
              </label>
              <select
                value={form.hasTestSuite}
                onChange={(e) =>
                  update({ hasTestSuite: e.target.value as HasTestSuiteFilter })
                }
                disabled={saving}
                className={INPUT_CLASS}
              >
                <option value="any">Any</option>
                <option value="yes">Yes</option>
                <option value="no">No</option>
              </select>
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700">
                Previous Status
              </label>
              <select
                value={form.previousStatus}
                onChange={(e) => update({ previousStatus: e.target.value })}
                disabled={saving}
                className={INPUT_CLASS}
              >
                {PREVIOUS_STATUS_OPTIONS.map((s) => (
                  <option key={s} value={s}>
                    {s.charAt(0).toUpperCase() + s.slice(1)}
                  </option>
                ))}
              </select>
            </div>
          </div>
        </fieldset>

        {/* Limits */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">
              Max Count
            </label>
            <input
              type="number"
              min={1}
              value={form.maxCount}
              onChange={(e) => update({ maxCount: e.target.value })}
              disabled={saving}
              className={INPUT_CLASS}
              placeholder="unlimited"
            />
          </div>
          <div className="flex items-end gap-2 pb-2">
            <input
              type="checkbox"
              id="batch-dry-run"
              checked={form.dryRun}
              onChange={(e) => update({ dryRun: e.target.checked })}
              disabled={saving}
              className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            <label
              htmlFor="batch-dry-run"
              className="text-sm font-medium text-gray-700"
            >
              Dry Run
            </label>
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-3 pt-2">
          <button
            className={BTN_SUCCESS}
            disabled={saving || !form.name.trim() || !runEnabled}
            onClick={() => onSaveAndRun(form)}
            title={
              runEnabled
                ? "Create the batch and start it immediately"
                : "Enable Test Kitchen in settings to run batches"
            }
          >
            {saving ? "Working…" : "Create & Run"}
          </button>
          <button
            className={BTN_PRIMARY}
            disabled={saving || !form.name.trim()}
            onClick={() => onSave(form)}
          >
            {saving ? "Saving…" : "Save"}
          </button>
          <button
            className={BTN_SECONDARY}
            disabled={saving}
            onClick={onCancel}
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Per-instance results table
// ---------------------------------------------------------------------------

function groupInstancesByRepo(
  instances: KitchenBatchInstance[],
): [string, KitchenBatchInstance[]][] {
  const groups = new Map<string, KitchenBatchInstance[]>();
  for (const inst of instances) {
    const arr = groups.get(inst.git_repo_name);
    if (arr) arr.push(inst);
    else groups.set(inst.git_repo_name, [inst]);
  }
  return Array.from(groups.entries()).sort(([a], [b]) => a.localeCompare(b));
}

function summariseStatuses(instances: KitchenBatchInstance[]): string {
  const counts = new Map<string, number>();
  for (const inst of instances) {
    counts.set(inst.status, (counts.get(inst.status) ?? 0) + 1);
  }
  return Array.from(counts.entries())
    .map(([status, n]) => `${n} ${status}`)
    .join(", ");
}

function BatchInstancesTable({
  instances,
}: {
  instances: KitchenBatchInstance[];
}) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const groups = groupInstancesByRepo(instances);

  function toggle(repo: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(repo)) next.delete(repo);
      else next.add(repo);
      return next;
    });
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-200 px-4 py-3">
        <h4 className="text-sm font-semibold text-gray-700">
          Instance Results ({instances.length})
        </h4>
      </div>
      <div className="divide-y divide-gray-100">
        {groups.map(([repo, rows]) => {
          const isOpen = expanded.has(repo);
          return (
            <div key={repo}>
              <button
                className="flex w-full items-center justify-between px-4 py-2 text-left hover:bg-gray-50"
                onClick={() => toggle(repo)}
                aria-expanded={isOpen}
              >
                <span className="flex items-center gap-2">
                  <span className="text-xs text-gray-400">
                    {isOpen ? "▲" : "▼"}
                  </span>
                  <span className="font-medium text-gray-800">{repo}</span>
                  <span className="text-xs text-gray-500">
                    ({rows.length})
                  </span>
                </span>
                <span className="text-xs text-gray-500">
                  {summariseStatuses(rows)}
                </span>
              </button>

              {isOpen && (
                <div className="overflow-x-auto bg-gray-50/50 px-4 pb-3">
                  <table className="min-w-full text-sm">
                    <thead>
                      <tr className="text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                        <th className="px-2 py-1">Instance</th>
                        <th className="px-2 py-1">Suite</th>
                        <th className="px-2 py-1">Platform</th>
                        <th className="px-2 py-1">Status</th>
                        <th className="px-2 py-1">Error</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-100">
                      {rows.map((inst) => (
                        <tr key={inst.id} className="hover:bg-white">
                          <td className="whitespace-nowrap px-2 py-1 font-medium text-gray-800">
                            {inst.instance_name}
                          </td>
                          <td className="whitespace-nowrap px-2 py-1 text-gray-600">
                            {inst.suite_name}
                          </td>
                          <td className="whitespace-nowrap px-2 py-1 text-gray-600">
                            {inst.platform_name}
                          </td>
                          <td className="whitespace-nowrap px-2 py-1">
                            <InstanceStatusBadge status={inst.status} />
                          </td>
                          <td
                            className="max-w-xs truncate px-2 py-1 text-gray-500"
                            title={inst.error_message || ""}
                          >
                            {inst.error_message || "—"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Batch Detail View
// ---------------------------------------------------------------------------

function BatchDetailView({
  batch,
  onRun,
  onCancel,
  onDelete,
  onBack,
  onBatchComplete,
  busy,
}: {
  batch: KitchenBatchDetail;
  onRun: () => void;
  onCancel: () => void;
  onDelete: () => void;
  onBack: () => void;
  onBatchComplete: () => void;
  busy: boolean;
}) {
  const [progress, setProgress] = useState<BatchProgress | null>(null);
  const [instances, setInstances] = useState<KitchenBatchInstance[]>([]);
  const [cancelling, setCancelling] = useState(false);
  const { onEvent } = useWebSocket();

  const isTerminal =
    batch.status === "completed" ||
    batch.status === "cancelled" ||
    batch.status === "failed";

  // Clear the optimistic cancelling state once the batch reaches a terminal
  // status (the parent refetch / batch_complete event delivers it).
  useEffect(() => {
    if (isTerminal) setCancelling(false);
  }, [isTerminal]);

  function handleCancelClick() {
    if (
      !window.confirm(
        "Cancel this batch? Pending instances will be cancelled and in-flight ones interrupted.",
      )
    )
      return;
    setCancelling(true);
    onCancel();
  }

  const refresh = useCallback(() => {
    fetchBatchProgress(batch.id)
      .then(setProgress)
      .catch(() => {});
    fetchBatchInstances(batch.id)
      .then(setInstances)
      .catch(() => {});
  }, [batch.id]);

  // Initial fetch + 5s poll fallback while the batch is active.
  useEffect(() => {
    const active = batch.status === "running" || batch.status === "preparing";
    const hasRun =
      active ||
      batch.status === "completed" ||
      batch.status === "cancelled" ||
      batch.status === "failed";
    if (!hasRun) return;

    refresh();

    if (active) {
      const interval = setInterval(refresh, 5000);
      return () => clearInterval(interval);
    }
  }, [batch.id, batch.status, refresh]);

  // Live updates: refresh on backend batch events for this batch. On
  // completion, ask the parent to refetch the detail so the status flips
  // (which also stops the poll above).
  useEffect(() => {
    const unsubProgress = onEvent("batch_progress", (data) => {
      if (eventBatchId(data) === batch.id) refresh();
    });
    const unsubComplete = onEvent("batch_complete", (data) => {
      if (eventBatchId(data) !== batch.id) return;
      refresh();
      onBatchComplete();
    });
    return () => {
      unsubProgress();
      unsubComplete();
    };
  }, [batch.id, onEvent, refresh, onBatchComplete]);

  const est = batch.estimate;
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h3 className="text-lg font-semibold text-gray-800">{batch.name}</h3>
          <div className="mt-1 flex flex-wrap items-center gap-3 text-sm text-gray-500">
            {cancelling && !isTerminal ? (
              <span className="inline-flex rounded-full bg-orange-100 px-2 py-0.5 text-xs font-medium text-orange-700">
                cancelling…
              </span>
            ) : (
              <StatusBadge status={batch.status} />
            )}
            {batch.dry_run && (
              <span className="rounded-full bg-purple-100 px-2 py-0.5 text-xs font-medium text-purple-700">
                dry run
              </span>
            )}
            {batch.created_by && <span>by {batch.created_by}</span>}
            <span>Created {formatDate(batch.created_at)}</span>
            {batch.started_at && (
              <span>Started {formatDate(batch.started_at)}</span>
            )}
            {batch.completed_at && (
              <span>Completed {formatDate(batch.completed_at)}</span>
            )}
          </div>
        </div>
        <button className={BTN_SECONDARY} onClick={onBack} disabled={busy}>
          ← Back
        </button>
      </div>

      {/* Filters summary */}
      <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-400">
          Filters
        </h4>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm sm:grid-cols-3">
          {batch.filters.cookbook_names && (
            <>
              <dt className="font-medium text-gray-600">Cookbook Names</dt>
              <dd className="col-span-1 text-gray-800 sm:col-span-2">
                {batch.filters.cookbook_names.join(", ")}
              </dd>
            </>
          )}
          {batch.filters.exclude_cookbooks && (
            <>
              <dt className="font-medium text-gray-600">Excluded</dt>
              <dd className="col-span-1 text-gray-800 sm:col-span-2">
                {batch.filters.exclude_cookbooks.join(", ")}
              </dd>
            </>
          )}
          {batch.filters.has_test_suite != null && (
            <>
              <dt className="font-medium text-gray-600">Has Test Suite</dt>
              <dd className="text-gray-800 sm:col-span-2">
                {batch.filters.has_test_suite ? "Yes" : "No"}
              </dd>
            </>
          )}
          {batch.filters.previous_status && (
            <>
              <dt className="font-medium text-gray-600">Previous Status</dt>
              <dd className="text-gray-800 sm:col-span-2">
                {batch.filters.previous_status}
              </dd>
            </>
          )}
          {batch.max_count != null && (
            <>
              <dt className="font-medium text-gray-600">Max Count</dt>
              <dd className="text-gray-800 sm:col-span-2">{batch.max_count}</dd>
            </>
          )}
        </dl>
      </div>

      {/* Progress bar */}
      {progress && progress.total > 0 && (
        <BatchProgressBar progress={progress} />
      )}

      {/* Per-instance results */}
      {instances.length > 0 && <BatchInstancesTable instances={instances} />}

      {/* Estimate summary */}
      {est && (
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
              <div className="text-xs font-medium uppercase tracking-wider text-gray-400">
                Total Cookbooks
              </div>
              <div className="mt-1 text-2xl font-bold tabular-nums text-blue-700">
                {est.total_cookbooks.toLocaleString()}
              </div>
            </div>
            <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
              <div className="text-xs font-medium uppercase tracking-wider text-gray-400">
                Estimated VMs
              </div>
              <div className="mt-1 text-2xl font-bold tabular-nums text-green-700">
                {est.total_estimated_vms.toLocaleString()}
              </div>
            </div>
            {est.skipped_cookbooks != null && est.skipped_cookbooks > 0 && (
              <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
                <div className="text-xs font-medium uppercase tracking-wider text-gray-400">
                  Skipped
                </div>
                <div className="mt-1 text-2xl font-bold tabular-nums text-amber-600">
                  {est.skipped_cookbooks.toLocaleString()}
                </div>
              </div>
            )}
          </div>

          {/* Per-platform breakdown */}
          {est.per_platform && Object.keys(est.per_platform).length > 0 && (
            <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-400">
                VMs per Platform
              </h4>
              <div className="flex flex-wrap gap-2">
                {Object.entries(est.per_platform)
                  .sort(([, a], [, b]) => b - a)
                  .map(([plat, count]) => (
                    <span
                      key={plat}
                      className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700"
                    >
                      {plat}: {count}
                    </span>
                  ))}
              </div>
            </div>
          )}

          {/* Resolved cookbooks table */}
          {est.cookbooks.length > 0 && (
            <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
              <div className="border-b border-gray-200 px-4 py-3">
                <h4 className="text-sm font-semibold text-gray-700">
                  Resolved Cookbooks ({est.cookbooks.length})
                </h4>
              </div>
              <div className="overflow-x-auto">
                <table className="min-w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                      <th className="px-4 py-2">Name</th>
                      <th className="px-4 py-2">Status</th>
                      <th className="px-4 py-2 text-right">Est. VMs</th>
                      <th className="px-4 py-2 text-right">Unmapped</th>
                      <th className="px-4 py-2 text-right">Excluded</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100">
                    {est.cookbooks.map((cb: ResolvedCookbook) => (
                      <tr key={cb.name} className="hover:bg-gray-50">
                        <td className="whitespace-nowrap px-4 py-2 font-medium text-gray-800">
                          {cb.name}
                        </td>
                        <td className="whitespace-nowrap px-4 py-2">
                          {cb.planning_status === "planned" ? (
                            <span className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
                              planned
                            </span>
                          ) : (
                            <span
                              className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700"
                              title={cb.planning_note || ""}
                            >
                              {cb.planning_status || "unknown"}
                            </span>
                          )}
                        </td>
                        <td className="whitespace-nowrap px-4 py-2 text-right tabular-nums text-gray-800">
                          {cb.estimated_vms}
                        </td>
                        <td className="whitespace-nowrap px-4 py-2 text-right tabular-nums text-gray-500">
                          {cb.unmapped || 0}
                        </td>
                        <td className="whitespace-nowrap px-4 py-2 text-right tabular-nums text-gray-500">
                          {(cb.excluded || 0) + (cb.user_excluded || 0)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Action buttons */}
      <div className="flex items-center gap-3">
        {batch.status === "draft" && (
          <button className={BTN_SUCCESS} disabled={busy} onClick={onRun}>
            {busy ? "Starting…" : "Run Batch"}
          </button>
        )}
        {(batch.status === "running" || batch.status === "previewing" || batch.status === "preparing") && (
          <button
            className={BTN_DANGER}
            disabled={busy || cancelling}
            onClick={handleCancelClick}
          >
            {cancelling ? "Cancelling…" : "Cancel Batch"}
          </button>
        )}
        {(batch.status === "draft" ||
          batch.status === "completed" ||
          batch.status === "cancelled" ||
          batch.status === "failed") && (
          <button className={BTN_DANGER} disabled={busy} onClick={onDelete}>
            Delete
          </button>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Excluded Cookbooks Section
// ---------------------------------------------------------------------------

function ExcludedCookbooksSection() {
  const [excluded, setExcluded] = useState<GitRepoListItem[]>([]);
  const [expanded, setExpanded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [addName, setAddName] = useState("");
  const [addReason, setAddReason] = useState("");
  const [addBy, setAddBy] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await listExcludedGitRepos();
      setExcluded(data);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to load exclusions");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (expanded) load();
  }, [expanded, load]);

  async function handleExclude() {
    if (!addName.trim() || !addReason.trim() || !addBy.trim()) return;
    setBusy(true);
    try {
      await excludeGitRepo(addName.trim(), {
        reason: addReason.trim(),
        excluded_by: addBy.trim(),
      });
      setShowAdd(false);
      setAddName("");
      setAddReason("");
      setAddBy("");
      await load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to exclude");
    } finally {
      setBusy(false);
    }
  }

  async function handleClear(name: string) {
    setBusy(true);
    try {
      await clearGitRepoExclusion(name);
      await load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to clear exclusion");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <button
        className="flex w-full items-center justify-between border-b border-gray-200 px-4 py-3 text-left"
        onClick={() => setExpanded((v) => !v)}
      >
        <h3 className="text-sm font-semibold text-gray-700">
          Excluded Cookbooks
        </h3>
        <span className="text-xs text-gray-400">{expanded ? "▲" : "▼"}</span>
      </button>

      {expanded && (
        <div className="p-4">
          {error && (
            <div className="mb-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
              {error}
            </div>
          )}

          {loading ? (
            <div className="py-4 text-center text-sm text-gray-400">
              Loading…
            </div>
          ) : excluded.length === 0 ? (
            <div className="py-4 text-center text-sm text-gray-400">
              No excluded cookbooks.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                    <th className="px-4 py-2">Name</th>
                    <th className="px-4 py-2">Clone Status</th>
                    <th className="px-4 py-2">TK Status</th>
                    <th className="px-4 py-2">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {excluded.map((r) => (
                    <tr key={r.name} className="hover:bg-gray-50">
                      <td className="whitespace-nowrap px-4 py-2 font-medium text-gray-800">
                        {r.name}
                      </td>
                      <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                        {r.clone_status || "—"}
                      </td>
                      <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                        {r.tk_status || "—"}
                      </td>
                      <td className="whitespace-nowrap px-4 py-2">
                        <button
                          className="text-sm font-medium text-red-600 hover:text-red-800 disabled:opacity-50"
                          disabled={busy}
                          onClick={() => handleClear(r.name)}
                        >
                          Clear
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Add exclusion form */}
          <div className="mt-4">
            {showAdd ? (
              <div className="space-y-3 rounded-md border border-gray-200 p-3">
                <div>
                  <label className="mb-1 block text-sm font-medium text-gray-700">
                    Cookbook / Repo Name
                  </label>
                  <input
                    type="text"
                    value={addName}
                    onChange={(e) => setAddName(e.target.value)}
                    disabled={busy}
                    className={INPUT_CLASS}
                    placeholder="e.g. my-cookbook"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-gray-700">
                    Reason
                  </label>
                  <input
                    type="text"
                    value={addReason}
                    onChange={(e) => setAddReason(e.target.value)}
                    disabled={busy}
                    className={INPUT_CLASS}
                    placeholder="Why is this cookbook excluded?"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-gray-700">
                    Excluded By
                  </label>
                  <input
                    type="text"
                    value={addBy}
                    onChange={(e) => setAddBy(e.target.value)}
                    disabled={busy}
                    className={INPUT_CLASS}
                    placeholder="Your name"
                  />
                </div>
                <div className="flex items-center gap-3">
                  <button
                    className={BTN_PRIMARY}
                    disabled={
                      busy ||
                      !addName.trim() ||
                      !addReason.trim() ||
                      !addBy.trim()
                    }
                    onClick={handleExclude}
                  >
                    {busy ? "Excluding…" : "Exclude"}
                  </button>
                  <button
                    className={BTN_SECONDARY}
                    disabled={busy}
                    onClick={() => setShowAdd(false)}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <button
                className={BTN_SECONDARY}
                onClick={() => setShowAdd(true)}
              >
                + Exclude Cookbook
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Page Component
// ---------------------------------------------------------------------------

export default function KitchenBatchesPage() {
  const [batches, setBatches] = useState<KitchenBatch[]>([]);
  const [selectedBatch, setSelectedBatch] = useState<KitchenBatchDetail | null>(
    null,
  );
  const [showCreate, setShowCreate] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [cancellingId, setCancellingId] = useState<string | null>(null);
  const [tkEnabled, setTkEnabled] = useState<boolean | null>(null);

  const loadBatches = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await listKitchenBatches();
      setBatches(data);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to load batches");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadBatches();
    fetchTestKitchenConfig()
      .then((res) => setTkEnabled(res.enabled === true))
      .catch(() => setTkEnabled(false));
  }, [loadBatches]);

  async function handleCreate(form: BatchFormData, run = false) {
    setBusy(true);
    setError(null);
    try {
      const created = await createKitchenBatch(formToRequest(form));
      setShowCreate(false);
      const detail = run
        ? await runKitchenBatch(created.id)
        : await getKitchenBatch(created.id);
      setSelectedBatch(detail);
      await loadBatches();
    } catch (e: unknown) {
      setError(
        e instanceof Error
          ? e.message
          : run
            ? "Failed to create and run batch"
            : "Failed to create batch",
      );
    } finally {
      setBusy(false);
    }
  }

  async function handleView(id: string) {
    setBusy(true);
    setError(null);
    try {
      const detail = await getKitchenBatch(id);
      setSelectedBatch(detail);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to load batch");
    } finally {
      setBusy(false);
    }
  }

  async function handleRun() {
    if (!selectedBatch) return;
    setBusy(true);
    setError(null);
    try {
      const updated = await runKitchenBatch(selectedBatch.id);
      setSelectedBatch(updated);
      await loadBatches();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to run batch");
    } finally {
      setBusy(false);
    }
  }

  async function handleCancel() {
    if (!selectedBatch) return;
    setBusy(true);
    setError(null);
    try {
      await cancelKitchenBatch(selectedBatch.id);
      // Refetch the full detail so the status badge flips to its terminal
      // value (and the estimate is preserved).
      const detail = await getKitchenBatch(selectedBatch.id);
      setSelectedBatch(detail);
      await loadBatches();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to cancel batch");
    } finally {
      setBusy(false);
    }
  }

  // Stable ref to the open batch id so handleBatchComplete can stay stable
  // (avoids re-subscribing the detail view's WebSocket listeners).
  const selectedIdRef = useRef<string | null>(null);
  useEffect(() => {
    selectedIdRef.current = selectedBatch?.id ?? null;
  }, [selectedBatch]);

  const handleBatchComplete = useCallback(async () => {
    const id = selectedIdRef.current;
    if (!id) return;
    try {
      const detail = await getKitchenBatch(id);
      setSelectedBatch(detail);
    } catch {
      /* ignore — the poll/next event will retry */
    }
    await loadBatches();
  }, [loadBatches]);

  async function handleDelete() {
    if (!selectedBatch) return;
    setBusy(true);
    setError(null);
    try {
      await deleteKitchenBatch(selectedBatch.id);
      setSelectedBatch(null);
      await loadBatches();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to delete batch");
    } finally {
      setBusy(false);
    }
  }

  // --- Loading ---
  if (loading && batches.length === 0) {
    return <LoadingSpinner message="Loading kitchen batches…" />;
  }

  // --- Detail view ---
  if (selectedBatch) {
    return (
      <div className="space-y-6">
        <div>
          <h2 className="text-xl font-semibold text-gray-800">
            Kitchen Batches
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Manage batch kitchen test runs.
          </p>
        </div>

        {error && <ErrorAlert message="Error" detail={error} />}

        <BatchDetailView
          batch={selectedBatch}
          onRun={handleRun}
          onCancel={handleCancel}
          onDelete={handleDelete}
          onBatchComplete={handleBatchComplete}
          onBack={() => setSelectedBatch(null)}
          busy={busy}
        />

        <ExcludedCookbooksSection />
      </div>
    );
  }

  // --- Create form ---
  if (showCreate) {
    return (
      <div className="space-y-6">
        <div>
          <h2 className="text-xl font-semibold text-gray-800">
            Kitchen Batches
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Manage batch kitchen test runs.
          </p>
        </div>

        {error && <ErrorAlert message="Error" detail={error} />}

        <BatchForm
          initial={emptyForm()}
          saving={busy}
          runEnabled={tkEnabled === true}
          onSave={(form) => handleCreate(form, false)}
          onSaveAndRun={(form) => handleCreate(form, true)}
          onCancel={() => setShowCreate(false)}
        />
      </div>
    );
  }

  // --- List view ---
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-xl font-semibold text-gray-800">
            Kitchen Batches
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Manage batch kitchen test runs.
          </p>
        </div>
        <button className={BTN_PRIMARY} onClick={() => setShowCreate(true)}>
          + New Batch
        </button>
      </div>

      {error && <ErrorAlert message="Error" detail={error} />}

      {/* Batch table */}
      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        {batches.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-gray-400">
            No batches yet. Create one to get started.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  <th className="px-4 py-2">Name</th>
                  <th className="px-4 py-2">Status</th>
                  <th className="px-4 py-2">Dry Run</th>
                  <th className="px-4 py-2 text-right">Max Count</th>
                  <th className="px-4 py-2">Created</th>
                  <th className="px-4 py-2">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {batches.map((b) => (
                  <tr key={b.id} className="hover:bg-gray-50">
                    <td className="whitespace-nowrap px-4 py-2 font-medium text-gray-800">
                      {b.name}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2">
                      <StatusBadge status={b.status} />
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                      {b.dry_run ? "Yes" : "No"}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-right tabular-nums text-gray-600">
                      {b.max_count ?? "—"}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-gray-600">
                      {formatDate(b.created_at)}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2">
                      <div className="flex items-center gap-2">
                        <button
                          className="text-sm font-medium text-blue-600 hover:text-blue-800"
                          onClick={() => handleView(b.id)}
                        >
                          View
                        </button>
                        {b.status === "draft" && (
                          <button
                            className="text-sm font-medium text-green-600 hover:text-green-800 disabled:opacity-50 disabled:cursor-not-allowed"
                            onClick={() => handleView(b.id)}
                            disabled={tkEnabled !== true}
                            title={
                              tkEnabled !== true
                                ? "Enable Test Kitchen in settings to run batches"
                                : "Run batch"
                            }
                          >
                            Run
                          </button>
                        )}
                        {(b.status === "running" ||
                          b.status === "previewing" ||
                          b.status === "preparing") && (
                          <button
                            className="text-sm font-medium text-red-600 hover:text-red-800 disabled:opacity-50 disabled:cursor-not-allowed"
                            disabled={cancellingId === b.id}
                            onClick={async () => {
                              if (
                                !window.confirm(
                                  "Cancel this batch? Pending instances will be cancelled and in-flight ones interrupted.",
                                )
                              )
                                return;
                              setCancellingId(b.id);
                              try {
                                await cancelKitchenBatch(b.id);
                                await loadBatches();
                              } catch (e: unknown) {
                                setError(
                                  e instanceof Error
                                    ? e.message
                                    : "Failed to cancel",
                                );
                              } finally {
                                setCancellingId(null);
                              }
                            }}
                          >
                            {cancellingId === b.id ? "Cancelling…" : "Cancel"}
                          </button>
                        )}
                        {(b.status === "draft" ||
                          b.status === "completed" ||
                          b.status === "cancelled" ||
                          b.status === "failed") && (
                          <button
                            className="text-sm font-medium text-red-600 hover:text-red-800"
                            onClick={async () => {
                              try {
                                await deleteKitchenBatch(b.id);
                                await loadBatches();
                              } catch (e: unknown) {
                                setError(
                                  e instanceof Error
                                    ? e.message
                                    : "Failed to delete",
                                );
                              }
                            }}
                          >
                            Delete
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Excluded cookbooks */}
      <ExcludedCookbooksSection />
    </div>
  );
}
