// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"testing"
	"time"
)

// resolveConfig prefers the live accessor when wired, so a server.websocket.*
// timeout change is reflected for connections opened after the change.
func TestWebSocketHandler_ResolveConfig_PrefersLiveFunc(t *testing.T) {
	hub := NewEventHub()

	current := WebSocketConfig{
		WriteTimeout: 5 * time.Second,
		PingInterval: 15 * time.Second,
		PongTimeout:  30 * time.Second,
	}
	h := NewWebSocketHandler(hub, WithWebSocketConfigFunc(func() WebSocketConfig {
		return current
	}))

	if got := h.resolveConfig(); got != current {
		t.Fatalf("resolveConfig = %+v, want %+v", got, current)
	}

	// A subsequent save changes the live values; the next connection picks them up.
	current = WebSocketConfig{
		WriteTimeout: 8 * time.Second,
		PingInterval: 20 * time.Second,
		PongTimeout:  45 * time.Second,
	}
	if got := h.resolveConfig(); got != current {
		t.Errorf("resolveConfig after live change = %+v, want %+v", got, current)
	}
}

// Without a live accessor, resolveConfig falls back to the static config (the
// path tests and non-router callers rely on).
func TestWebSocketHandler_ResolveConfig_FallsBackToStatic(t *testing.T) {
	hub := NewEventHub()

	static := WebSocketConfig{
		WriteTimeout: 7 * time.Second,
		PingInterval: 25 * time.Second,
		PongTimeout:  50 * time.Second,
	}
	h := NewWebSocketHandler(hub, WithWebSocketConfig(static))

	if got := h.resolveConfig(); got != static {
		t.Errorf("resolveConfig = %+v, want %+v", got, static)
	}
}

// With neither option, resolveConfig returns the package defaults.
func TestWebSocketHandler_ResolveConfig_Defaults(t *testing.T) {
	hub := NewEventHub()
	h := NewWebSocketHandler(hub)

	if got := h.resolveConfig(); got != defaultWebSocketConfig() {
		t.Errorf("resolveConfig = %+v, want defaults %+v", got, defaultWebSocketConfig())
	}
}
