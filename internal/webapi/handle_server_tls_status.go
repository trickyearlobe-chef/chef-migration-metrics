// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sync"
)

// TLSStatus is the public, DB-independent view of the TLS listener health,
// served by GET /api/v1/server/tls-status. When the static TLS listener fails
// at startup the server falls open to plain HTTP (see tls.md § 2.4) and reports
// Degraded=true here so the UI can warn on every page, including before login.
type TLSStatus struct {
	Degraded bool   `json:"degraded"`
	Reason   string `json:"reason,omitempty"`
}

// TLSStatusHolder is a concurrency-safe holder for the degraded TLS state. The
// startup goroutine writes it once on a fallback; the banner poll reads it.
type TLSStatusHolder struct {
	mu     sync.RWMutex
	status TLSStatus
}

// NewTLSStatusHolder returns a holder in the healthy (not degraded) state.
func NewTLSStatusHolder() *TLSStatusHolder {
	return &TLSStatusHolder{}
}

// SetDegraded records that the server fell back to plain HTTP, with an
// operator-facing reason. The reason must never contain private key material.
func (h *TLSStatusHolder) SetDegraded(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = TLSStatus{Degraded: true, Reason: reason}
}

// Status returns a copy of the current TLS status.
func (h *TLSStatusHolder) Status() TLSStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

// WithTLSStatus wires in the TLS degraded-state holder reported by the
// /api/v1/server/tls-status endpoint. When nil, the endpoint reports healthy.
func WithTLSStatus(h *TLSStatusHolder) RouterOption {
	return func(r *Router) {
		r.tlsStatus = h
	}
}

// handleServerTLSStatus reports whether the server is running in the degraded
// plain-HTTP fallback. Public and DB-free by design so the banner renders
// pre-login and even when other subsystems are unavailable.
func (r *Router) handleServerTLSStatus(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"TLS status endpoint requires GET.")
		return
	}
	var status TLSStatus
	if r.tlsStatus != nil {
		status = r.tlsStatus.Status()
	}
	WriteJSON(w, http.StatusOK, status)
}
