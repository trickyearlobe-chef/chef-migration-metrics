// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// GET /api/v1/ownership/aliases?owner=<name>
// POST /api/v1/ownership/aliases (create single alias)
// DELETE /api/v1/ownership/aliases/<id>
// POST /api/v1/ownership/aliases/import (bulk CSV/JSON)
// GET /api/v1/ownership/aliases/suggest?q=<query>&limit=<n>
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipAliases(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handleListOwnerAliases(w, req)
	case http.MethodPost:
		r.handleCreateOwnerAlias(w, req)
	case http.MethodDelete:
		r.handleDeleteOwnerAlias(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"Allowed: GET, POST, DELETE")
	}
}

func (r *Router) handleListOwnerAliases(w http.ResponseWriter, req *http.Request) {
	ownerName := req.URL.Query().Get("owner")
	if ownerName == "" {
		WriteBadRequest(w, "owner query parameter is required.")
		return
	}

	aliases, err := r.db.GetOwnerAliasesByOwner(req.Context(), ownerName)
	if err != nil {
		r.logf("ERROR", "ownership/aliases: listing aliases for %s: %v", ownerName, err)
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to list aliases.")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{"aliases": aliases})
}

// createOwnerAliasRequest records another name a person is known by, so the
// next import recognises them.
type createOwnerAliasRequest struct {
	OwnerName  string `json:"owner_name"`
	AliasType  string `json:"alias_type"`
	AliasValue string `json:"alias_value"`
	Source     string `json:"source"`
}

func (r *Router) handleCreateOwnerAlias(w http.ResponseWriter, req *http.Request) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	var body createOwnerAliasRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	source := body.Source
	if source == "" {
		source = "manual"
	}

	alias, err := r.db.InsertOwnerAlias(req.Context(), datastore.InsertOwnerAliasParams{
		OwnerName:  body.OwnerName,
		AliasType:  body.AliasType,
		AliasValue: body.AliasValue,
		Source:     source,
	})
	if err != nil {
		if err == datastore.ErrAlreadyExists {
			WriteError(w, http.StatusConflict, "conflict",
				"This alias is already assigned to an owner.")
			return
		}
		if strings.Contains(err.Error(), "is required") {
			WriteBadRequest(w, err.Error())
			return
		}
		r.logf("ERROR", "ownership/aliases: creating alias: %v", err)
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to create alias.")
		return
	}

	WriteJSON(w, http.StatusCreated, alias)
}

func (r *Router) handleDeleteOwnerAlias(w http.ResponseWriter, req *http.Request) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	// Extract ID from path: /api/v1/ownership/aliases/<id>
	id := strings.TrimPrefix(req.URL.Path, "/api/v1/ownership/aliases/")
	if id == "" {
		WriteBadRequest(w, "Alias ID is required in the URL path.")
		return
	}

	if err := r.db.DeleteOwnerAlias(req.Context(), id); err != nil {
		if err == datastore.ErrNotFound {
			WriteError(w, http.StatusNotFound, ErrCodeNotFound, "Alias not found.")
			return
		}
		r.logf("ERROR", "ownership/aliases: deleting alias %s: %v", id, err)
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to delete alias.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleOwnershipAliasesImport handles POST /api/v1/ownership/aliases/import.
func (r *Router) handleOwnershipAliasesImport(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	ctx := req.Context()

	if err := req.ParseMultipartForm(10 << 20); err != nil {
		WriteBadRequest(w, "Invalid multipart/form-data request.")
		return
	}

	format := req.FormValue("format")
	if format != "csv" && format != "json" {
		WriteBadRequest(w, `format field is required and must be "csv" or "json".`)
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		WriteBadRequest(w, "file field is required.")
		return
	}
	defer func() { _ = file.Close() }()

	type aliasRow struct {
		OwnerName  string `json:"owner_name"`
		AliasType  string `json:"alias_type"`
		AliasValue string `json:"alias_value"`
		Source     string `json:"source"`
	}

	var rows []aliasRow

	switch format {
	case "csv":
		reader := csv.NewReader(file)
		reader.TrimLeadingSpace = true

		header, err := reader.Read()
		if err != nil {
			WriteBadRequest(w, "Failed to read CSV header.")
			return
		}
		expectedHeader := []string{"owner_name", "alias_type", "alias_value", "source"}
		if len(header) < 3 {
			WriteBadRequest(w, fmt.Sprintf("CSV header must have at least columns: %v", expectedHeader[:3]))
			return
		}

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				WriteBadRequest(w, fmt.Sprintf("Failed to parse CSV at line %d: %v", len(rows)+2, err))
				return
			}
			if len(record) < 3 {
				WriteBadRequest(w, fmt.Sprintf("CSV line %d has fewer than 3 columns.", len(rows)+2))
				return
			}
			row := aliasRow{
				OwnerName:  record[0],
				AliasType:  record[1],
				AliasValue: record[2],
			}
			if len(record) > 3 {
				row.Source = record[3]
			}
			rows = append(rows, row)
		}

	case "json":
		var body struct {
			Aliases []aliasRow `json:"aliases"`
		}
		if err := json.NewDecoder(file).Decode(&body); err != nil {
			WriteBadRequest(w, "Invalid or malformed JSON in uploaded file.")
			return
		}
		rows = body.Aliases
	}

	if len(rows) == 0 {
		WriteBadRequest(w, "No rows found in the uploaded file.")
		return
	}
	if len(rows) > 10000 {
		WriteBadRequest(w, fmt.Sprintf("Import is limited to 10,000 rows per request; got %d.", len(rows)))
		return
	}

	type importError struct {
		Line  int    `json:"line"`
		Error string `json:"error"`
	}

	var (
		imported   int
		skipped    int
		importErrs []importError
	)

	for i, row := range rows {
		lineNum := i + 2
		if format == "json" {
			lineNum = i + 1
		}

		source := row.Source
		if source == "" {
			source = "import"
		}

		_, err := r.db.InsertOwnerAlias(ctx, datastore.InsertOwnerAliasParams{
			OwnerName:  row.OwnerName,
			AliasType:  row.AliasType,
			AliasValue: row.AliasValue,
			Source:     source,
		})
		if err != nil {
			if err == datastore.ErrAlreadyExists {
				skipped++
				continue
			}
			importErrs = append(importErrs, importError{Line: lineNum, Error: err.Error()})
			continue
		}
		imported++
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"errors":   importErrs,
	})
}

// handleOwnershipAliasSuggest handles GET /api/v1/ownership/aliases/suggest.
func (r *Router) handleOwnershipAliasSuggest(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "GET only")
		return
	}

	query := req.URL.Query().Get("q")
	if query == "" {
		WriteBadRequest(w, "q query parameter is required.")
		return
	}

	limit := 10
	if l := req.URL.Query().Get("limit"); l != "" {
		if _, err := fmt.Sscanf(l, "%d", &limit); err != nil || limit <= 0 {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
	}

	suggestions, err := r.db.SuggestOwnerAliases(req.Context(), query, limit)
	if err != nil {
		r.logf("ERROR", "ownership/aliases/suggest: %v", err)
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Failed to search for suggestions.")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}
