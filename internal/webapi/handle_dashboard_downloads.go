// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"
	"sort"
)

// ---------------------------------------------------------------------------
// Dashboard — cookbook download status endpoint
// ---------------------------------------------------------------------------

// cookbookDownloadStatusResponse is what could not be fetched from the Chef
// servers, which is the difference between "this cookbook is compatible" and
// "we never got to look at it".
// failedCookbook is one cookbook version that could not be fetched, with what
// went wrong.
type failedCookbook struct {
	OrganisationName string `json:"organisation_name"`
	Name             string `json:"name"`
	Version          string `json:"version"`
	DownloadError    string `json:"download_error"`
	IsActive         bool   `json:"is_active"`
}

type downloadStatusCounts struct {
	OK      int `json:"ok"`
	Failed  int `json:"failed"`
	Pending int `json:"pending"`
}

type downloadStatusPercentages struct {
	OKPercent      float64 `json:"ok_percent"`
	FailedPercent  float64 `json:"failed_percent"`
	PendingPercent float64 `json:"pending_percent"`
}

type cookbookDownloadStatusResponse struct {
	TotalCookbooks      int                       `json:"total_cookbooks"`
	StatusCounts        downloadStatusCounts      `json:"status_counts"`
	StatusPercentages   downloadStatusPercentages `json:"status_percentages"`
	FailedCookbooks     []failedCookbook          `json:"failed_cookbooks"`
	FailedCookbookCount int                       `json:"failed_cookbook_count"`
	HasFailures         bool                      `json:"has_failures"`
	FailureMessage      string                    `json:"failure_message"`
}

// handleDashboardCookbookDownloadStatus handles
// GET /api/v1/dashboard/cookbook-download-status.
// Returns a summary of cookbook download statuses across all organisations,
// including aggregate counts by status and a list of failed cookbook versions
// with their error details so operators can investigate download failures.
func (r *Router) handleDashboardCookbookDownloadStatus(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for cookbook download status: %v", err)
		WriteInternalError(w, "Failed to compute cookbook download status.")
		return
	}

	// Aggregate counts by download status.
	statusCounts := map[string]int{
		"ok":      0,
		"failed":  0,
		"pending": 0,
	}
	totalCookbooks := 0

	var failedList []failedCookbook

	// Build an org name lookup for annotating failures.
	orgNameByID := make(map[string]string, len(orgs))
	for _, org := range orgs {
		orgNameByID[org.Name] = org.Name
	}

	for _, org := range orgs {
		serverCookbooks, cbErr := r.db.ListServerCookbooksByOrganisation(req.Context(), org.Name)
		if cbErr != nil {
			r.logf("WARN", "listing server cookbooks for org %s in download status: %v", org.Name, cbErr)
			continue
		}
		for _, sc := range serverCookbooks {
			totalCookbooks++
			status := sc.DownloadStatus
			if status == "" {
				status = "pending"
			}
			statusCounts[status]++

			if status == "failed" {
				failedList = append(failedList, failedCookbook{
					OrganisationName: sc.OrganisationName,
					Name:             sc.Name,
					Version:          sc.Version,
					DownloadError:    sc.DownloadError,
					IsActive:         sc.IsActive,
				})
			}
		}
	}

	// Sort failures: active cookbooks first (higher priority), then by
	// name and version for stable ordering.
	sort.Slice(failedList, func(i, j int) bool {
		if failedList[i].IsActive != failedList[j].IsActive {
			return failedList[i].IsActive // active first
		}
		if failedList[i].Name != failedList[j].Name {
			return failedList[i].Name < failedList[j].Name
		}
		return failedList[i].Version < failedList[j].Version
	})

	// Compute percentages.
	okPercent := 0.0
	failedPercent := 0.0
	pendingPercent := 0.0
	if totalCookbooks > 0 {
		okPercent = float64(statusCounts["ok"]) / float64(totalCookbooks) * 100
		failedPercent = float64(statusCounts["failed"]) / float64(totalCookbooks) * 100
		pendingPercent = float64(statusCounts["pending"]) / float64(totalCookbooks) * 100
	}

	if failedList == nil {
		failedList = []failedCookbook{}
	}

	WriteJSON(w, http.StatusOK, cookbookDownloadStatusResponse{
		TotalCookbooks: totalCookbooks,
		StatusCounts: downloadStatusCounts{
			OK:      statusCounts["ok"],
			Failed:  statusCounts["failed"],
			Pending: statusCounts["pending"],
		},
		StatusPercentages: downloadStatusPercentages{
			OKPercent:      okPercent,
			FailedPercent:  failedPercent,
			PendingPercent: pendingPercent,
		},
		FailedCookbooks:     failedList,
		FailedCookbookCount: len(failedList),
		HasFailures:         len(failedList) > 0,
		FailureMessage:      cookbookDownloadFailureMessage(len(failedList)),
	})
}

// cookbookDownloadFailureMessage returns a human-readable summary message
// for the dashboard.
func cookbookDownloadFailureMessage(failedCount int) string {
	if failedCount == 0 {
		return "All cookbook versions downloaded successfully."
	}
	return fmt.Sprintf(
		"%d cookbook version(s) failed to download. These versions are excluded from compatibility analysis. "+
			"They will be retried on the next collection run.",
		failedCount,
	)
}
