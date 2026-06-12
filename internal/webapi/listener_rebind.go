// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"sync"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// ErrNoListenerRebinder is returned by ListenerRebindHolder.Apply when no
// in-place rebinder is wired in, or the wired rebinder cannot apply the desired
// config live — the deployment cannot rebind the listener for this change
// (running outside the server process, in tests, or in a listener topology that
// does not yet support rebind: active auto-443, an http_redirect_port, ACME, or a
// degraded fallback). The caller treats this as "restart required": the new
// config is already persisted and applied on the next restart.
var ErrNoListenerRebinder = errors.New("webapi: no listener rebinder configured")

// ListenerRebindHolder is a concurrency-safe holder for the running server's
// in-place listener rebinder. It is wired into the Router up front (via
// WithListenerRebinder) so the server controller — built after the router during
// listener setup — can populate it later, mirroring the TLSReloadHolder pattern.
//
// The rebinder is a plain function rather than an interface so the controller
// (internal/serverctl) need not import webapi to satisfy it. It receives the full
// desired server config so it can rebuild either listener topology — a plain or a
// static-TLS listener — for an off↔static mode transition as well as a plain
// listen address/port change.
type ListenerRebindHolder struct {
	mu     sync.RWMutex
	rebind func(cfg config.ServerConfig) (ReloadGranularity, error)
}

// NewListenerRebindHolder returns an empty holder. Apply returns
// ErrNoListenerRebinder until a rebinder is set.
func NewListenerRebindHolder() *ListenerRebindHolder {
	return &ListenerRebindHolder{}
}

// Set records the running server's listener rebinder. A nil func clears it
// (Apply then reports ErrNoListenerRebinder).
func (h *ListenerRebindHolder) Set(fn func(cfg config.ServerConfig) (ReloadGranularity, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rebind = fn
}

// Apply rebinds the running listener in place to serve the desired server config
// and reports the granularity that was needed. It returns ErrNoListenerRebinder
// (with the pessimistic ReloadProcess) when no rebinder is wired or the wired
// rebinder refuses the target topology, or the rebinder's own error when the new
// bind failed (the previous listener keeps serving in that case).
func (h *ListenerRebindHolder) Apply(cfg config.ServerConfig) (ReloadGranularity, error) {
	h.mu.RLock()
	fn := h.rebind
	h.mu.RUnlock()
	if fn == nil {
		return ReloadProcess, ErrNoListenerRebinder
	}
	return fn(cfg)
}

// WithListenerRebinder wires in the holder used to rebind the running HTTP/TLS
// listener in place when server.listen_address/port or the TLS mode is saved
// through the admin API. When nil, saving such a change persists it and relies on
// a restart to apply.
func WithListenerRebinder(h *ListenerRebindHolder) RouterOption {
	return func(r *Router) {
		r.listenerRebind = h
	}
}
