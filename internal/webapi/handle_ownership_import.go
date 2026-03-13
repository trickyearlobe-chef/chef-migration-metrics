// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import — bulk import ownership assignments
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipImport(w http.ResponseWriter, req *http.Request) {
	if !r.requireOwnership(w) {
		return
	}
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	ctx := req.Context()

	// Parse the multipart form. Allow up to 10 MB in memory.
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
	defer file.Close()

	// Parse rows from the uploaded file.
	type importRow struct {
		Owner        string `json:"owner"`
		EntityType   string `json:"entity_type"`
		EntityKey    string `json:"entity_key"`
		Organisation string `json:"organisation"`
		Notes        string `json:"notes"`
	}

	var rows []importRow

	switch format {
	case "csv":
		reader := csv.NewReader(file)
		reader.TrimLeadingSpace = true

		// Read and validate header.
		header, err := reader.Read()
		if err != nil {
			WriteBadRequest(w, "Failed to read CSV header.")
			return
		}
		expectedHeader := []string{"owner", "entity_type", "entity_key", "organisation", "notes"}
		if len(header) < len(expectedHeader) {
			WriteBadRequest(w, fmt.Sprintf("CSV header must have columns: %v", expectedHeader))
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
			row := importRow{
				Owner:      record[0],
				EntityType: record[1],
				EntityKey:  record[2],
			}
			if len(record) > 3 {
				row.Organisation = record[3]
			}
			if len(record) > 4 {
				row.Notes = record[4]
			}
			rows = append(rows, row)
		}

	case "json":
		var body struct {
			Assignments []importRow `json:"assignments"`
		}
		if err := json.NewDecoder(file).Decode(&body); err != nil {
			WriteBadRequest(w, "Invalid or malformed JSON in uploaded file.")
			return
		}
		rows = body.Assignments
	}

	if len(rows) == 0 {
		WriteBadRequest(w, "No rows found in the uploaded file.")
		return
	}
	if len(rows) > 10000 {
		WriteBadRequest(w, fmt.Sprintf("Import is limited to 10,000 rows per request; got %d.", len(rows)))
		return
	}

	validEntityTypes := map[string]bool{
		"node":     true,
		"cookbook": true,
		"git_repo": true,
		"role":     true,
		"policy":   true,
	}

	// Pre-resolve unique owners and cache them.
	ownerCache := make(map[string]datastore.Owner)
	// Pre-resolve unique organisations and cache them.
	orgCache := make(map[string]datastore.Organisation)

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
		// Line numbers are 1-based; for CSV, line 1 is the header, so data
		// starts at line 2. For JSON we also use 1-based indexing.
		var lineNum int
		if format == "csv" {
			lineNum = i + 2 // +1 for header, +1 for 1-based
		} else {
			lineNum = i + 1
		}

		// Validate required fields.
		if row.Owner == "" {
			importErrs = append(importErrs, importError{Line: lineNum, Error: "owner is required"})
			continue
		}
		if row.EntityType == "" {
			importErrs = append(importErrs, importError{Line: lineNum, Error: "entity_type is required"})
			continue
		}
		if !validEntityTypes[row.EntityType] {
			importErrs = append(importErrs, importError{Line: lineNum, Error: fmt.Sprintf("entity_type %q is not valid", row.EntityType)})
			continue
		}
		if row.EntityKey == "" {
			importErrs = append(importErrs, importError{Line: lineNum, Error: "entity_key is required"})
			continue
		}

		// Resolve owner.
		owner, ok := ownerCache[row.Owner]
		if !ok {
			var err error
			owner, err = r.db.GetOwnerByName(ctx, row.Owner)
			if errors.Is(err, datastore.ErrNotFound) {
				importErrs = append(importErrs, importError{Line: lineNum, Error: fmt.Sprintf("owner %q not found", row.Owner)})
				continue
			}
			if err != nil {
				r.logf("ERROR", "ownership/import: looking up owner %s: %v", row.Owner, err)
				importErrs = append(importErrs, importError{Line: lineNum, Error: "failed to look up owner"})
				continue
			}
			ownerCache[row.Owner] = owner
		}

		// Resolve organisation if provided.
		var orgID string
		var orgName string
		if row.Organisation != "" {
			orgName = row.Organisation
			org, ok := orgCache[row.Organisation]
			if !ok {
				var err error
				org, err = r.db.GetOrganisationByName(ctx, row.Organisation)
				if errors.Is(err, datastore.ErrNotFound) {
					importErrs = append(importErrs, importError{Line: lineNum, Error: fmt.Sprintf("organisation %q not found", row.Organisation)})
					continue
				}
				if err != nil {
					r.logf("ERROR", "ownership/import: looking up org %s: %v", row.Organisation, err)
					importErrs = append(importErrs, importError{Line: lineNum, Error: "failed to look up organisation"})
					continue
				}
				orgCache[row.Organisation] = org
			}
			orgID = org.ID
		}

		// Create the assignment.
		_, err := r.db.InsertAssignment(ctx, datastore.InsertAssignmentParams{
			OwnerID:          owner.ID,
			EntityType:       row.EntityType,
			EntityKey:        row.EntityKey,
			OrganisationID:   orgID,
			AssignmentSource: "import",
			Confidence:       "definitive",
			Notes:            row.Notes,
		})
		if err != nil {
			if errors.Is(err, datastore.ErrAlreadyExists) {
				skipped++
				continue
			}
			r.logf("ERROR", "ownership/import: creating assignment at line %d: %v", lineNum, err)
			importErrs = append(importErrs, importError{Line: lineNum, Error: "failed to create assignment"})
			continue
		}

		// Audit the imported assignment.
		detailsJSON, _ := json.Marshal(map[string]any{
			"assignment_source": "import",
			"confidence":        "definitive",
		})
		r.auditOwnership(req, "assignment_created", row.Owner, row.EntityType, row.EntityKey, orgName, detailsJSON)

		imported++
	}

	r.logf("INFO", "ownership/import: imported=%d skipped=%d errors=%d", imported, skipped, len(importErrs))

	// Ensure errors is always a JSON array, not null.
	if importErrs == nil {
		importErrs = []importError{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"errors":   importErrs,
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/cookbooks/:name/committers — list git repo committers
// ---------------------------------------------------------------------------

func (r *Router) handleCookbookCommitters(w http.ResponseWriter, req *http.Request, cookbookName string) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Look up the git repo URL for this cookbook.
	repoURL, err := r.db.GetGitRepoURLForCookbook(ctx, cookbookName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Cookbook %q is not git-sourced or does not exist.", cookbookName))
		return
	}
	if err != nil {
		r.logf("ERROR", "cookbook-committers: looking up git repo for %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to look up cookbook git repository.")
		return
	}

	// Parse pagination.
	pg := ParsePagination(req)

	// Parse sort parameters.
	sortField := queryString(req, "sort", "last_commit_at")
	switch sortField {
	case "last_commit_at", "commit_count", "author_name":
		// valid
	default:
		sortField = "last_commit_at"
	}

	order := queryString(req, "order", "desc")
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	// Parse optional since filter.
	var since time.Time
	if s := req.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		} else {
			WriteBadRequest(w, "since must be a valid RFC3339 timestamp.")
			return
		}
	}

	committers, total, err := r.db.ListCommittersByRepo(ctx, datastore.CommitterListFilter{
		GitRepoURL: repoURL,
		Since:      since,
		Sort:       sortField,
		Order:      order,
		Limit:      pg.Limit(),
		Offset:     pg.Offset(),
	})
	if err != nil {
		r.logf("ERROR", "cookbook-committers: listing committers for %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to list committers.")
		return
	}

	// Ensure data is always a JSON array, not null.
	if committers == nil {
		committers = []datastore.GitRepoCommitter{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"cookbook_name": cookbookName,
		"git_repo_url":  repoURL,
		"data":          committers,
		"pagination":    NewPaginationResponse(pg, total),
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/cookbooks/:name/committers/assign — assign committers
// ---------------------------------------------------------------------------

func (r *Router) handleCookbookCommittersAssign(w http.ResponseWriter, req *http.Request, cookbookName string) {
	if !r.requireOwnership(w) {
		return
	}
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	ctx := req.Context()

	// Look up the git repo URL for this cookbook.
	repoURL, err := r.db.GetGitRepoURLForCookbook(ctx, cookbookName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Cookbook %q is not git-sourced or does not exist.", cookbookName))
		return
	}
	if err != nil {
		r.logf("ERROR", "cookbook-committers-assign: looking up git repo for %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to look up cookbook git repository.")
		return
	}

	var body struct {
		Committers []struct {
			AuthorEmail string `json:"author_email"`
			OwnerName   string `json:"owner_name"`
			DisplayName string `json:"display_name"`
		} `json:"committers"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	if len(body.Committers) == 0 {
		WriteBadRequest(w, "At least one committer is required.")
		return
	}

	var (
		ownersCreated      int
		assignmentsCreated int
		skippedCount       int
	)

	for i, c := range body.Committers {
		if c.AuthorEmail == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("committers[%d].author_email is required.", i))
			return
		}
		if c.OwnerName == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("committers[%d].owner_name is required.", i))
			return
		}

		// Look up or create the owner.
		owner, err := r.db.GetOwnerByName(ctx, c.OwnerName)
		if errors.Is(err, datastore.ErrNotFound) {
			// Create the owner as an individual.
			displayName := c.DisplayName
			if displayName == "" {
				displayName = c.OwnerName
			}
			owner, err = r.db.InsertOwner(ctx, datastore.InsertOwnerParams{
				Name:         c.OwnerName,
				DisplayName:  displayName,
				ContactEmail: c.AuthorEmail,
				OwnerType:    "individual",
			})
			if errors.Is(err, datastore.ErrAlreadyExists) {
				// Another concurrent request may have created it; try fetching again.
				owner, err = r.db.GetOwnerByName(ctx, c.OwnerName)
				if err != nil {
					r.logf("ERROR", "cookbook-committers-assign: re-fetching owner %s: %v", c.OwnerName, err)
					WriteInternalError(w, "Failed to look up owner.")
					return
				}
			} else if err != nil {
				r.logf("ERROR", "cookbook-committers-assign: creating owner %s: %v", c.OwnerName, err)
				WriteInternalError(w, "Failed to create owner.")
				return
			} else {
				ownersCreated++
				r.logf("INFO", "cookbook-committers-assign: created owner %q for cookbook %s", c.OwnerName, cookbookName)
			}
		} else if err != nil {
			r.logf("ERROR", "cookbook-committers-assign: looking up owner %s: %v", c.OwnerName, err)
			WriteInternalError(w, "Failed to look up owner.")
			return
		}

		// Create a git_repo assignment linking the owner to the cookbook's repo URL.
		_, err = r.db.InsertAssignment(ctx, datastore.InsertAssignmentParams{
			OwnerID:          owner.ID,
			EntityType:       "git_repo",
			EntityKey:        repoURL,
			AssignmentSource: "manual",
			Confidence:       "definitive",
		})
		if err != nil {
			if errors.Is(err, datastore.ErrAlreadyExists) {
				skippedCount++
				continue
			}
			r.logf("ERROR", "cookbook-committers-assign: creating assignment for %s -> %s: %v", c.OwnerName, repoURL, err)
			WriteInternalError(w, "Failed to create assignment.")
			return
		}

		// Audit the assignment.
		detailsJSON, _ := json.Marshal(map[string]any{
			"assignment_source": "manual",
			"confidence":        "definitive",
			"cookbook_name":     cookbookName,
			"author_email":      c.AuthorEmail,
		})
		r.auditOwnership(req, "assignment_created", c.OwnerName, "git_repo", repoURL, "", detailsJSON)

		assignmentsCreated++
	}

	r.logf("INFO", "cookbook-committers-assign: cookbook=%s owners_created=%d assignments_created=%d skipped=%d",
		cookbookName, ownersCreated, assignmentsCreated, skippedCount)

	WriteJSON(w, http.StatusOK, map[string]any{
		"owners_created":      ownersCreated,
		"assignments_created": assignmentsCreated,
		"skipped":             skippedCount,
	})
}

// ---------------------------------------------------------------------------
// Helpers (unexported, local to this file)
// ---------------------------------------------------------------------------
