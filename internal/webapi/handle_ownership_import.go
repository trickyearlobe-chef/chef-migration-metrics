// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

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
	case "last_commit_at", "first_commit_at", "commit_count", "author_name", "author_email":
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

	// Look up which committer emails are already registered as owners of
	// this git repo, so the frontend can pre-select them.
	ownerEmails, ownerErr := r.db.GetOwnerEmailsForGitRepo(ctx, cookbookName)
	if ownerErr != nil {
		// Non-fatal: log but don't fail the request. Worst case is that
		// the checkboxes are not pre-checked.
		r.logf("WARN", "cookbook-committers: looking up owner emails for %s: %v", cookbookName, ownerErr)
		ownerEmails = map[string]bool{}
	}

	// Build response items with the is_owner annotation.
	type committerWithOwner struct {
		datastore.GitRepoCommitter
		IsOwner bool `json:"is_owner"`
	}

	items := make([]committerWithOwner, len(committers))
	for i, c := range committers {
		items[i] = committerWithOwner{
			GitRepoCommitter: c,
			IsOwner:          ownerEmails[strings.ToLower(c.AuthorEmail)],
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"cookbook_name": cookbookName,
		"git_repo_url":  repoURL,
		"data":          items,
		"pagination":    NewPaginationResponse(pg, total),
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/cookbooks/:name/committers/assign — assign committers
// ---------------------------------------------------------------------------

// committerAssignment maps one email address that appears in a repository's
// history onto the person who owns it.
type committerAssignment struct {
	AuthorEmail string `json:"author_email"`
	OwnerName   string `json:"owner_name"`
	DisplayName string `json:"display_name"`
}

// assignCommittersRequest assigns the people who commit to one cookbook. Which
// cookbook is in the address.
type assignCommittersRequest struct {
	Committers []committerAssignment `json:"committers"`
}

func (r *Router) handleCookbookCommittersAssign(w http.ResponseWriter, req *http.Request, cookbookName string) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	ctx := req.Context()

	// Confirm the cookbook is git-sourced. The lookup matches git_repos on the
	// name, so the repo's name is the one we were called with — and that name,
	// not the URL it returns, is what an assignment is keyed by.
	if _, err := r.db.GetGitRepoURLForCookbook(ctx, cookbookName); errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Cookbook %q is not git-sourced or does not exist.", cookbookName))
		return
	} else if err != nil {
		r.logf("ERROR", "cookbook-committers-assign: looking up git repo for %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to look up cookbook git repository.")
		return
	}
	repoName := cookbookName

	var body assignCommittersRequest
	if !decodeJSONBody(w, req, &body) {
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

		// Normalise owner name to lowercase to satisfy the CHECK constraint
		// on owners.name (^[a-z0-9][a-z0-9._-]*$).
		c.OwnerName = strings.ToLower(c.OwnerName)

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

		// Record the commit address the person was recognised by, whether or
		// not the owner was created just now.
		//
		// One person commits under several addresses — a personal one, a
		// noreply one, an old domain — and every address after the first
		// finds the owner already there. Recording only on creation drops
		// them, which is the case this is for.
		//
		// It is recorded as a git address because that is what it is: commit
		// addresses and corporate addresses differ often enough that
		// conflating them attaches work to the wrong person.
		if _, aliasErr := r.db.InsertOwnerAlias(ctx, datastore.InsertOwnerAliasParams{
			OwnerName:  owner.Name,
			AliasType:  "git_email",
			AliasValue: c.AuthorEmail,
			Source:     "committer",
		}); aliasErr != nil && !errors.Is(aliasErr, datastore.ErrAlreadyExists) {
			// A missing alias costs later matching accuracy, not this
			// assignment.
			r.logf("WARN", "cookbook-committers-assign: seeding git_email alias %q for %s: %v",
				c.AuthorEmail, owner.Name, aliasErr)
		}

		// Create a git_repo assignment linking the owner to the repo.
		//
		// Keyed by the repo NAME, not its URL. Everything that lists repos —
		// the repo list's owner filter, the unowned filter, the export — reads
		// ownership by name, because repo URLs are volatile. Writing the URL
		// here recorded the owner where none of them could read it, so a repo
		// somebody had just claimed went on reading as unowned.
		_, err = r.db.InsertAssignment(ctx, datastore.InsertAssignmentParams{
			OwnerName:        owner.Name,
			EntityType:       "git_repo",
			EntityKey:        repoName,
			AssignmentSource: "manual",
			Confidence:       "definitive",
		})
		if err != nil {
			if errors.Is(err, datastore.ErrAlreadyExists) {
				skippedCount++
				continue
			}
			r.logf("ERROR", "cookbook-committers-assign: creating assignment for %s -> %s: %v", c.OwnerName, repoName, err)
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
		r.auditOwnership(req, "assignment_created", c.OwnerName, "git_repo", repoName, "", detailsJSON)

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
