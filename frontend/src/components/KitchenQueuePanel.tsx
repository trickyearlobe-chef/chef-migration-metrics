// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import {
  fetchKitchenQueue,
  cancelKitchenQueueItem,
  retryKitchenQueueItem,
} from "../api";
import type { KitchenQueueItem } from "../types";
import { useWebSocket } from "../hooks/useWebSocket";

interface KitchenQueuePanelProps {
  repoName: string;
}

const statusColors: Record<string, string> = {
  queued: "bg-yellow-100 text-yellow-800",
  running: "bg-blue-100 text-blue-800",
  completed: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
  cancelled: "bg-gray-100 text-gray-600",
  interrupted: "bg-orange-100 text-orange-800",
};

export function KitchenQueuePanel({ repoName }: KitchenQueuePanelProps) {
  const [items, setItems] = useState<KitchenQueueItem[]>([]);
  const [stats, setStats] = useState<{ queued: number; running: number; workers_active: number } | null>(null);
  const { onEvent } = useWebSocket();

  const refresh = useCallback(async () => {
    try {
      const resp = await fetchKitchenQueue({
        repo: repoName,
        status: "queued,running",
      });
      setItems(resp.items);
      setStats(resp.stats);
    } catch {
      // Silently fail — panel is supplementary
    }
  }, [repoName]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    return onEvent("kitchen_queue_update", () => {
      refresh();
    });
  }, [onEvent, refresh]);

  async function handleCancel(id: string) {
    try {
      await cancelKitchenQueueItem(id);
      refresh();
    } catch {
      // ignore
    }
  }

  async function handleRetry(id: string) {
    try {
      await retryKitchenQueueItem(id);
      refresh();
    } catch {
      // ignore
    }
  }

  if (items.length === 0) return null;

  return (
    <div className="mt-4 rounded-lg border border-gray-200 bg-white p-4">
      <div className="flex items-center justify-between mb-3">
        <h4 className="text-sm font-semibold text-gray-700">Queue</h4>
        {stats && (
          <span className="text-xs text-gray-500">
            {stats.running} running · {stats.queued} queued
          </span>
        )}
      </div>

      <div className="space-y-2">
        {items.map((item) => (
          <QueueItemRow
            key={item.id}
            item={item}
            onCancel={handleCancel}
            onRetry={handleRetry}
          />
        ))}
      </div>
    </div>
  );
}

function QueueItemRow({
  item,
  onCancel,
  onRetry,
}: {
  item: KitchenQueueItem;
  onCancel: (id: string) => void;
  onRetry: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  const label = item.run_type === "git"
    ? item.instance_name || `${item.suite_name}-${item.platform_name}`
    : `${item.organisation_name}/${item.node_name}`;

  const duration = item.started_at
    ? formatDuration(item.started_at, item.completed_at)
    : null;

  return (
    <div className="rounded border border-gray-100 p-2">
      <div className="flex items-center gap-2 text-xs">
        <span className={`inline-flex items-center rounded-full px-1.5 py-0.5 font-medium ${statusColors[item.status] || "bg-gray-100 text-gray-600"}`}>
          {item.status}
        </span>
        <span className="font-mono text-gray-700 truncate flex-1">{label}</span>
        {duration && <span className="text-gray-400">{duration}</span>}

        {(item.status === "queued" || item.status === "running") && (
          <button
            onClick={() => onCancel(item.id)}
            className="text-red-600 hover:text-red-800 font-medium"
          >
            Cancel
          </button>
        )}
        {(item.status === "failed" || item.status === "interrupted" || item.status === "cancelled") && (
          <button
            onClick={() => onRetry(item.id)}
            className="text-blue-600 hover:text-blue-800 font-medium"
          >
            Retry
          </button>
        )}

        {item.output && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="text-gray-500 hover:text-gray-700"
          >
            {expanded ? "▾" : "▸"}
          </button>
        )}
      </div>

      {item.error_message && (
        <p className="mt-1 text-xs text-red-600 truncate">{item.error_message}</p>
      )}

      {expanded && item.output && (
        <pre className="mt-2 max-h-48 overflow-auto rounded bg-gray-900 p-2 text-xs text-gray-200 font-mono">
          {item.output}
        </pre>
      )}
    </div>
  );
}

function formatDuration(startedAt: string, completedAt?: string): string {
  const start = new Date(startedAt).getTime();
  const end = completedAt ? new Date(completedAt).getTime() : Date.now();
  const seconds = Math.round((end - start) / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return `${minutes}m ${secs}s`;
}
