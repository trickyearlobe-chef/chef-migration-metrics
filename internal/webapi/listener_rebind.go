// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"sync"
)

// ErrNoListenerRebinder is returned by ListenerRebindHolder.Rebind when no
// in-place rebinder is wired in — the deployment cannot apply a listen
// address/port change live (running outside the server process, in tests, or in
// a listener topology that does not yet support rebind: active auto-443, ACME,
// or a degraded fallback). The caller treats this as "restart required": the new
// listen target is already persisted and applied on the next restart.
var ErrNoListenerRebinder = errors.New("webapi: no listener rebinder configured")

// ListenerRebindHolder is a concurrency-safe holder for the running server's
// in-place listener rebinder. It is wired into the Router up front (via
// WithListenerRebinder) so the server controller — built after the router during
// listener setup — can populate it later, mirroring the TLSReloadHolder pattern.
//
// The rebinder is a plain function rather than an interface so the controller
// (internal/serverctl) need not import webapi to satisfy it.
type ListenerRebindHolder struct {
	mu     sync.RWMutex
	rebind func(addr string, port int) (ReloadGranularity, error)
}

// NewListenerRebindHolder returns an empty holder. Rebind returns
// ErrNoListenerRebinder until a rebinder is set.
func NewListenerRebindHolder() *ListenerRebindHolder {
	return &ListenerRebindHolder{}
}

// Set records the running server's listener rebinder. A nil func clears it
// (Rebind then reports ErrNoListenerRebinder).
func (h *ListenerRebindHolder) Set(fn func(addr string, port int) (ReloadGranularity, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rebind = fn
}

// Rebind applies a listen address/port change to the running listener in place
// and reports the granularity that was needed. It returns ErrNoListenerRebinder
// (with the pessimistic ReloadProcess) when no rebinder is wired, or the
// rebinder's own error when the new bind failed (the previous listener keeps
// serving in that case).
func (h *ListenerRebindHolder) Rebind(addr string, port int) (ReloadGranularity, error) {
	h.mu.RLock()
	fn := h.rebind
	h.mu.RUnlock()
	if fn == nil {
		return ReloadProcess, ErrNoListenerRebinder
	}
	return fn(addr, port)
}

// WithListenerRebinder wires in the holder used to rebind the running HTTP/TLS
// listener in place when server.listen_address/port is saved through the admin
// API. When nil, saving a changed listen target persists it and relies on a
// restart to apply.
func WithListenerRebinder(h *ListenerRebindHolder) RouterOption {
	return func(r *Router) {
		r.listenerRebind = h
	}
}
