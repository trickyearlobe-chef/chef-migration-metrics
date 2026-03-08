import { useState, useEffect } from "react";
import { pollHealth } from "../api";
import type { HealthResponse } from "../types";

/**
 * Small status badge that polls GET /api/v1/health on an interval and
 * shows green (healthy) or red (unhealthy / unreachable).
 *
 * Displays the backend version string on hover via a tooltip.
 */
export function HealthBadge({ intervalMs = 30_000 }: { intervalMs?: number }) {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [reachable, setReachable] = useState(true);

  useEffect(() => {
    const stop = pollHealth((h) => {
      if (h) {
        setHealth(h);
        setReachable(true);
      } else {
        setReachable(false);
      }
    }, intervalMs);

    return stop;
  }, [intervalMs]);

  const isHealthy = reachable && health?.status === "healthy";
  const statusColor = isHealthy ? "bg-green-500" : "bg-red-500";
  const ringColor = isHealthy
    ? "ring-green-400/30"
    : "ring-red-400/30";
  const label = !reachable
    ? "Backend unreachable"
    : health?.status === "healthy"
      ? "Healthy"
      : `Unhealthy${health?.error ? `: ${health.error}` : ""}`;

  const tooltip = [
    label,
    health?.version ? `Version: ${health.version}` : null,
    health?.websocket_enabled != null
      ? `WebSocket: ${health.websocket_enabled ? "on" : "off"}`
      : null,
    health?.websocket_clients != null
      ? `WS clients: ${health.websocket_clients}`
      : null,
  ]
    .filter(Boolean)
    .join("\n");

  return (
    <span
      className="inline-flex items-center gap-1.5 text-xs text-gray-500"
      title={tooltip}
    >
      <span className="relative flex h-2.5 w-2.5">
        {/* Animated ping ring for healthy state */}
        {isHealthy && (
          <span
            className={`absolute inline-flex h-full w-full animate-ping rounded-full opacity-50 ${statusColor}`}
          />
        )}
        <span
          className={`relative inline-flex h-2.5 w-2.5 rounded-full ring-2 ${statusColor} ${ringColor}`}
        />
      </span>
      <span className="hidden sm:inline">
        {!reachable
          ? "Offline"
          : health?.status === "healthy"
            ? health.version ?? "OK"
            : "Unhealthy"}
      </span>
    </span>
  );
}
