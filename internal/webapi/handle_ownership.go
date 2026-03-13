// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Owner filter helpers — used by list/dashboard handlers (Spec § 4.5)
// ---------------------------------------------------------------------------

// ownerFilter holds parsed owner filter parameters from a request.
type ownerFilter struct {
	OwnerNames []string // comma-separated owner names
	Unowned    bool     // true = only unowned entities
	Active     bool     // true if either OwnerNames or Unowned is set
}

// parseOwnerFilter extracts owner filter parameters from the request.
func parseOwnerFilter(req *http.Request) ownerFilter {
	f := ownerFilter{}
	if v := req.URL.Query().Get("owner"); v != "" {
		f.OwnerNames = strings.Split(v, ",")
		f.Active = true
	}
	if req.URL.Query().Get("unowned") == "true" {
		f.Unowned = true
		f.Active = true
	}
	return f
}

// validateOwnerFilter checks that the owner and unowned parameters are not
// both set. Returns true if valid; writes a 400 response and returns false
// if invalid.
func validateOwnerFilter(w http.ResponseWriter, f ownerFilter) bool {
	if len(f.OwnerNames) > 0 && f.Unowned {
		WriteBadRequest(w, "The 'owner' and 'unowned' parameters are mutually exclusive.")
		return false
	}
	return true
}

// resolveOwnedEntityKeys returns the set of entity keys owned by the given
// owners for the specified entity type. Returns nil if no filtering should
// be applied.
func (r *Router) resolveOwnedEntityKeys(ctx context.Context, ownerNames []string, entityType string) (map[string]bool, error) {
	keys := make(map[string]bool)
	for _, name := range ownerNames {
		assignments, _, err := r.db.ListAssignmentsByOwner(ctx, datastore.AssignmentListFilter{
			OwnerName:  name,
			EntityType: entityType,
			Limit:      10000, // generous limit
		})
		if err != nil {
			return nil, err
		}
		for _, a := range assignments {
			keys[a.EntityKey] = true
		}
	}
	return keys, nil
}

// resolveAllOwnedEntityKeys returns the set of all entity keys that have any
// owner for the specified entity type. Used for the "unowned" filter.
func (r *Router) resolveAllOwnedEntityKeys(ctx context.Context, entityType string) (map[string]bool, error) {
	owners, _, err := r.db.ListOwners(ctx, datastore.OwnerListFilter{Limit: 10000})
	if err != nil {
		return nil, err
	}
	keys := make(map[string]bool)
	for _, o := range owners {
		assignments, _, err := r.db.ListAssignmentsByOwner(ctx, datastore.AssignmentListFilter{
			OwnerName:  o.Name,
			EntityType: entityType,
			Limit:      10000,
		})
		if err != nil {
			return nil, err
		}
		for _, a := range assignments {
			keys[a.EntityKey] = true
		}
	}
	return keys, nil
}

// ownerNameRe validates owner names: lowercase alphanumeric, dots,
// underscores, hyphens; must start with alphanumeric.
var ownerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// requireOwnership returns false and writes a 404 if ownership is disabled.
func (r *Router) requireOwnership(w http.ResponseWriter) bool {
	if !r.cfg.Ownership.Enabled {
		WriteNotFound(w, "Ownership tracking is not enabled.")
		return false
	}
	return true
}

// requireOperatorOrAdmin checks that the user has at least operator role.
// Returns false and writes a 403 if not. When auth is not configured,
// returns true (development mode).
func requireOperatorOrAdmin(w http.ResponseWriter, req *http.Request) bool {
	info := auth.SessionFromContext(req.Context())
	if info == nil {
		// No auth configured — allow (dev mode).
		return true
	}
	if info.Role == "admin" || info.Role == "operator" {
		return true
	}
	WriteForbidden(w, "This action requires the operator or admin role.")
	return false
}

// requireAdminRole checks that the user has admin role.
func requireAdminRole(w http.ResponseWriter, req *http.Request) bool {
	info := auth.SessionFromContext(req.Context())
	if info == nil {
		return true
	}
	if info.Role == "admin" {
		return true
	}
	WriteForbidden(w, "This action requires the admin role.")
	return false
}

// ---------------------------------------------------------------------------
// GET/POST /api/v1/owners
// ---------------------------------------------------------------------------

func (r *Router) handleOwners(w http.ResponseWriter, req *http.Request) {
	if !r.requireOwnership(w) {
		return
	}

	// Exact match for collection endpoint.
	if req.URL.Path != "/api/v1/owners" {
		// Sub-path: /api/v1/owners/:name[/...]
		r.handleOwnerSubpath(w, req)
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.handleListOwners(w, req)
	case http.MethodPost:
		r.handleCreateOwner(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and POST.")
	}
}

func (r *Router) handleListOwners(w http.ResponseWriter, req *http.Request) {
	pg := ParsePagination(req)
	q := req.URL.Query()
	f := datastore.OwnerListFilter{
		OwnerType: q.Get("owner_type"),
		Search:    q.Get("search"),
		SortField: q.Get("sort"),
		SortDir:   q.Get("order"),
		Limit:     pg.Limit(),
		Offset:    pg.Offset(),
	}

	// Determine target chef version for readiness enrichment.
	targetVersion := q.Get("target_chef_version")
	if targetVersion == "" && len(r.cfg.TargetChefVersions) > 0 {
		targetVersion = r.cfg.TargetChefVersions[0]
	}

	owners, total, err := r.db.ListOwnersWithSummary(req.Context(), f, targetVersion)
	if err != nil {
		r.logf("ERROR", "ownership: listing owners: %v", err)
		WriteInternalError(w, "Failed to list owners.")
		return
	}

	type readinessSummary struct {
		TargetChefVersion string `json:"target_chef_version"`
		TotalNodes        int    `json:"total_nodes"`
		Ready             int    `json:"ready"`
		Blocked           int    `json:"blocked"`
		Stale             int    `json:"stale"`
	}

	type ownerResp struct {
		Name             string            `json:"name"`
		DisplayName      string            `json:"display_name,omitempty"`
		ContactEmail     string            `json:"contact_email,omitempty"`
		ContactChannel   string            `json:"contact_channel,omitempty"`
		OwnerType        string            `json:"owner_type"`
		Metadata         json.RawMessage   `json:"metadata,omitempty"`
		AssignmentCounts map[string]int    `json:"assignment_counts"`
		Readiness        *readinessSummary `json:"readiness,omitempty"`
		CreatedAt        time.Time         `json:"created_at"`
		UpdatedAt        time.Time         `json:"updated_at"`
	}

	data := make([]ownerResp, 0, len(owners))
	for _, o := range owners {
		resp := ownerResp{
			Name:           o.Name,
			DisplayName:    o.DisplayName,
			ContactEmail:   o.ContactEmail,
			ContactChannel: o.ContactChannel,
			OwnerType:      o.OwnerType,
			Metadata:       o.Metadata,
			AssignmentCounts: map[string]int{
				"node":     o.NodeCount,
				"cookbook": o.CookbookCount,
				"git_repo": o.GitRepoCount,
				"role":     o.RoleCount,
				"policy":   o.PolicyCount,
			},
			CreatedAt: o.CreatedAt,
			UpdatedAt: o.UpdatedAt,
		}

		if targetVersion != "" && o.TotalNodes > 0 {
			resp.Readiness = &readinessSummary{
				TargetChefVersion: targetVersion,
				TotalNodes:        o.TotalNodes,
				Ready:             o.ReadyNodes,
				Blocked:           o.BlockedNodes,
				Stale:             o.StaleNodes,
			}
		}

		data = append(data, resp)
	}

	WritePaginated(w, data, pg, total)
}

func (r *Router) handleCreateOwner(w http.ResponseWriter, req *http.Request) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	var body struct {
		Name           string          `json:"name"`
		DisplayName    string          `json:"display_name"`
		ContactEmail   string          `json:"contact_email"`
		ContactChannel string          `json:"contact_channel"`
		OwnerType      string          `json:"owner_type"`
		Metadata       json.RawMessage `json:"metadata"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	if body.Name == "" {
		WriteBadRequest(w, "name is required.")
		return
	}
	if !ownerNameRe.MatchString(body.Name) {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"name must match ^[a-z0-9][a-z0-9._-]*$ (lowercase, alphanumeric start).")
		return
	}
	if body.OwnerType == "" {
		WriteBadRequest(w, "owner_type is required.")
		return
	}
	validTypes := map[string]bool{"team": true, "individual": true, "business_unit": true, "cost_centre": true, "custom": true}
	if !validTypes[body.OwnerType] {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("owner_type %q is not valid. Must be one of: team, individual, business_unit, cost_centre, custom.", body.OwnerType))
		return
	}

	owner, err := r.db.InsertOwner(req.Context(), datastore.InsertOwnerParams{
		Name:           body.Name,
		DisplayName:    body.DisplayName,
		ContactEmail:   body.ContactEmail,
		ContactChannel: body.ContactChannel,
		OwnerType:      body.OwnerType,
		Metadata:       body.Metadata,
	})
	if err != nil {
		if errors.Is(err, datastore.ErrAlreadyExists) {
			WriteError(w, http.StatusConflict, "conflict",
				fmt.Sprintf("Owner %q already exists.", body.Name))
			return
		}
		r.logf("ERROR", "ownership: creating owner: %v", err)
		WriteInternalError(w, "Failed to create owner.")
		return
	}

	// Audit log.
	r.auditOwnership(req, "owner_created", owner.Name, "", "", "",
		json.RawMessage(fmt.Sprintf(`{"owner_type":%q}`, owner.OwnerType)))

	WriteJSON(w, http.StatusCreated, owner)
}

// ---------------------------------------------------------------------------
// /api/v1/owners/:name[/assignments[/:id]]
// ---------------------------------------------------------------------------

func (r *Router) handleOwnerSubpath(w http.ResponseWriter, req *http.Request) {
	segs := pathSegments(req.URL.Path, "/api/v1/owners/")
	if len(segs) == 0 {
		WriteNotFound(w, "Owner name is required.")
		return
	}

	ownerName := segs[0]

	// /api/v1/owners/:name
	if len(segs) == 1 {
		switch req.Method {
		case http.MethodGet:
			r.handleGetOwner(w, req, ownerName)
		case http.MethodPut:
			r.handleUpdateOwner(w, req, ownerName)
		case http.MethodDelete:
			r.handleDeleteOwner(w, req, ownerName)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
				"This endpoint supports GET, PUT, and DELETE.")
		}
		return
	}

	// /api/v1/owners/:name/assignments[/:id]
	if segs[1] == "assignments" {
		if len(segs) == 2 {
			switch req.Method {
			case http.MethodGet:
				r.handleListAssignments(w, req, ownerName)
			case http.MethodPost:
				r.handleCreateAssignments(w, req, ownerName)
			default:
				WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
					"This endpoint supports GET and POST.")
			}
			return
		}
		if len(segs) == 3 {
			assignmentID := segs[2]
			switch req.Method {
			case http.MethodDelete:
				r.handleDeleteAssignment(w, req, ownerName, assignmentID)
			default:
				WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
					"This endpoint supports DELETE.")
			}
			return
		}
	}

	WriteNotFound(w, fmt.Sprintf("Unknown ownership endpoint: %s", req.URL.Path))
}

// ---------------------------------------------------------------------------
// GET /api/v1/owners/:name
// ---------------------------------------------------------------------------

func (r *Router) handleGetOwner(w http.ResponseWriter, req *http.Request, name string) {
	owner, err := r.db.GetOwnerByName(req.Context(), name)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Owner %q not found.", name))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership: getting owner %s: %v", name, err)
		WriteInternalError(w, "Failed to get owner.")
		return
	}

	counts, _ := r.db.CountAssignmentsByOwner(req.Context(), name)
	if counts == nil {
		counts = map[string]int{}
	}
	for _, et := range []string{"node", "cookbook", "git_repo", "role", "policy"} {
		if _, ok := counts[et]; !ok {
			counts[et] = 0
		}
	}

	// Determine target chef version: prefer query param, fall back to first
	// configured version.
	targetVersion := req.URL.Query().Get("target_chef_version")
	if targetVersion == "" && len(r.cfg.TargetChefVersions) > 0 {
		targetVersion = r.cfg.TargetChefVersions[0]
	}

	resp := map[string]any{
		"name":              owner.Name,
		"display_name":      owner.DisplayName,
		"contact_email":     owner.ContactEmail,
		"contact_channel":   owner.ContactChannel,
		"owner_type":        owner.OwnerType,
		"metadata":          owner.Metadata,
		"assignment_counts": counts,
		"created_at":        owner.CreatedAt,
		"updated_at":        owner.UpdatedAt,
	}

	// Include enrichment summaries when a target version is available.
	if targetVersion != "" {
		ctx := req.Context()

		readiness, err := r.db.GetOwnerReadinessSummary(ctx, name, targetVersion)
		if err != nil {
			r.logf("WARN", "ownership: readiness summary for %s: %v", name, err)
		} else {
			resp["readiness_summary"] = readiness
		}

		cbSummary, err := r.db.GetOwnerCookbookSummary(ctx, name, targetVersion)
		if err != nil {
			r.logf("WARN", "ownership: cookbook summary for %s: %v", name, err)
		} else {
			resp["cookbook_summary"] = cbSummary
		}

		grSummary, err := r.db.GetOwnerGitRepoSummary(ctx, name, targetVersion)
		if err != nil {
			r.logf("WARN", "ownership: git repo summary for %s: %v", name, err)
		} else {
			resp["git_repo_summary"] = grSummary
		}
	}

	WriteJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// PUT /api/v1/owners/:name
// ---------------------------------------------------------------------------

func (r *Router) handleUpdateOwner(w http.ResponseWriter, req *http.Request, name string) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	var body struct {
		DisplayName    *string          `json:"display_name"`
		ContactEmail   *string          `json:"contact_email"`
		ContactChannel *string          `json:"contact_channel"`
		OwnerType      *string          `json:"owner_type"`
		Metadata       *json.RawMessage `json:"metadata"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	if body.OwnerType != nil {
		validTypes := map[string]bool{"team": true, "individual": true, "business_unit": true, "cost_centre": true, "custom": true}
		if !validTypes[*body.OwnerType] {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("owner_type %q is not valid.", *body.OwnerType))
			return
		}
	}

	updated, err := r.db.UpdateOwner(req.Context(), name, datastore.UpdateOwnerParams{
		DisplayName:    body.DisplayName,
		ContactEmail:   body.ContactEmail,
		ContactChannel: body.ContactChannel,
		OwnerType:      body.OwnerType,
		Metadata:       body.Metadata,
	})
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Owner %q not found.", name))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership: updating owner %s: %v", name, err)
		WriteInternalError(w, "Failed to update owner.")
		return
	}

	// Build list of changed fields for audit.
	var changed []string
	if body.DisplayName != nil {
		changed = append(changed, "display_name")
	}
	if body.ContactEmail != nil {
		changed = append(changed, "contact_email")
	}
	if body.ContactChannel != nil {
		changed = append(changed, "contact_channel")
	}
	if body.OwnerType != nil {
		changed = append(changed, "owner_type")
	}
	if body.Metadata != nil {
		changed = append(changed, "metadata")
	}
	detailsJSON, _ := json.Marshal(map[string]any{"changed_fields": changed})
	r.auditOwnership(req, "owner_updated", name, "", "", "", detailsJSON)

	WriteJSON(w, http.StatusOK, updated)
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/owners/:name
// ---------------------------------------------------------------------------

func (r *Router) handleDeleteOwner(w http.ResponseWriter, req *http.Request, name string) {
	if !requireAdminRole(w, req) {
		return
	}

	cascaded, err := r.db.DeleteOwner(req.Context(), name)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Owner %q not found.", name))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership: deleting owner %s: %v", name, err)
		WriteInternalError(w, "Failed to delete owner.")
		return
	}

	detailsJSON, _ := json.Marshal(map[string]any{"assignments_cascaded": cascaded})
	r.auditOwnership(req, "owner_deleted", name, "", "", "", detailsJSON)

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// GET /api/v1/owners/:name/assignments
// ---------------------------------------------------------------------------

func (r *Router) handleListAssignments(w http.ResponseWriter, req *http.Request, ownerName string) {
	pg := ParsePagination(req)
	f := datastore.AssignmentListFilter{
		OwnerName:        ownerName,
		EntityType:       req.URL.Query().Get("entity_type"),
		OrganisationName: req.URL.Query().Get("organisation"),
		AssignmentSource: req.URL.Query().Get("assignment_source"),
		Limit:            pg.Limit(),
		Offset:           pg.Offset(),
	}

	assignments, total, err := r.db.ListAssignmentsByOwner(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "ownership: listing assignments for %s: %v", ownerName, err)
		WriteInternalError(w, "Failed to list assignments.")
		return
	}

	WritePaginated(w, assignments, pg, total)
}

// ---------------------------------------------------------------------------
// POST /api/v1/owners/:name/assignments
// ---------------------------------------------------------------------------

func (r *Router) handleCreateAssignments(w http.ResponseWriter, req *http.Request, ownerName string) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	owner, err := r.db.GetOwnerByName(req.Context(), ownerName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Owner %q not found.", ownerName))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership: looking up owner %s: %v", ownerName, err)
		WriteInternalError(w, "Failed to look up owner.")
		return
	}

	var body struct {
		Assignments []struct {
			EntityType   string `json:"entity_type"`
			EntityKey    string `json:"entity_key"`
			Organisation string `json:"organisation"`
			Notes        string `json:"notes"`
		} `json:"assignments"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	if len(body.Assignments) == 0 {
		WriteBadRequest(w, "At least one assignment is required.")
		return
	}

	validEntityTypes := map[string]bool{"node": true, "cookbook": true, "git_repo": true, "role": true, "policy": true}

	var created []datastore.OwnershipAssignment
	for i, a := range body.Assignments {
		if !validEntityTypes[a.EntityType] {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("assignments[%d].entity_type %q is not valid.", i, a.EntityType))
			return
		}
		if a.EntityKey == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("assignments[%d].entity_key is required.", i))
			return
		}

		// Resolve organisation name to ID if provided.
		var orgID string
		if a.Organisation != "" {
			org, err := r.db.GetOrganisationByName(req.Context(), a.Organisation)
			if errors.Is(err, datastore.ErrNotFound) {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("assignments[%d].organisation %q not found.", i, a.Organisation))
				return
			}
			if err != nil {
				r.logf("ERROR", "ownership: looking up org %s: %v", a.Organisation, err)
				WriteInternalError(w, "Failed to look up organisation.")
				return
			}
			orgID = org.ID
		}

		assignment, err := r.db.InsertAssignment(req.Context(), datastore.InsertAssignmentParams{
			OwnerID:          owner.ID,
			EntityType:       a.EntityType,
			EntityKey:        a.EntityKey,
			OrganisationID:   orgID,
			AssignmentSource: "manual",
			Confidence:       "definitive",
			Notes:            a.Notes,
		})
		if err != nil {
			if errors.Is(err, datastore.ErrAlreadyExists) {
				WriteError(w, http.StatusConflict, "conflict",
					fmt.Sprintf("Duplicate assignment: %s %s already assigned to %s.", a.EntityType, a.EntityKey, ownerName))
				return
			}
			r.logf("ERROR", "ownership: creating assignment: %v", err)
			WriteInternalError(w, "Failed to create assignment.")
			return
		}

		// Audit.
		detailsJSON, _ := json.Marshal(map[string]any{
			"assignment_source": "manual",
			"confidence":        "definitive",
		})
		r.auditOwnership(req, "assignment_created", ownerName, a.EntityType, a.EntityKey, a.Organisation, detailsJSON)

		created = append(created, assignment)
	}

	WriteJSON(w, http.StatusCreated, map[string]any{
		"created":     len(created),
		"assignments": created,
	})
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/owners/:name/assignments/:id
// ---------------------------------------------------------------------------

func (r *Router) handleDeleteAssignment(w http.ResponseWriter, req *http.Request, ownerName string, assignmentID string) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	// Get the assignment details for audit logging.
	assignment, err := r.db.GetAssignment(req.Context(), assignmentID)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, "Assignment not found.")
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership: getting assignment %s: %v", assignmentID, err)
		WriteInternalError(w, "Failed to get assignment.")
		return
	}

	if err := r.db.DeleteAssignment(req.Context(), assignmentID); err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, "Assignment not found.")
			return
		}
		r.logf("ERROR", "ownership: deleting assignment %s: %v", assignmentID, err)
		WriteInternalError(w, "Failed to delete assignment.")
		return
	}

	detailsJSON, _ := json.Marshal(map[string]any{
		"assignment_source": assignment.AssignmentSource,
		"confidence":        assignment.Confidence,
	})
	r.auditOwnership(req, "assignment_deleted", ownerName, assignment.EntityType, assignment.EntityKey, "", detailsJSON)

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/reassign
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipReassign(w http.ResponseWriter, req *http.Request) {
	if !r.requireOwnership(w) {
		return
	}
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	var body struct {
		FromOwner         string `json:"from_owner"`
		ToOwner           string `json:"to_owner"`
		EntityType        string `json:"entity_type"`
		Organisation      string `json:"organisation"`
		DeleteSourceOwner bool   `json:"delete_source_owner"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	if body.FromOwner == "" || body.ToOwner == "" {
		WriteBadRequest(w, "from_owner and to_owner are required.")
		return
	}
	if body.FromOwner == body.ToOwner {
		WriteBadRequest(w, "from_owner and to_owner must be different.")
		return
	}

	// delete_source_owner requires admin.
	if body.DeleteSourceOwner {
		if !requireAdminRole(w, req) {
			return
		}
	}

	fromOwner, err := r.db.GetOwnerByName(req.Context(), body.FromOwner)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Source owner %q not found.", body.FromOwner))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership: looking up source owner: %v", err)
		WriteInternalError(w, "Failed to look up source owner.")
		return
	}

	toOwner, err := r.db.GetOwnerByName(req.Context(), body.ToOwner)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Target owner %q not found.", body.ToOwner))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership: looking up target owner: %v", err)
		WriteInternalError(w, "Failed to look up target owner.")
		return
	}

	// Resolve organisation name to ID.
	var orgID string
	if body.Organisation != "" {
		org, err := r.db.GetOrganisationByName(req.Context(), body.Organisation)
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Organisation %q not found.", body.Organisation))
			return
		}
		if err != nil {
			r.logf("ERROR", "ownership: looking up org: %v", err)
			WriteInternalError(w, "Failed to look up organisation.")
			return
		}
		orgID = org.ID
	}

	reassigned, skippedCount, err := r.db.ReassignOwnership(req.Context(), fromOwner.ID, toOwner.ID, body.EntityType, orgID)
	if err != nil {
		r.logf("ERROR", "ownership: reassigning: %v", err)
		WriteInternalError(w, "Failed to reassign ownership.")
		return
	}

	sourceDeleted := false
	if body.DeleteSourceOwner && reassigned+skippedCount > 0 {
		// Only delete if all assignments were covered.
		remaining, _ := r.db.CountAssignmentsByOwner(req.Context(), body.FromOwner)
		totalRemaining := 0
		for _, c := range remaining {
			totalRemaining += c
		}
		if totalRemaining == 0 {
			if _, err := r.db.DeleteOwner(req.Context(), body.FromOwner); err != nil {
				r.logf("WARN", "ownership: failed to delete source owner %s after reassignment: %v", body.FromOwner, err)
			} else {
				sourceDeleted = true
			}
		}
	}

	// Audit the reassignment.
	detailsJSON, _ := json.Marshal(map[string]any{
		"from_owner":           body.FromOwner,
		"to_owner":             body.ToOwner,
		"reassigned":           reassigned,
		"skipped":              skippedCount,
		"source_owner_deleted": sourceDeleted,
	})
	r.auditOwnership(req, "assignment_reassigned", body.ToOwner, "", "", "", detailsJSON)

	WriteJSON(w, http.StatusOK, map[string]any{
		"reassigned":           reassigned,
		"skipped":              skippedCount,
		"from_owner":           body.FromOwner,
		"to_owner":             body.ToOwner,
		"source_owner_deleted": sourceDeleted,
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/ownership/lookup
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipLookup(w http.ResponseWriter, req *http.Request) {
	if !r.requireOwnership(w) {
		return
	}
	if !requireGET(w, req) {
		return
	}

	entityType := req.URL.Query().Get("entity_type")
	entityKey := req.URL.Query().Get("entity_key")
	organisation := req.URL.Query().Get("organisation")

	if entityType == "" || entityKey == "" {
		WriteBadRequest(w, "entity_type and entity_key query parameters are required.")
		return
	}

	// Resolve organisation name to ID.
	var orgID string
	if organisation != "" {
		org, err := r.db.GetOrganisationByName(req.Context(), organisation)
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Organisation %q not found.", organisation))
			return
		}
		if err != nil {
			r.logf("ERROR", "ownership: looking up org for lookup: %v", err)
			WriteInternalError(w, "Failed to look up organisation.")
			return
		}
		orgID = org.ID
	}

	owners, err := r.db.LookupOwnership(req.Context(), entityType, entityKey, orgID)
	if err != nil {
		r.logf("ERROR", "ownership: looking up ownership: %v", err)
		WriteInternalError(w, "Failed to look up ownership.")
		return
	}

	if owners == nil {
		owners = []datastore.OwnershipLookupResult{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"entity_type":  entityType,
		"entity_key":   entityKey,
		"organisation": organisation,
		"owners":       owners,
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/ownership/audit-log
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipAuditLog(w http.ResponseWriter, req *http.Request) {
	if !r.requireOwnership(w) {
		return
	}
	if !requireGET(w, req) {
		return
	}

	pg := ParsePagination(req)
	f := datastore.AuditLogFilter{
		Action:     req.URL.Query().Get("action"),
		Actor:      req.URL.Query().Get("actor"),
		OwnerName:  req.URL.Query().Get("owner_name"),
		EntityType: req.URL.Query().Get("entity_type"),
		EntityKey:  req.URL.Query().Get("entity_key"),
		Limit:      pg.Limit(),
		Offset:     pg.Offset(),
	}

	if s := req.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Since = t
		}
	}
	if s := req.URL.Query().Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Until = t
		}
	}

	entries, total, err := r.db.ListAuditLog(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "ownership: listing audit log: %v", err)
		WriteInternalError(w, "Failed to list audit log.")
		return
	}

	WritePaginated(w, entries, pg, total)
}

// ---------------------------------------------------------------------------
// Ownership dispatch (for /api/v1/ownership/*)
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipEndpoints(w http.ResponseWriter, req *http.Request) {
	if !r.requireOwnership(w) {
		return
	}

	switch req.URL.Path {
	case "/api/v1/ownership/reassign":
		r.handleOwnershipReassign(w, req)
	case "/api/v1/ownership/lookup":
		r.handleOwnershipLookup(w, req)
	case "/api/v1/ownership/audit-log":
		r.handleOwnershipAuditLog(w, req)
	case "/api/v1/ownership/import":
		r.handleOwnershipImport(w, req)
	default:
		WriteNotFound(w, fmt.Sprintf("Unknown ownership endpoint: %s", req.URL.Path))
	}
}

// ---------------------------------------------------------------------------
// Audit helper
// ---------------------------------------------------------------------------

func (r *Router) auditOwnership(req *http.Request, action, ownerName, entityType, entityKey, organisation string, details json.RawMessage) {
	actor := adminUsername(req)

	if err := r.db.InsertAuditEntry(req.Context(), datastore.InsertAuditEntryParams{
		Action:       action,
		Actor:        actor,
		OwnerName:    ownerName,
		EntityType:   entityType,
		EntityKey:    entityKey,
		Organisation: organisation,
		Details:      details,
	}); err != nil {
		r.logf("WARN", "ownership: failed to write audit log: %v", err)
	}
}
