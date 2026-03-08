// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// WebSocketConfig holds runtime configuration for WebSocket connections.
// These values are typically sourced from config.WebSocketConfig.
type WebSocketConfig struct {
	// WriteTimeout is the maximum time to write a single WebSocket frame.
	WriteTimeout time.Duration

	// PingInterval is how often the server sends ping frames.
	PingInterval time.Duration

	// PongTimeout is how long the server waits for a pong response.
	// Must be greater than PingInterval.
	PongTimeout time.Duration
}

// defaultWebSocketConfig returns sensible defaults matching the spec.
func defaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{
		WriteTimeout: 10 * time.Second,
		PingInterval: 30 * time.Second,
		PongTimeout:  60 * time.Second,
	}
}

// WebSocketHandler handles the /api/v1/ws endpoint. It upgrades the HTTP
// connection to a WebSocket, registers with the EventHub, and streams events
// to the client until the connection closes or the context is cancelled.
type WebSocketHandler struct {
	hub *EventHub
	cfg WebSocketConfig

	// logger is an optional callback for logging WebSocket lifecycle events.
	// If nil, events are silently discarded. The webapi package does not
	// import the logging package to avoid circular dependencies — the
	// caller provides a logging function at construction time.
	logger func(level, msg string)
}

// WebSocketHandlerOption is a functional option for NewWebSocketHandler.
type WebSocketHandlerOption func(*WebSocketHandler)

// WithWebSocketConfig sets the WebSocket connection configuration.
func WithWebSocketConfig(cfg WebSocketConfig) WebSocketHandlerOption {
	return func(h *WebSocketHandler) {
		h.cfg = cfg
	}
}

// WithWebSocketLogger sets a logging callback for WebSocket lifecycle events.
// The level parameter is one of "DEBUG", "INFO", "WARN", "ERROR".
func WithWebSocketLogger(fn func(level, msg string)) WebSocketHandlerOption {
	return func(h *WebSocketHandler) {
		h.logger = fn
	}
}

// NewWebSocketHandler creates a new WebSocketHandler that streams events from
// the given EventHub to connected clients.
func NewWebSocketHandler(hub *EventHub, opts ...WebSocketHandlerOption) *WebSocketHandler {
	h := &WebSocketHandler{
		hub: hub,
		cfg: defaultWebSocketConfig(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP implements http.Handler. It upgrades the request to a WebSocket
// connection and streams events until the connection closes.
//
// Authentication should be enforced by middleware before this handler is
// reached. The handler itself does not validate session tokens — it trusts
// that the middleware has already done so.
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests for the WebSocket upgrade.
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method_not_allowed","message":"WebSocket endpoint requires GET"}`, http.StatusMethodNotAllowed)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// We serve the frontend from the same origin, so no need for
		// special CORS handling. If cross-origin is needed, the caller
		// can configure InsecureSkipVerify or set OriginPatterns.
	})
	if err != nil {
		h.logf("WARN", "websocket upgrade failed: %v", err)
		// websocket.Accept already wrote an HTTP error response.
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "server closing connection")

	h.logf("DEBUG", "websocket client connected from %s", r.RemoteAddr)

	// Register with the event hub.
	c := h.hub.Register()
	defer h.hub.Unregister(c)

	// Check if the client was immediately rejected (hub at capacity or stopped).
	select {
	case _, ok := <-c.send:
		if !ok {
			h.logf("WARN", "websocket client rejected (hub at capacity or stopped) from %s", r.RemoteAddr)
			conn.Close(websocket.StatusTryAgainLater, "server at capacity")
			return
		}
		// We accidentally consumed an event — this shouldn't happen since
		// we just registered. Put it back by sending a connected event below.
	default:
		// Channel is open and empty — expected path.
	}

	// Send the initial "connected" event.
	connectedEvt := NewEvent(EventConnected, map[string]any{
		// session_expires_at would be populated by the auth middleware
		// if we had access to session data here. For now, omit it.
	})
	if err := h.writeEvent(r.Context(), conn, connectedEvt); err != nil {
		h.logf("DEBUG", "websocket client disconnected during handshake from %s: %v", r.RemoteAddr, err)
		return
	}

	// Start the read pump in a goroutine. The read pump discards all
	// incoming messages (the channel is server-to-client only) but must
	// run to process WebSocket control frames (close, pong). When the
	// client disconnects, the read pump returns and we cancel the context.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go h.readPump(ctx, cancel, conn, r.RemoteAddr)

	// Start the ping ticker.
	pingTicker := time.NewTicker(h.cfg.PingInterval)
	defer pingTicker.Stop()

	// Write pump: drain the client's send channel and write events.
	for {
		select {
		case <-ctx.Done():
			h.logf("DEBUG", "websocket context cancelled for %s", r.RemoteAddr)
			return

		case evt, ok := <-c.send:
			if !ok {
				// Hub closed our send channel (slow consumer or shutdown).
				h.logf("DEBUG", "websocket send channel closed for %s (slow consumer or hub shutdown)", r.RemoteAddr)
				conn.Close(websocket.StatusGoingAway, "server shutting down")
				return
			}
			if err := h.writeEvent(ctx, conn, evt); err != nil {
				h.logf("DEBUG", "websocket write failed for %s: %v", r.RemoteAddr, err)
				return
			}

		case <-pingTicker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, h.cfg.WriteTimeout)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				h.logf("DEBUG", "websocket ping failed for %s: %v", r.RemoteAddr, err)
				return
			}
		}
	}
}

// writeEvent serialises an Event to JSON and writes it as a text frame.
func (h *WebSocketHandler) writeEvent(ctx context.Context, conn *websocket.Conn, evt Event) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshalling event: %w", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, h.cfg.WriteTimeout)
	defer cancel()

	return conn.Write(writeCtx, websocket.MessageText, data)
}

// readPump reads and discards all messages from the client. Its primary
// purpose is to process WebSocket control frames (close, pong). When the
// read fails (client disconnect, protocol error), it cancels the context
// to signal the write pump to stop.
func (h *WebSocketHandler) readPump(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, remoteAddr string) {
	defer cancel()

	// Set the read limit to a small value — we don't expect meaningful
	// messages from the client.
	conn.SetReadLimit(4096)

	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			// Normal closure or context cancellation — not worth logging at WARN.
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				h.logf("DEBUG", "websocket client %s closed normally", remoteAddr)
			} else if ctx.Err() != nil {
				// Context was cancelled (server shutdown or write pump exited).
				h.logf("DEBUG", "websocket read pump context cancelled for %s", remoteAddr)
			} else {
				h.logf("DEBUG", "websocket read error from %s: %v", remoteAddr, err)
			}
			return
		}
		// Discard the message — the channel is server-to-client only.
	}
}

// logf logs a formatted message if a logger is configured.
func (h *WebSocketHandler) logf(level, format string, args ...any) {
	if h.logger != nil {
		h.logger(level, fmt.Sprintf(format, args...))
	}
}
