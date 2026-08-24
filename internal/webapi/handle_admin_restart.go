// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import "net/http"

// ---------------------------------------------------------------------------
// POST /api/v1/admin/restart
// ---------------------------------------------------------------------------

// handleAdminRestart triggers a graceful restart of the running process so that
// restart-required configuration changes (listen address/port, TLS, WebSocket,
// auth) take effect without shell access.
//
// The handler returns 202 immediately, then signals the restart trigger. The
// trigger (wired by main via WithRestartTrigger) initiates a graceful shutdown
// of the running process and exits with a non-zero restart code, which the
// service supervisor (systemd Restart=on-failure) turns into a fresh start.
// Because the trigger only signals the main goroutine — it does not block — this
// response flushes and any in-flight requests drain before the process exits.
func (r *Router) handleAdminRestart(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports POST.")
		return
	}

	if r.restartFunc == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Restart is not available in this deployment — no service supervisor is wired to restart the process.")
		return
	}

	r.logf("INFO", "admin/restart: graceful restart requested via admin API")

	WriteJSON(w, http.StatusAccepted, map[string]any{
		"status":  "restarting",
		"message": "Restart initiated. The application will shut down gracefully and restart.",
	})

	// Signal the main goroutine. This is non-blocking — graceful shutdown
	// (and the eventual process exit) proceeds only after this handler returns
	// and the HTTP server drains in-flight requests.
	r.restartFunc()
}
