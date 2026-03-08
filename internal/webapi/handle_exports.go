// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
)

// ---------------------------------------------------------------------------
// Export endpoints — data export generation, job tracking, and file download.
// ---------------------------------------------------------------------------

// exportRequest is the JSON body for POST /api/v1/exports.
type exportRequest struct {
	ExportType        string         `json:"export_type"`
	Format            string         `json:"format"`
	TargetChefVersion string         `json:"target_chef_version,omitempty"`
	Filters           export.Filters `json:"filters"`
}

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

// handleExports dispatches POST /api/v1/exports — create a new export.
func (r *Router) handleExports(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"Export creation requires POST.")
		return
	}

	var body exportRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, fmt.Sprintf("Invalid JSON request body: %v", err))
		return
	}

	// Validate export_type.
	if !datastore.ValidExportType(body.ExportType) {
		WriteBadRequest(w, fmt.Sprintf(
			"Invalid export_type %q. Must be one of: ready_nodes, blocked_nodes, cookbook_remediation.",
			body.ExportType))
		return
	}

	// Validate format.
	if !datastore.ValidExportFormat(body.Format) {
		WriteBadRequest(w, fmt.Sprintf(
			"Invalid format %q. Must be one of: csv, json, chef_search_query.",
			body.Format))
		return
	}

	// chef_search_query is only valid for ready_nodes.
	if body.Format == datastore.ExportFormatChefSearchQuery && body.ExportType != datastore.ExportTypeReadyNodes {
		WriteBadRequest(w, "chef_search_query format is only supported for ready_nodes export type.")
		return
	}

	// Node exports require a target_chef_version.
	if body.TargetChefVersion == "" && (body.ExportType == datastore.ExportTypeReadyNodes || body.ExportType == datastore.ExportTypeBlockedNodes) {
		// Default to the first configured target version.
		if len(r.cfg.TargetChefVersions) > 0 {
			body.TargetChefVersion = r.cfg.TargetChefVersions[0]
		} else {
			WriteBadRequest(w, "target_chef_version is required for node exports and none is configured.")
			return
		}
	}

	// Propagate the target version into the filters for consistency.
	if body.TargetChefVersion != "" {
		body.Filters.TargetChefVersion = body.TargetChefVersion
	}

	// Estimate row count using CountNodeReadiness or a simple heuristic.
	estimatedRows := r.estimateExportRows(req, body)

	asyncThreshold := r.cfg.Exports.AsyncThreshold
	if asyncThreshold <= 0 {
		asyncThreshold = 10000
	}

	maxRows := r.cfg.Exports.MaxRows
	if maxRows <= 0 {
		maxRows = 100000
	}

	if estimatedRows > asyncThreshold {
		// Asynchronous export — create a job, return 202.
		r.handleAsyncExport(w, req, body, maxRows)
	} else {
		// Synchronous export — generate inline, stream response.
		r.handleSyncExport(w, req, body, maxRows)
	}
}

// handleSyncExport generates a small export inline and streams the response.
func (r *Router) handleSyncExport(w http.ResponseWriter, req *http.Request, body exportRequest, maxRows int) {
	ctx := req.Context()

	result, err := r.generateExport(ctx, body, maxRows, "")
	if err != nil {
		r.logf("ERROR", "generating synchronous export: %v", err)
		WriteInternalError(w, "Failed to generate export.")
		return
	}

	// Set response headers for file download.
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	w.Header().Set("X-Export-Row-Count", fmt.Sprintf("%d", result.RowCount))
	w.WriteHeader(http.StatusOK)
	w.Write(result.Data)
}

// handleAsyncExport creates an export job, launches background generation,
// and returns 202 with the job ID.
func (r *Router) handleAsyncExport(w http.ResponseWriter, req *http.Request, body exportRequest, maxRows int) {
	ctx := req.Context()

	retentionHours := r.cfg.Exports.RetentionHours
	if retentionHours <= 0 {
		retentionHours = 24
	}

	filtersJSON, err := body.Filters.MarshalToJSON()
	if err != nil {
		r.logf("ERROR", "marshalling export filters: %v", err)
		WriteInternalError(w, "Failed to create export job.")
		return
	}

	job, err := r.db.InsertExportJob(ctx, datastore.InsertExportJobParams{
		ExportType:  body.ExportType,
		Format:      body.Format,
		Filters:     filtersJSON,
		RequestedBy: "", // TODO: set from auth context when auth is implemented
		ExpiresAt:   time.Now().UTC().Add(time.Duration(retentionHours) * time.Hour),
	})
	if err != nil {
		r.logf("ERROR", "inserting export job: %v", err)
		WriteInternalError(w, "Failed to create export job.")
		return
	}

	// Launch background generation goroutine.
	go r.runAsyncExport(job.ID, body, maxRows)

	// Broadcast event via WebSocket.
	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventExportStarted, map[string]any{
			"job_id":      job.ID,
			"export_type": body.ExportType,
			"format":      body.Format,
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

// runAsyncExport is launched as a goroutine to generate an export file in
// the background. It updates the export_jobs table with progress and results.
func (r *Router) runAsyncExport(jobID string, body exportRequest, maxRows int) {
	ctx := r.asyncContext()

	// Update status to processing.
	if err := r.db.UpdateExportJobStatus(ctx, jobID, datastore.ExportStatusProcessing, 0, "", 0, ""); err != nil {
		r.logf("ERROR", "updating export job %s to processing: %v", jobID, err)
		return
	}

	// Determine output path.
	outputDir := r.cfg.Exports.OutputDirectory
	if outputDir == "" {
		outputDir = "/var/lib/chef-migration-metrics/exports"
	}

	ext := body.Format
	if ext == "chef_search_query" {
		ext = "txt"
	}
	outputPath := filepath.Join(outputDir, fmt.Sprintf("%s.%s", jobID, ext))

	// Generate the export.
	result, err := r.generateExport(ctx, body, maxRows, outputPath)
	if err != nil {
		r.logf("ERROR", "async export generation failed for job %s: %v", jobID, err)
		if updateErr := r.db.UpdateExportJobStatus(ctx, jobID, datastore.ExportStatusFailed, 0, "", 0, err.Error()); updateErr != nil {
			r.logf("ERROR", "updating export job %s to failed: %v", jobID, updateErr)
		}
		if r.hub != nil {
			r.hub.Broadcast(NewEvent(EventExportFailed, map[string]any{
				"job_id": jobID,
				"error":  err.Error(),
			}))
		}
		return
	}

	// Update status to completed.
	if err := r.db.UpdateExportJobStatus(ctx, jobID, datastore.ExportStatusCompleted,
		result.RowCount, result.FilePath, result.FileSizeBytes, ""); err != nil {
		r.logf("ERROR", "updating export job %s to completed: %v", jobID, err)
		return
	}

	r.logf("INFO", "export job %s completed: %d rows, %d bytes", jobID, result.RowCount, result.FileSizeBytes)

	if r.hub != nil {
		r.hub.Broadcast(NewEvent(EventExportComplete, map[string]any{
			"job_id":    jobID,
			"row_count": result.RowCount,
			"file_size": result.FileSizeBytes,
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

// generateExport dispatches to the appropriate export generator based on the
// export type. If outputPath is non-empty, the result is written to disk
// (async mode); otherwise the data is returned in-memory (sync mode).
func (r *Router) generateExport(ctx context.Context, body exportRequest, maxRows int, outputPath string) (*export.ExportResult, error) {
	switch body.ExportType {
	case datastore.ExportTypeReadyNodes:
		return export.GenerateReadyNodeExport(ctx, r.db, export.ReadyNodeExportParams{
			TargetChefVersion: body.TargetChefVersion,
			Format:            body.Format,
			Filters:           body.Filters,
			MaxRows:           maxRows,
			OutputPath:        outputPath,
		})

	case datastore.ExportTypeBlockedNodes:
		return export.GenerateBlockedNodeExport(ctx, r.db, export.BlockedNodeExportParams{
			TargetChefVersion: body.TargetChefVersion,
			Format:            body.Format,
			Filters:           body.Filters,
			MaxRows:           maxRows,
			OutputPath:        outputPath,
		})

	case datastore.ExportTypeCookbookRemediation:
		return export.GenerateCookbookRemediationExport(ctx, r.db, export.CookbookRemediationExportParams{
			Format:     body.Format,
			Filters:    body.Filters,
			MaxRows:    maxRows,
			OutputPath: outputPath,
		})

	default:
		return nil, fmt.Errorf("unsupported export type: %s", body.ExportType)
	}
}

// estimateExportRows returns a rough count of rows the export will produce.
// This is used to decide between synchronous and asynchronous export modes.
// It uses quick count queries where available, falling back to a conservative
// default.
func (r *Router) estimateExportRows(req *http.Request, body exportRequest) int {
	ctx := req.Context()

	switch body.ExportType {
	case datastore.ExportTypeReadyNodes, datastore.ExportTypeBlockedNodes:
		orgs, err := r.db.ListOrganisations(ctx)
		if err != nil {
			return 0 // treat as small — will generate sync
		}
		orgs = export.FilterOrganisations(orgs, body.Filters.Organisation)

		totalEstimate := 0
		for _, org := range orgs {
			total, _, _, err := r.db.CountNodeReadiness(ctx, org.ID, body.TargetChefVersion)
			if err != nil {
				continue
			}
			totalEstimate += total
		}
		return totalEstimate

	case datastore.ExportTypeCookbookRemediation:
		orgs, err := r.db.ListOrganisations(ctx)
		if err != nil {
			return 0
		}
		orgs = export.FilterOrganisations(orgs, body.Filters.Organisation)

		totalEstimate := 0
		for _, org := range orgs {
			cbs, err := r.db.ListCookbooksByOrganisation(ctx, org.ID)
			if err != nil {
				continue
			}
			totalEstimate += len(cbs)
		}
		return totalEstimate
	}

	return 0
}

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
