// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sync"
)

// Degraded-listener kinds reported by TLSStatus.Kind (tls.md § 6.3).
const (
	// DegradedKindSelfSigned is the primary fail-open: HTTPS with an ephemeral
	// untrusted self-signed certificate, so the recovery UI stays encrypted.
	DegradedKindSelfSigned = "self-signed"
	// DegradedKindPlain is the last-resort fail-open: cleartext HTTP, used only
	// when even the self-signed listener cannot be brought up.
	DegradedKindPlain = "plain"
)

// TLSStatus is the public, DB-independent view of the TLS listener health,
// served by GET /api/v1/server/tls-status. When the TLS listener cannot be built
// at startup the server falls open to a degraded listener (a self-signed HTTPS
// cert, or plain HTTP as a last resort — see tls.md § 6.3) and reports
// Degraded=true here so the UI can warn on every page, including before login.
type TLSStatus struct {
	Degraded bool `json:"degraded"`
	// Kind is the degraded-listener kind ("self-signed" | "plain"), empty when
	// healthy.
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason,omitempty"`
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

// SetDegraded records that the server fell back to the last-resort plain HTTP
// listener, with an operator-facing reason. The reason must never contain private
// key material.
func (h *TLSStatusHolder) SetDegraded(reason string) {
	h.SetDegradedKind(DegradedKindPlain, reason)
}

// SetDegradedKind records the degraded state with an explicit kind
// (DegradedKindSelfSigned or DegradedKindPlain). The reason must never contain
// private key material.
func (h *TLSStatusHolder) SetDegradedKind(kind, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = TLSStatus{Degraded: true, Kind: kind, Reason: reason}
}

// SetHealthy clears the degraded state. It is used when a real certificate is
// promoted in place over a degraded self-signed listener (e.g. ACME issuance
// succeeds), so the banner and the HSTS gate recover without a restart.
func (h *TLSStatusHolder) SetHealthy() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = TLSStatus{}
}

// IsDegraded reports whether the listener is currently in a degraded fallback.
// The HSTS gate consults it live so HSTS is suppressed while degraded.
func (h *TLSStatusHolder) IsDegraded() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status.Degraded
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
