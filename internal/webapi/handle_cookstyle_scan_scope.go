// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// /api/v1/cookstyle/scan-scope — which files a converge never executes
// ---------------------------------------------------------------------------
//
// The curated seed list lives in code and reaches files with predictable names.
// It cannot reach a script that only runs because a build job invokes it: that
// sits at a different path in every customer's repositories. This endpoint is
// the operator's half, and the reason the list is a decision somebody makes and
// can see rather than a rule inferred. See journeys/scan-trust.md.
//
// GET is open to any signed-in user on purpose — being judged by a list you
// cannot read is the thing this exists to prevent. Writing is admin-only,
// because changing what counts as cookbook code changes every verdict in the
// estate.

// scanScopeEntry is one row of the effective list as a reader sees it: the
// pattern, whether it is excluded, why, and where the decision came from.
type scanScopeEntry struct {
	Pattern string `json:"pattern"`

	// Excluded is false only for a seeded pattern somebody has overturned. Such
	// a row stays in the list rather than vanishing, so the decision can be
	// found and reversed.
	Excluded bool `json:"excluded"`

	Reason string `json:"reason"`

	// Source is "curated" for a seeded entry standing unmodified, "operator" for
	// anything a person decided — including an overturned default.
	Source string `json:"source"`

	CreatedBy string `json:"created_by,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// scanScopeListResponse is the whole effective list, seeded and operator
// entries together.
type scanScopeListResponse struct {
	Data []scanScopeEntry `json:"data"`
}

// scanScopePutRequest records or revises one decision.
type scanScopePutRequest struct {
	Pattern  string `json:"pattern"`
	Excluded bool   `json:"excluded"`
	Reason   string `json:"reason"`
}

// handleCookstyleScanScope dispatches /api/v1/cookstyle/scan-scope.
func (r *Router) handleCookstyleScanScope(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.getCookstyleScanScope(w, req)
	case http.MethodPut:
		if !requireAdminRole(w, req) {
			return
		}
		r.putCookstyleScanScope(w, req)
	case http.MethodDelete:
		if !requireAdminRole(w, req) {
			return
		}
		r.deleteCookstyleScanScope(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET, PUT and DELETE.")
	}
}

// getCookstyleScanScope returns the effective list: the seeded entries with the
// operator's decisions layered over them, each saying which it is.
func (r *Router) getCookstyleScanScope(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	rows, err := r.db.ListScanScopeExclusions(ctx)
	if err != nil {
		// The seeded list is still worth showing, and is what the scans are
		// using in this state too (see analysis.NewScanScopeFromStore).
		r.logf("ERROR", "listing scan scope exclusions: %v", err)
		rows = nil
	}

	decided := make(map[string]datastore.ScanScopeExclusion, len(rows))
	for _, row := range rows {
		decided[row.Pattern] = row
	}

	entries := make([]scanScopeEntry, 0, len(analysis.DefaultScanScopeExclusions())+len(rows))
	seeded := make(map[string]bool)

	for _, ex := range analysis.DefaultScanScopeExclusions() {
		seeded[ex.Pattern] = true
		if row, ok := decided[ex.Pattern]; ok {
			entries = append(entries, scanScopeEntryFromRow(row))
			continue
		}
		entries = append(entries, scanScopeEntry{
			Pattern:  ex.Pattern,
			Excluded: true,
			Reason:   ex.Reason,
			Source:   "curated",
		})
	}

	for _, row := range rows {
		if seeded[row.Pattern] {
			continue
		}
		entries = append(entries, scanScopeEntryFromRow(row))
	}

	WriteJSON(w, http.StatusOK, scanScopeListResponse{Data: entries})
}

func scanScopeEntryFromRow(row datastore.ScanScopeExclusion) scanScopeEntry {
	e := scanScopeEntry{
		Pattern:   row.Pattern,
		Excluded:  row.Excluded,
		Reason:    row.Reason,
		Source:    "operator",
		CreatedBy: row.CreatedBy,
	}
	if !row.UpdatedAt.IsZero() {
		e.UpdatedAt = row.UpdatedAt.Format("2006-01-02T15:04:05Z")
	}
	return e
}

func (r *Router) putCookstyleScanScope(w http.ResponseWriter, req *http.Request) {
	var body scanScopePutRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid JSON request body.")
		return
	}

	pattern := strings.TrimSpace(body.Pattern)
	if pattern == "" {
		WriteBadRequest(w, "A pattern is required. Use a path like Rakefile, or a directory like tooling/ci/*.")
		return
	}

	// An exclusion without a recorded reason is how the blocked list gets made
	// to look good. Required in both directions: overturning a seeded pattern is
	// as much of a claim as adding one.
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		WriteBadRequest(w, "A reason is required — it is what makes this decision checkable by somebody else.")
		return
	}

	ctx := req.Context()
	if err := r.db.UpsertScanScopeExclusion(ctx, pattern, body.Excluded, reason, adminUsername(req)); err != nil {
		r.logf("ERROR", "upserting scan scope exclusion %q: %v", pattern, err)
		WriteInternalError(w, "Failed to save the decision.")
		return
	}

	// Verdicts, prevalence counts and complexity all read the scope live, so
	// this takes effect on the next scan and the next page load. Existing stored
	// verdicts are re-derived by the usual rescan/reclassification paths.
	r.auditCookstyle(req, "scan_scope_changed", pattern, r.defaultTargetVersion(), map[string]any{
		"excluded": body.Excluded,
		"reason":   reason,
	})

	WriteJSON(w, http.StatusOK, map[string]any{
		"pattern":  pattern,
		"excluded": body.Excluded,
		"status":   "saved",
	})
}

func (r *Router) deleteCookstyleScanScope(w http.ResponseWriter, req *http.Request) {
	pattern := strings.TrimSpace(queryString(req, "pattern", ""))
	if pattern == "" {
		WriteBadRequest(w, "A pattern is required.")
		return
	}

	ctx := req.Context()
	if err := r.db.DeleteScanScopeExclusion(ctx, pattern); err != nil {
		r.logf("ERROR", "deleting scan scope exclusion %q: %v", pattern, err)
		WriteInternalError(w, "Failed to remove the decision.")
		return
	}

	// A seeded pattern returns to its curated behaviour; an operator-only
	// pattern stops applying at all.
	r.auditCookstyle(req, "scan_scope_decision_removed", pattern, r.defaultTargetVersion(), nil)

	WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}
