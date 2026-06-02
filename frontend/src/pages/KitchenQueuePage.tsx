// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback, useRef } from "react";
import { fetchKitchenQueue, cancelKitchenQueueItem, retryKitchenQueueItem } from "../api";
import type { KitchenQueueItem, KitchenQueueStats } from "../types";
import { LoadingSpinner, ErrorAlert } from "../components/Feedback";
import { useWebSocket } from "../hooks/useWebSocket";

const STATUS_COLORS: Record<string, string> = {
  queued: "bg-blue-100 text-blue-700",
  running: "bg-yellow-100 text-yellow-800",
  completed: "bg-green-100 text-green-700",
  failed: "bg-red-100 text-red-700",
  cancelled: "bg-gray-100 text-gray-700",
  interrupted: "bg-orange-100 text-orange-700",
};

const ALL_STATUSES = ["active", "all", "queued", "running", "completed", "failed", "cancelled", "interrupted"] as const;

function formatTime(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}

function computeDuration(started?: string, completed?: string): string {
  if (!started) return "—";
  const start = new Date(started).getTime();
  const end = completed ? new Date(completed).getTime() : Date.now();
  const seconds = Math.floor((end - start) / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const rem = seconds % 60;
  return `${minutes}m ${rem}s`;
}

export default function KitchenQueuePage() {
  const [items, setItems] = useState<KitchenQueueItem[]>([]);
  const [stats, setStats] = useState<KitchenQueueStats>({ queued: 0, running: 0, workers_active: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>("active");
  const [typeFilter, setTypeFilter] = useState<string>("all");
  const [repoFilter, setRepoFilter] = useState("");
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const { onEvent } = useWebSocket();

  const loadQueue = useCallback(async () => {
    try {
      const params: { repo?: string; type?: "git" | "node"; status?: string } = {};
      if (statusFilter === "active") {
        params.status = "queued,running";
      } else if (statusFilter !== "all") {
        params.status = statusFilter;
      }
      if (typeFilter !== "all") params.type = typeFilter as "git" | "node";
      if (repoFilter.trim()) params.repo = repoFilter.trim();

      const response = await fetchKitchenQueue(params);
      setItems(response.items);
      setStats(response.stats);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load kitchen queue");
    } finally {
      setLoading(false);
    }
  }, [statusFilter, typeFilter, repoFilter]);

  useEffect(() => {
    loadQueue();
  }, [loadQueue]);

  // Auto-refresh when there are active items
  useEffect(() => {
    const hasActive = stats.queued > 0 || stats.running > 0;
    if (hasActive) {
      intervalRef.current = setInterval(loadQueue, 5000);
    } else if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [stats.queued, stats.running, loadQueue]);

  // WebSocket live updates
  useEffect(() => {
    const unsubscribe = onEvent("kitchen_queue_update", () => {
      loadQueue();
    });
    return unsubscribe;
  }, [onEvent, loadQueue]);

  const handleCancel = async (id: string) => {
    try {
      await cancelKitchenQueueItem(id);
      await loadQueue();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to cancel item");
    }
  };

  const handleRetry = async (id: string) => {
    try {
      await retryKitchenQueueItem(id);
      await loadQueue();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to retry item");
    }
  };

  if (loading) return <LoadingSpinner message="Loading kitchen queue..." />;

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold text-gray-800">Kitchen Queue</h2>

      {error && <ErrorAlert message={error} onRetry={() => setError(null)} />}

      {/* Stats Cards */}
      <div className="flex flex-row gap-4">
        <div className="rounded-lg border border-gray-200 px-4 py-3">
          <p className="text-sm text-gray-500">Queued</p>
          <p className="text-2xl font-semibold text-blue-700">{stats.queued}</p>
        </div>
        <div className="rounded-lg border border-gray-200 px-4 py-3">
          <p className="text-sm text-gray-500">Running</p>
          <p className="text-2xl font-semibold text-yellow-700">{stats.running}</p>
        </div>
        <div className="rounded-lg border border-gray-200 px-4 py-3">
          <p className="text-sm text-gray-500">Workers Active</p>
          <p className="text-2xl font-semibold text-green-700">{stats.workers_active}</p>
        </div>
      </div>

      {/* Filter Controls */}
      <div className="flex flex-wrap items-center gap-4">
        <div>
          <label htmlFor="status-filter" className="block text-sm font-medium text-gray-700">
            Status
          </label>
          <select
            id="status-filter"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="mt-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
          >
            {ALL_STATUSES.map((s) => (
              <option key={s} value={s}>
                {s === "active" ? "Active (Queued + Running)" : s === "all" ? "All" : s.charAt(0).toUpperCase() + s.slice(1)}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="type-filter" className="block text-sm font-medium text-gray-700">
            Type
          </label>
          <select
            id="type-filter"
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value)}
            className="mt-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
          >
            <option value="all">All</option>
            <option value="git">Git</option>
            <option value="node">Node</option>
          </select>
        </div>
        <div>
          <label htmlFor="repo-filter" className="block text-sm font-medium text-gray-700">
            Repo
          </label>
          <input
            id="repo-filter"
            type="text"
            value={repoFilter}
            onChange={(e) => setRepoFilter(e.target.value)}
            placeholder="Filter by repo name"
            className="mt-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
          />
        </div>
      </div>

      {/* Queue Table */}
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200">
          <thead>
            <tr className="text-left text-sm font-medium text-gray-500">
              <th className="px-3 py-2">Status</th>
              <th className="px-3 py-2">Type</th>
              <th className="px-3 py-2">Repo / Node</th>
              <th className="px-3 py-2">Instance</th>
              <th className="px-3 py-2">Priority</th>
              <th className="px-3 py-2">Enqueued</th>
              <th className="px-3 py-2">Started</th>
              <th className="px-3 py-2">Duration</th>
              <th className="px-3 py-2">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200">
            {items.length === 0 && (
              <tr>
                <td colSpan={9} className="px-3 py-6 text-center text-sm text-gray-500">
                  No queue items found.
                </td>
              </tr>
            )}
            {items.map((item) => (
              <QueueRow
                key={item.id}
                item={item}
                expanded={expandedId === item.id}
                onToggle={() => setExpandedId(expandedId === item.id ? null : item.id)}
                onCancel={handleCancel}
                onRetry={handleRetry}
              />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function QueueRow({
  item,
  expanded,
  onToggle,
  onCancel,
  onRetry,
}: {
  item: KitchenQueueItem;
  expanded: boolean;
  onToggle: () => void;
  onCancel: (id: string) => void;
  onRetry: (id: string) => void;
}) {
  const canCancel = item.status === "queued" || item.status === "running";
  const canRetry = item.status === "failed" || item.status === "cancelled" || item.status === "interrupted";
  const repoOrNode = item.run_type === "git" ? item.git_repo_name ?? "—" : item.node_name ?? "—";

  return (
    <>
      <tr
        className="cursor-pointer hover:bg-gray-50 text-sm"
        onClick={onToggle}
      >
        <td className="px-3 py-2">
          <span
            className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_COLORS[item.status] ?? ""}`}
          >
            {item.status}
          </span>
        </td>
        <td className="px-3 py-2">{item.run_type}</td>
        <td className="px-3 py-2 max-w-[200px] truncate" title={repoOrNode}>
          {repoOrNode}
        </td>
        <td className="px-3 py-2">{item.instance_name ?? "—"}</td>
        <td className="px-3 py-2">{item.priority}</td>
        <td className="px-3 py-2 whitespace-nowrap">{formatTime(item.enqueued_at)}</td>
        <td className="px-3 py-2 whitespace-nowrap">{formatTime(item.started_at)}</td>
        <td className="px-3 py-2 whitespace-nowrap">{computeDuration(item.started_at, item.completed_at)}</td>
        <td className="px-3 py-2 space-x-2" onClick={(e) => e.stopPropagation()}>
          {canCancel && (
            <button
              className="text-sm text-blue-600 hover:text-blue-800"
              onClick={() => onCancel(item.id)}
            >
              Cancel
            </button>
          )}
          {canRetry && (
            <button
              className="text-sm text-blue-600 hover:text-blue-800"
              onClick={() => onRetry(item.id)}
            >
              Retry
            </button>
          )}
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={9} className="bg-gray-50 px-6 py-3">
            {item.error_message && (
              <div className="mb-2">
                <span className="text-xs font-medium text-red-700">Error: </span>
                <span className="text-xs text-red-600">{item.error_message}</span>
              </div>
            )}
            {item.output ? (
              <pre className="max-h-60 overflow-auto rounded bg-gray-900 p-3 text-xs text-gray-100">
                {item.output}
              </pre>
            ) : (
              <p className="text-xs text-gray-500">No output available.</p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}
