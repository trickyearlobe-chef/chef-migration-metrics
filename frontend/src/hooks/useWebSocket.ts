// Copyright 2025 Chef Migration Metrics Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { useState, useRef, useEffect, useCallback } from "react";

/** Options for configuring the WebSocket hook. */
export interface UseWebSocketOptions {
  /** Auto-connect on mount. Default: true */
  autoConnect?: boolean;
  /** Base reconnect delay in ms. Default: 3000 */
  reconnectDelay?: number;
  /** Max reconnect attempts before giving up. Default: 10 */
  maxReconnectAttempts?: number;
}

/** The current state of the WebSocket connection. */
export type ConnectionStatus = "connecting" | "connected" | "disconnected" | "error";

/** The shape of an incoming WebSocket message. */
interface WebSocketMessage {
  event: string;
  timestamp: string;
  data: unknown;
}

/** The shape of a stored subscription used for replay on reconnect. */
interface Subscription {
  event: string;
  filters?: Record<string, string>;
}

/** The return value of the useWebSocket hook. */
export interface UseWebSocketReturn {
  /** Current connection state. */
  status: ConnectionStatus;
  /** Subscribe to an event type with optional filters. */
  subscribe: (event: string, filters?: Record<string, string>) => void;
  /** Unsubscribe from an event type. */
  unsubscribe: (event: string) => void;
  /** Register a callback for incoming events. Returns a cleanup function. */
  onEvent: (event: string, callback: (data: unknown) => void) => () => void;
  /** Manually disconnect. */
  disconnect: () => void;
  /** Manually reconnect. */
  reconnect: () => void;
}

/** Maximum reconnect delay cap in milliseconds. */
const MAX_BACKOFF_MS = 30_000;

/**
 * React hook for managing a WebSocket connection to the migration metrics
 * backend. Handles automatic reconnection with exponential backoff,
 * subscription management, and event dispatching.
 */
export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const {
    autoConnect = true,
    reconnectDelay = 3000,
    maxReconnectAttempts = 10,
  } = options;

  const [status, setStatus] = useState<ConnectionStatus>("disconnected");

  // Mutable refs that persist across renders without triggering re-renders.
  const wsRef = useRef<WebSocket | null>(null);
  const subscriptionsRef = useRef<Map<string, Subscription>>(new Map());
  const listenersRef = useRef<Map<string, Set<(data: unknown) => void>>>(new Map());
  const attemptRef = useRef<number>(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const intentionalDisconnectRef = useRef<boolean>(false);

  // ── Helpers ──────────────────────────────────────────────────────────

  /** Build the WebSocket URL from the current page location. */
  const buildUrl = useCallback((): string => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    return protocol + "//" + window.location.host + "/api/v1/ws";
  }, []);

  /** Clear any pending reconnect timer. */
  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current !== null) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  }, []);

  /** Replay all active subscriptions over the given WebSocket. */
  const replaySubscriptions = useCallback((ws: WebSocket) => {
    subscriptionsRef.current.forEach((sub) => {
      ws.send(JSON.stringify({ action: "subscribe", event: sub.event, filters: sub.filters }));
    });
  }, []);

  /** Schedule a reconnect attempt using exponential backoff. */
  const scheduleReconnect = useCallback(
    (connectFn: () => void) => {
      if (intentionalDisconnectRef.current) {
        return;
      }
      if (attemptRef.current >= maxReconnectAttempts) {
        setStatus("error");
        return;
      }
      const delay = Math.min(reconnectDelay * Math.pow(2, attemptRef.current), MAX_BACKOFF_MS);
      attemptRef.current += 1;
      reconnectTimerRef.current = setTimeout(connectFn, delay);
    },
    [maxReconnectAttempts, reconnectDelay],
  );

  // ── Core connect logic ──────────────────────────────────────────────

  const connect = useCallback(() => {
    // Tear down any existing connection first.
    if (wsRef.current) {
      wsRef.current.onopen = null;
      wsRef.current.onclose = null;
      wsRef.current.onerror = null;
      wsRef.current.onmessage = null;
      if (
        wsRef.current.readyState === WebSocket.OPEN ||
        wsRef.current.readyState === WebSocket.CONNECTING
      ) {
        wsRef.current.close();
      }
      wsRef.current = null;
    }

    clearReconnectTimer();
    intentionalDisconnectRef.current = false;
    setStatus("connecting");

    const ws = new WebSocket(buildUrl());
    wsRef.current = ws;

    ws.onopen = () => {
      attemptRef.current = 0;
      setStatus("connected");
      replaySubscriptions(ws);
    };

    ws.onclose = () => {
      setStatus("disconnected");
      scheduleReconnect(connect);
    };

    ws.onerror = () => {
      // The browser fires onerror right before onclose, so we do not
      // change status here. onclose will handle reconnection.
    };

    ws.onmessage = (messageEvent: MessageEvent) => {
      let msg: WebSocketMessage;
      try {
        msg = JSON.parse(messageEvent.data as string) as WebSocketMessage;
      } catch {
        return; // silently ignore malformed messages
      }

      const callbacks = listenersRef.current.get(msg.event);
      if (callbacks) {
        callbacks.forEach((cb) => cb(msg.data));
      }
    };
  }, [buildUrl, clearReconnectTimer, replaySubscriptions, scheduleReconnect]);

  // ── Public API ──────────────────────────────────────────────────────

  const subscribe = useCallback(
    (event: string, filters?: Record<string, string>) => {
      const sub: Subscription = { event, filters };
      subscriptionsRef.current.set(event, sub);

      const ws = wsRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ action: "subscribe", event, filters }));
      }
    },
    [],
  );

  const unsubscribe = useCallback((event: string) => {
    subscriptionsRef.current.delete(event);

    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ action: "unsubscribe", event }));
    }
  }, []);

  const onEvent = useCallback(
    (event: string, callback: (data: unknown) => void): (() => void) => {
      if (!listenersRef.current.has(event)) {
        listenersRef.current.set(event, new Set());
      }
      listenersRef.current.get(event)!.add(callback);

      // Return a cleanup function that removes this specific listener.
      return () => {
        const set = listenersRef.current.get(event);
        if (set) {
          set.delete(callback);
          if (set.size === 0) {
            listenersRef.current.delete(event);
          }
        }
      };
    },
    [],
  );

  const disconnect = useCallback(() => {
    intentionalDisconnectRef.current = true;
    clearReconnectTimer();

    const ws = wsRef.current;
    if (ws) {
      ws.onopen = null;
      ws.onclose = null;
      ws.onerror = null;
      ws.onmessage = null;
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close();
      }
      wsRef.current = null;
    }
    setStatus("disconnected");
  }, [clearReconnectTimer]);

  const reconnectManual = useCallback(() => {
    attemptRef.current = 0;
    connect();
  }, [connect]);

  // ── Lifecycle ───────────────────────────────────────────────────────

  useEffect(() => {
    if (autoConnect) {
      connect();
    }

    return () => {
      // Clean up on unmount: prevent reconnects and close the socket.
      intentionalDisconnectRef.current = true;
      clearReconnectTimer();

      const ws = wsRef.current;
      if (ws) {
        ws.onopen = null;
        ws.onclose = null;
        ws.onerror = null;
        ws.onmessage = null;
        if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
          ws.close();
        }
        wsRef.current = null;
      }
    };
  }, []);

  return {
    status,
    subscribe,
    unsubscribe,
    onEvent,
    disconnect,
    reconnect: reconnectManual,
  };
}
