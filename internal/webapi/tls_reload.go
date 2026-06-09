// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"sync"
)

// ErrNoTLSReloader is returned by TLSReloadHolder.Reload when no in-place
// reloader is wired in — either the deployment is not running static TLS or
// the running listener is file-backed (cert_source: file). The caller treats
// this as "restart required", not a failure: the new certificate is already
// persisted and applied on the next restart.
var ErrNoTLSReloader = errors.New("webapi: no TLS certificate reloader configured")

// CertReloader swaps the live TLS certificate for new PEM material without a
// restart. It is implemented by the static-mode CertManager's in-memory PEM
// source (cert_source: db). main.go wires the running listener's CertManager
// in after startup.
type CertReloader interface {
	ReloadFromPEM(certPEM, keyPEM []byte) error
}

// TLSReloadHolder is a concurrency-safe holder for the running listener's
// in-place certificate reloader. It is wired into the Router up front (via
// WithTLSReload) so a later listener construction can populate it, mirroring
// the TLSStatusHolder pattern.
type TLSReloadHolder struct {
	mu       sync.RWMutex
	reloader CertReloader
}

// NewTLSReloadHolder returns an empty holder. Reload returns ErrNoTLSReloader
// until a reloader is set.
func NewTLSReloadHolder() *TLSReloadHolder {
	return &TLSReloadHolder{}
}

// Set records the running listener's certificate reloader. A nil reloader
// clears it (Reload then reports ErrNoTLSReloader).
func (h *TLSReloadHolder) Set(r CertReloader) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reloader = r
}

// Reload swaps the live certificate for the given PEM material. It returns
// ErrNoTLSReloader when no reloader is wired in, or the reloader's own error
// when the swap fails (the previous certificate keeps serving in that case).
func (h *TLSReloadHolder) Reload(certPEM, keyPEM []byte) error {
	h.mu.RLock()
	r := h.reloader
	h.mu.RUnlock()
	if r == nil {
		return ErrNoTLSReloader
	}
	return r.ReloadFromPEM(certPEM, keyPEM)
}

// WithTLSReload wires in the holder used to swap the running static-TLS
// certificate in place when a cert_source: db pair is saved through the admin
// API. When nil, saving a DB pair persists it and relies on a restart to apply.
func WithTLSReload(h *TLSReloadHolder) RouterOption {
	return func(r *Router) {
		r.tlsReload = h
	}
}
