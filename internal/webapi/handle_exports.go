// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Export endpoints — stream the current filtered list of a list-view entity
// (nodes, cookbooks, roles, git_repos), track async jobs, and serve downloads.
// The export carries the SAME query params as the corresponding list endpoint
// and runs the SAME datastore query, so it reproduces the filtered list exactly.
// See specifications/web-api-exports.md.
// ---------------------------------------------------------------------------

// exportJobResponse is the JSON envelope returned for export job status.
type exportJobResponse struct {
	JobID         string `json:"job_id"`
	ExportType    string `json:"export_type"`
	Format        string `json:"format"`
	Status        string `json:"status"`
	RowCount      int    `json:"row_count,omitempty"`
	FileSizeBytes int64  `json:"file_size_bytes,omitempty"`
	DownloadURL   string `json:"download_url,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	RequestedAt   string `json:"requested_at"`
	CompletedAt   string `json:"completed_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	Message       string `json:"message,omitempty"`
}

// handleExports dispatches POST /api/v1/exports — create a new export. The
// request carries the list view's query params plus export_type and format.
func (r *Router) handleExports(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"Export creation requires POST.")
		return
	}

	exportType := req.URL.Query().Get("export_type")
	format := req.URL.Query().Get("format")

	spec, ok := r.exportRegistry()[exportType]
	if !ok {
		WriteBadRequest(w, fmt.Sprintf(
			"Invalid export_type %q. Must be one of: nodes, cookbooks, roles, git_repos.",
			exportType))
		return
	}
	if !datastore.ValidExportFormat(format) {
		WriteBadRequest(w, fmt.Sprintf(
			"Invalid format %q. Must be one of: csv, json, chef_search_query.", format))
		return
	}
	if format == datastore.ExportFormatChefSearchQuery && !spec.SupportsChefSearch {
		WriteBadRequest(w, "chef_search_query format is only supported for the nodes export.")
		return
	}

	estimatedRows, err := spec.Count(req.Context(), r, req)
	if err != nil {
		r.logf("ERROR", "estimating export rows for %s: %v", exportType, err)
		WriteInternalError(w, "Failed to prepare export.")
		return
	}

	asyncThreshold := r.liveConfig().Exports.AsyncThreshold
	if asyncThreshold <= 0 {
		asyncThreshold = 10000
	}

	if estimatedRows > asyncThreshold {
		r.handleAsyncExport(w, req, spec, exportType, format)
	} else {
		r.handleSyncExport(w, req, spec, exportType, format)
	}
}

// handleSyncExport streams a small export into a buffer and returns it inline.
// Sync exports are below the async threshold, so buffering is bounded and lets
// us report the exact row count in a header.
func (r *Router) handleSyncExport(w http.ResponseWriter, req *http.Request, spec exportSpec, exportType, format string) {
	var buf bytes.Buffer
	rowCount, err := r.streamExportTo(req.Context(), &buf, spec, req, format)
	if err != nil {
		r.logf("ERROR", "generating synchronous export: %v", err)
		WriteInternalError(w, "Failed to generate export.")
		return
	}

	w.Header().Set("Content-Type", contentTypeForFormat(format))
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, downloadFilename(exportType, format, time.Now().UTC())))
	w.Header().Set("X-Export-Row-Count", fmt.Sprintf("%d", rowCount))
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// handleAsyncExport creates an export job (persisting the raw query so the
// background goroutine reconstructs the same filter), launches generation, and
// returns 202.
func (r *Router) handleAsyncExport(w http.ResponseWriter, req *http.Request, spec exportSpec, exportType, format string) {
	ctx := req.Context()

	retentionHours := r.liveConfig().Exports.RetentionHours
	if retentionHours <= 0 {
		retentionHours = 24
	}

	filtersJSON, err := marshalExportQuery(req)
	if err != nil {
		r.logf("ERROR", "marshalling export query: %v", err)
		WriteInternalError(w, "Failed to create export job.")
		return
	}

	job, err := r.db.InsertExportJob(ctx, datastore.InsertExportJobParams{
		ExportType:  exportType,
		Format:      format,
		Filters:     filtersJSON,
		RequestedBy: "", // TODO: set from auth context when auth is implemented
		ExpiresAt:   time.Now().UTC().Add(time.Duration(retentionHours) * time.Hour),
	})
	if err != nil {
		r.logf("ERROR", "inserting export job: %v", err)
		WriteInternalError(w, "Failed to create export job.")
		return
	}

	go r.runAsyncExport(job.ID, spec, exportType, format, filtersJSON)

	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventExportStarted, map[string]any{
			"job_id":      job.ID,
			"export_type": exportType,
			"format":      format,
		}))
	}

	WriteJSON(w, http.StatusAccepted, exportJobResponse{
		JobID:       job.ID,
		ExportType:  job.ExportType,
		Format:      job.Format,
		Status:      job.Status,
		RequestedAt: job.RequestedAt.Format(time.RFC3339),
		Message:     "Export job created. Poll GET /api/v1/exports/" + job.ID + " for status.",
	})
}

// runAsyncExport streams an export file to disk in the background, updating the
// export_jobs row with progress and results. It reconstructs the original
// request from the persisted query so it reproduces the same filtered list.
func (r *Router) runAsyncExport(jobID string, spec exportSpec, exportType, format string, filtersJSON []byte) {
	ctx, cancel := context.WithTimeout(r.asyncContext(), 1*time.Hour)
	defer cancel()

	fail := func(err error) {
		r.logf("ERROR", "async export %s failed: %v", jobID, err)
		if uErr := r.db.UpdateExportJobStatus(ctx, jobID, datastore.ExportStatusFailed, 0, "", 0, err.Error()); uErr != nil {
			r.logf("ERROR", "updating export job %s to failed: %v", jobID, uErr)
		}
		if r.hub != nil {
			r.hub.Broadcast(NewEvent(EventExportFailed, map[string]any{"job_id": jobID, "error": err.Error()}))
		}
	}

	if err := r.db.UpdateExportJobStatus(ctx, jobID, datastore.ExportStatusProcessing, 0, "", 0, ""); err != nil {
		r.logf("ERROR", "updating export job %s to processing: %v", jobID, err)
		return
	}

	req, err := reconstructExportRequest(ctx, filtersJSON)
	if err != nil {
		fail(fmt.Errorf("reconstructing export request: %w", err))
		return
	}

	outputDir := r.liveConfig().Exports.OutputDirectory
	if outputDir == "" {
		outputDir = "/var/lib/chef-migration-metrics/exports"
	}
	ext := format
	if ext == datastore.ExportFormatChefSearchQuery {
		ext = "txt"
	}
	outputPath := filepath.Join(outputDir, fmt.Sprintf("%s.%s", jobID, ext))

	f, err := os.Create(outputPath) //nolint:gosec // path is server-controlled (jobID + configured dir)
	if err != nil {
		fail(fmt.Errorf("creating export file: %w", err))
		return
	}
	cw := &countingWriter{w: f}
	rowCount, streamErr := r.streamExportTo(ctx, cw, spec, req, format)
	closeErr := f.Close()
	if streamErr != nil {
		fail(streamErr)
		return
	}
	if closeErr != nil {
		fail(fmt.Errorf("closing export file: %w", closeErr))
		return
	}

	if err := r.db.UpdateExportJobStatus(ctx, jobID, datastore.ExportStatusCompleted,
		rowCount, outputPath, cw.n, ""); err != nil {
		r.logf("ERROR", "updating export job %s to completed: %v", jobID, err)
		return
	}

	r.logf("INFO", "export job %s completed: %d rows, %d bytes", jobID, rowCount, cw.n)

	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventExportComplete, map[string]any{
			"job_id":    jobID,
			"row_count": rowCount,
			"file_size": cw.n,
		}))
	}
}

// handleExportStatus dispatches requests under /api/v1/exports/:id.
// It handles:
//   - GET /api/v1/exports/:id — job status
//   - GET /api/v1/exports/:id/download — file download
func (r *Router) handleExportStatus(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	segs := pathSegments(req.URL.Path, "/api/v1/exports/")
	if len(segs) == 0 {
		WriteNotFound(w, "Export job ID is required.")
		return
	}

	jobID := segs[0]

	// Check for /download sub-path.
	if len(segs) >= 2 && segs[1] == "download" {
		r.handleExportDownload(w, req, jobID)
		return
	}

	// Return job status.
	ctx := req.Context()
	job, err := r.db.GetExportJob(ctx, jobID)
	if errors.Is(err, datastore.ErrNotFound) || job == nil {
		WriteNotFound(w, fmt.Sprintf("Export job %q not found.", jobID))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting export job %s: %v", jobID, err)
		WriteInternalError(w, "Failed to get export job.")
		return
	}

	resp := exportJobResponse{
		JobID:       job.ID,
		ExportType:  job.ExportType,
		Format:      job.Format,
		Status:      job.Status,
		RowCount:    job.RowCount,
		RequestedAt: job.RequestedAt.Format(time.RFC3339),
	}

	if job.FileSizeBytes > 0 {
		resp.FileSizeBytes = job.FileSizeBytes
	}
	if !job.CompletedAt.IsZero() {
		resp.CompletedAt = job.CompletedAt.Format(time.RFC3339)
	}
	if !job.ExpiresAt.IsZero() {
		resp.ExpiresAt = job.ExpiresAt.Format(time.RFC3339)
	}
	if job.ErrorMessage != "" {
		resp.ErrorMessage = job.ErrorMessage
	}
	if job.Status == datastore.ExportStatusCompleted {
		resp.DownloadURL = fmt.Sprintf("/api/v1/exports/%s/download", job.ID)
	}

	WriteJSON(w, http.StatusOK, resp)
}

// handleExportDownload serves the completed export file for download.
func (r *Router) handleExportDownload(w http.ResponseWriter, req *http.Request, jobID string) {
	ctx := req.Context()

	job, err := r.db.GetExportJob(ctx, jobID)
	if errors.Is(err, datastore.ErrNotFound) || job == nil {
		WriteNotFound(w, fmt.Sprintf("Export job %q not found.", jobID))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting export job %s for download: %v", jobID, err)
		WriteInternalError(w, "Failed to get export job.")
		return
	}

	if job.Status != datastore.ExportStatusCompleted {
		status := http.StatusConflict
		msg := fmt.Sprintf("Export job %q is not yet completed (status: %s).", jobID, job.Status)
		switch job.Status {
		case datastore.ExportStatusExpired:
			status = http.StatusGone
			msg = fmt.Sprintf("Export job %q has expired. Please create a new export.", jobID)
		case datastore.ExportStatusFailed:
			msg = fmt.Sprintf("Export job %q failed: %s", jobID, job.ErrorMessage)
		}
		WriteError(w, status, "export_not_ready", msg)
		return
	}

	// Check if expired by time (even if status hasn't been flipped yet).
	if !job.ExpiresAt.IsZero() && time.Now().UTC().After(job.ExpiresAt) {
		WriteError(w, http.StatusGone, "export_expired",
			fmt.Sprintf("Export job %q has expired. Please create a new export.", jobID))
		return
	}

	if job.FilePath == "" {
		WriteNotFound(w, "Export file path is not set.")
		return
	}

	// Verify the file exists on disk.
	info, statErr := os.Stat(job.FilePath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			WriteNotFound(w, "Export file has been removed from disk.")
			return
		}
		r.logf("ERROR", "stat export file %s: %v", job.FilePath, statErr)
		WriteInternalError(w, "Failed to access export file.")
		return
	}

	// Determine content type and filename from the job metadata.
	contentType := contentTypeForFormat(job.Format)
	filename := downloadFilename(job.ExportType, job.Format, job.RequestedAt)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	http.ServeFile(w, req, job.FilePath)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// asyncContext returns a background context for async export goroutines.
// The request context cannot be used because it is cancelled when the HTTP
// response is sent.
func (r *Router) asyncContext() context.Context {
	return context.Background()
}

// contentTypeForFormat returns the MIME type for the given export format.
func contentTypeForFormat(format string) string {
	switch format {
	case datastore.ExportFormatCSV:
		return "text/csv; charset=utf-8"
	case datastore.ExportFormatJSON:
		return "application/json; charset=utf-8"
	case datastore.ExportFormatChefSearchQuery:
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// downloadFilename generates a Content-Disposition filename for the export.
func downloadFilename(exportType, format string, requestedAt time.Time) string {
	datePart := requestedAt.Format("2006-01-02")
	ext := format
	if ext == "chef_search_query" {
		ext = "txt"
	}
	return fmt.Sprintf("%s_%s.%s", exportType, datePart, ext)
}
