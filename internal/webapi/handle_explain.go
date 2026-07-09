// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// handle_explain.go implements the admin EXPLAIN runner endpoints
// (performance-diagnostics "Layer 4"). Registered admin-only and independently
// of the request recorder — the EXPLAIN capability does not depend on request
// instrumentation being enabled.

// explainStatementTimeoutMs caps each EXPLAIN run. ANALYZE executes the query, so
// this bounds the cost of explaining a genuinely slow query.
const explainStatementTimeoutMs = 15000

// ---------------------------------------------------------------------------
// GET /api/v1/admin/performance/explain/catalog — list canned explains
// ---------------------------------------------------------------------------

func (r *Router) handleExplainCatalog(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "This endpoint supports GET.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"entries": emptySliceIfNil(r.db.ExplainCatalog()),
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/performance/explain — run EXPLAIN on a catalog entry or SQL
// ---------------------------------------------------------------------------

type explainRequest struct {
	CatalogKey string `json:"catalog_key"`
	SQL        string `json:"sql"`
	Analyze    bool   `json:"analyze"`
	RunTwice   bool   `json:"run_twice"`
}

type explainResponse struct {
	Label              string                `json:"label"`
	ParamSummary       string                `json:"param_summary"`
	SQL                string                `json:"sql,omitempty"`  // echoed for free-text only
	Note               string                `json:"note,omitempty"` // e.g. ANALYZE downgraded for a write
	Analyze            bool                  `json:"analyze"`
	StatementTimeoutMs int                   `json:"statement_timeout_ms"`
	CapturedAt         string                `json:"captured_at"`
	AppVersion         string                `json:"app_version"`
	Run1               datastore.ExplainRun  `json:"run1"`
	Run2               *datastore.ExplainRun `json:"run2,omitempty"`
}

func (r *Router) handleExplain(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "This endpoint supports POST.")
		return
	}

	var body explainRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid request body.")
		return
	}

	if body.CatalogKey == "" && body.SQL == "" {
		WriteBadRequest(w, "Provide either catalog_key or sql.")
		return
	}
	if body.CatalogKey != "" && body.SQL != "" {
		WriteBadRequest(w, "Provide only one of catalog_key or sql.")
		return
	}

	ctx := req.Context()

	var (
		sqlText, label, paramSummary string
		args                         []interface{}
		echoSQL                      string
	)

	if body.CatalogKey != "" {
		s, a, lbl, summary, err := r.db.ResolveCatalogExplain(ctx, body.CatalogKey, datastore.CatalogParams{
			DefaultTargetVersion: r.defaultTargetVersion(),
		})
		if err != nil {
			if errors.Is(err, datastore.ErrExplainUnavailable) {
				WriteBadRequest(w, "This diagnostic has no live sample data to run against yet.")
				return
			}
			WriteBadRequest(w, "Unknown or unresolvable catalog entry.")
			return
		}
		sqlText, args, label, paramSummary = s, a, lbl, summary
		// Catalog SQL is not echoed — a resolved plan/param summary may relate to
		// live identifiers; the caller runs it by key, not by text.
	} else {
		clean, err := datastore.ValidateFreeformExplainSQL(body.SQL)
		if err != nil {
			WriteBadRequest(w, "Only a single data statement (SELECT/WITH/INSERT/UPDATE/DELETE/MERGE) may be explained. Utility statements like COPY or VACUUM are not explainable.")
			return
		}
		sqlText = clean
		echoSQL = clean
		label = "Custom query"
		paramSummary = "free-text"
	}

	analyze := body.Analyze
	note := ""
	result, err := r.db.RunExplain(ctx, sqlText, args, datastore.ExplainOptions{
		Analyze:            analyze,
		RunTwice:           body.RunTwice,
		StatementTimeoutMs: explainStatementTimeoutMs,
	})
	// ANALYZE executes the statement; if it is a write, the READ ONLY transaction
	// rejects it. Rather than error, fall back to a plan-only EXPLAIN so the admin
	// still gets a plan for a hot write query.
	if err != nil && analyze && datastore.SQLWritesUnderReadOnly(err) {
		analyze = false
		note = "ANALYZE skipped: this statement writes data, which cannot run under the read-only guard. Showing a plan-only EXPLAIN."
		result, err = r.db.RunExplain(ctx, sqlText, args, datastore.ExplainOptions{
			Analyze:            false,
			RunTwice:           body.RunTwice,
			StatementTimeoutMs: explainStatementTimeoutMs,
		})
	}
	if err != nil {
		// Surface the underlying DB error (syntax, statement timeout, …) — this is
		// an admin diagnostic tool and the message is the useful signal.
		WriteError(w, http.StatusBadRequest, ErrCodeBadRequest, "EXPLAIN failed: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, explainResponse{
		Label:              label,
		ParamSummary:       paramSummary,
		SQL:                echoSQL,
		Note:               note,
		Analyze:            analyze,
		StatementTimeoutMs: explainStatementTimeoutMs,
		CapturedAt:         time.Now().UTC().Format(time.RFC3339),
		AppVersion:         r.version,
		Run1:               result.Run1,
		Run2:               result.Run2,
	})
}
