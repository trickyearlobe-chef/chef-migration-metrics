// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
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

	// Always stream the export directly to the response as a download. The
	// encoder holds only one page in memory, so this works at any size. There is
	// no async job path — that remains future scaffolding (the export_jobs table
	// and status/download endpoints are dormant, reserved for pipeline export to
	// logstash/elasticsearch/observe).
	r.streamExportDownload(w, req, spec, exportType, format)
}

// streamExportDownload streams the export directly to the response as a file
// download, holding only one page in memory. Query/source errors surface as a
// clean 500 before any bytes are written; an error once streaming has begun can
// only be logged, since the response status is already committed.
func (r *Router) streamExportDownload(w http.ResponseWriter, req *http.Request, spec exportSpec, exportType, format string) {
	ctx := req.Context()

	src, err := spec.NewSource(ctx, r, req)
	if err != nil {
		r.logf("ERROR", "preparing export %s: %v", exportType, err)
		WriteInternalError(w, "Failed to generate export.")
		return
	}

	w.Header().Set("Content-Type", contentTypeForFormat(format))
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, downloadFilename(exportType, format, time.Now().UTC())))

	var streamErr error
	switch format {
	case datastore.ExportFormatJSON:
		_, streamErr = export.StreamJSON(ctx, w, spec.Columns, src)
	case datastore.ExportFormatChefSearchQuery:
		_, streamErr = export.StreamChefSearchQuery(ctx, w, spec.ChefSearchName, src)
	default:
		_, streamErr = export.StreamCSV(ctx, w, spec.Columns, src)
	}
	if streamErr != nil {
		r.logf("ERROR", "streaming export %s: %v", exportType, streamErr)
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
