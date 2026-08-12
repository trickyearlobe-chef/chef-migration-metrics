// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/backup"
)

// handleAdminBackups dispatches to the appropriate backup handler based on
// method and path suffix.
func (r *Router) handleAdminBackups(w http.ResponseWriter, req *http.Request) {
	if r.backupService == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Backup tools (pg_dump/pg_restore) are not available. Ensure PostgreSQL client tools are installed.")
		return
	}

	// Extract sub-path: /api/v1/admin/backups/{id}/restore or /api/v1/admin/backups/{id}
	subPath := strings.TrimPrefix(req.URL.Path, "/api/v1/admin/backups")
	subPath = strings.TrimPrefix(subPath, "/")

	switch {
	case subPath == "" || subPath == "/":
		// /api/v1/admin/backups
		switch req.Method {
		case http.MethodGet:
			r.handleBackupList(w, req)
		case http.MethodPost:
			r.handleBackupCreate(w, req)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Use GET or POST")
		}

	case strings.HasSuffix(subPath, "/restore"):
		// /api/v1/admin/backups/{id}/restore
		if !requirePOST(w, req) {
			return
		}
		id := strings.TrimSuffix(subPath, "/restore")
		r.handleBackupRestore(w, req, id)

	default:
		// /api/v1/admin/backups/{id}
		id := subPath
		switch req.Method {
		case http.MethodGet:
			r.handleBackupGet(w, req, id)
		case http.MethodDelete:
			r.handleBackupDelete(w, req, id)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Use GET or DELETE")
		}
	}
}

// handleAdminBackupStatus returns the current backup/restore job status.
func (r *Router) handleAdminBackupStatus(w http.ResponseWriter, req *http.Request) {
	if r.backupService == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Backup tools (pg_dump/pg_restore) are not available.")
		return
	}
	if !requireGET(w, req) {
		return
	}

	job := r.backupService.ActiveJob()
	if job == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"active": true,
		"id":     job.ID,
		"status": job.Status,
	})
}

func (r *Router) handleBackupList(w http.ResponseWriter, _ *http.Request) {
	manifests, err := r.backupService.List()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	type backupItem struct {
		ID            string        `json:"id"`
		Filename      string        `json:"filename"`
		SizeBytes     int64         `json:"size_bytes"`
		SHA256        string        `json:"sha256,omitempty"`
		Status        backup.Status `json:"status"`
		Error         string        `json:"error,omitempty"`
		CreatedAt     time.Time     `json:"created_at"`
		CompletedAt   *time.Time    `json:"completed_at,omitempty"`
		InitiatedBy   string        `json:"initiated_by,omitempty"`
		AppVersion    string        `json:"app_version,omitempty"`
		SchemaVersion int           `json:"schema_version"`
	}

	items := make([]backupItem, 0, len(manifests))
	for _, m := range manifests {
		item := backupItem{
			ID:            m.ID,
			Filename:      m.Filename,
			SizeBytes:     m.SizeBytes,
			SHA256:        m.SHA256,
			Status:        m.Status,
			Error:         m.Error,
			CreatedAt:     m.CreatedAt,
			InitiatedBy:   m.InitiatedBy,
			AppVersion:    m.AppVersion,
			SchemaVersion: m.SchemaVersion,
		}
		if !m.CompletedAt.IsZero() {
			t := m.CompletedAt
			item.CompletedAt = &t
		}
		items = append(items, item)
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"backups":    items,
		"backup_dir": r.backupService.Dir(),
	})
}

func (r *Router) handleBackupCreate(w http.ResponseWriter, req *http.Request) {
	if r.backupService.IsActive() {
		WriteError(w, http.StatusConflict, "conflict",
			"Another backup or restore operation is already in progress.")
		return
	}

	initiatedBy := adminUsername(req)

	m, err := r.backupService.Create(req.Context(), initiatedBy)
	if err != nil {
		WriteError(w, http.StatusConflict, "conflict", err.Error())
		return
	}

	// Run the backup in the background with a detached context (not tied to the
	// request lifecycle). Hand the goroutine its own copy of the manifest so its
	// status mutations don't race the response read below.
	bm := m
	go r.backupService.RunCreate(context.Background(), &bm)

	WriteJSON(w, http.StatusAccepted, map[string]any{
		"id":     m.ID,
		"status": m.Status,
	})
}

func (r *Router) handleBackupGet(w http.ResponseWriter, _ *http.Request, id string) {
	m, err := r.backupService.Get(id)
	if err != nil {
		WriteError(w, http.StatusNotFound, ErrCodeNotFound, "Backup not found.")
		return
	}
	WriteJSON(w, http.StatusOK, m)
}

func (r *Router) handleBackupDelete(w http.ResponseWriter, _ *http.Request, id string) {
	if err := r.backupService.Delete(id); err != nil {
		if strings.Contains(err.Error(), "active") {
			WriteError(w, http.StatusConflict, "conflict", err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type restoreRequest struct {
	Confirm string `json:"confirm"`
}

func (r *Router) handleBackupRestore(w http.ResponseWriter, req *http.Request, id string) {
	// Two different failures, kept apart: a body this call cannot read at all,
	// and a body it read that did not confirm. Folded together they were
	// indistinguishable, so somebody who misspelt the field was told to confirm
	// — which they had.
	var body restoreRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}
	if body.Confirm != "RESTORE" {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			`Restore requires confirmation. Send {"confirm": "RESTORE"} in the request body.`)
		return
	}

	if r.backupService.IsActive() {
		WriteError(w, http.StatusConflict, "conflict",
			"Another backup or restore operation is already in progress.")
		return
	}

	// Verify the backup exists and checksum is valid
	if err := r.backupService.VerifyChecksum(id); err != nil {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError, err.Error())
		return
	}

	WriteJSON(w, http.StatusAccepted, map[string]any{
		"id":      id,
		"status":  "restoring",
		"message": "Restore initiated. The application will restart after completion.",
	})

	go r.executeRestore(id)
}

// executeRestore runs the full restore lifecycle: maintenance mode → stop
// workers → pg_restore → exit (or resume on failure).
func (r *Router) executeRestore(id string) {
	r.logf("INFO", "restore: entering maintenance mode for backup %s", id)

	// Enter maintenance mode — all API routes return 503
	r.maintenanceMessage.Store("Database restore in progress. Please wait.")
	r.maintenanceMode.Store(true)

	// Broadcast maintenance event to connected WebSocket clients
	if r.hub != nil {
		r.hub.Broadcast(Event{
			Type:      "maintenance",
			Timestamp: time.Now().UTC(),
			Data: map[string]any{
				"active":  true,
				"message": "Database restore in progress",
			},
		})
	}

	// Stop background workers via the restore hook
	if r.restoreHook != nil {
		r.logf("INFO", "restore: stopping background workers")
		r.restoreHook()
		r.logf("INFO", "restore: background workers stopped")
	}

	// Brief pause for any in-flight DB operations to complete
	time.Sleep(2 * time.Second)

	// Run the actual restore
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	err := r.backupService.RunRestore(ctx, id)
	if err != nil {
		r.logf("ERROR", "restore: failed: %v", err)

		// Resume normal operation on failure
		r.maintenanceMode.Store(false)
		r.maintenanceMessage.Store("")

		if r.hub != nil {
			r.hub.Broadcast(Event{
				Type:      "maintenance",
				Timestamp: time.Now().UTC(),
				Data: map[string]any{
					"active":  false,
					"message": fmt.Sprintf("Restore failed: %v", err),
				},
			})
		}
		return
	}

	r.logf("INFO", "restore: completed successfully, terminating process for restart")

	// Broadcast success before exit
	if r.hub != nil {
		r.hub.Broadcast(Event{
			Type:      "maintenance",
			Timestamp: time.Now().UTC(),
			Data: map[string]any{
				"active":  false,
				"message": "Restore completed. Application is restarting.",
			},
		})
	}

	// Brief pause so the WS message is delivered
	time.Sleep(500 * time.Millisecond)

	// Terminate the process — systemd/supervisor will restart it
	exitFn := r.exitFunc
	if exitFn == nil {
		exitFn = os.Exit
	}
	exitFn(0)
}
