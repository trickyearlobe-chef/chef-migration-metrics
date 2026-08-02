// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
)

// gitReposExportSpec exports the current filtered Git Repos list. Git repos are
// few, so the list is materialised in full. Repos are not org-scoped, so the
// filtered query plus the ownership filter determines the row set.
func (r *Router) gitReposExportSpec() exportSpec {
	return exportSpec{
		Filename:  "git_repos",
		Columns:   gitRepoExportColumns(),
		NewSource: newGitRepoExportSource,
	}
}

func newGitRepoExportSource(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error) {
	f := gitRepoFilterFromValues(req.URL.Query()) // Limit/Offset 0 → all rows
	repos, _, err := r.db.ListGitReposFiltered(ctx, f)
	if err != nil {
		return nil, err
	}

	// Ownership is applied after the query for the same reason the list view
	// applies it there: assignments live in their own table, keyed on the repo
	// name, with no join to git_repos. Without this an export taken from a list
	// filtered by owner comes back as the whole estate, and nothing in the file
	// says which set it is.
	if of := parseOwnerFilter(req); of.Active {
		ownedKeys, oErr := r.resolveOwnershipFilter(ctx, of, "git_repo")
		if oErr != nil {
			return nil, oErr
		}
		repos = filterByOwnershipKey(repos, ownedKeys, of,
			func(gr datastore.GitRepo) string { return gr.Name })
	}

	anyRows := make([]any, len(repos))
	for i := range repos {
		anyRows[i] = repos[i]
	}
	return export.NewSliceSource(anyRows), nil
}

// gitRepoExportColumns is the single source of truth for the git-repo export's
// CSV header and JSON keys.
func gitRepoExportColumns() []export.Column {
	gr := func(row any) datastore.GitRepo { return row.(datastore.GitRepo) }
	return []export.Column{
		{Header: "name", Value: func(r any) any { return gr(r).Name }},
		{Header: "git_repo_url", Value: func(r any) any { return gr(r).GitRepoURL }},
		{Header: "head_commit_sha", Value: func(r any) any { return gr(r).HeadCommitSHA }},
		{Header: "default_branch", Value: func(r any) any { return gr(r).DefaultBranch }},
		{Header: "has_test_suite", Value: func(r any) any { return gr(r).HasTestSuite }},
		{Header: "clone_status", Value: func(r any) any { return gr(r).CloneStatus }},
		{Header: "clone_error", Value: func(r any) any { return gr(r).CloneError }},
		{Header: "compatibility_status", Value: func(r any) any { return gr(r).CompatibilityStatus }},
		{Header: "cookstyle_status", Value: func(r any) any { return gr(r).CookstyleStatus }},
		{Header: "tk_status", Value: func(r any) any { return gr(r).TKStatus }},
		{Header: "tk_passed", Value: func(r any) any { return gr(r).TKPassed }},
		{Header: "tk_total", Value: func(r any) any { return gr(r).TKTotal }},
		{Header: "kitchen_excluded", Value: func(r any) any { return gr(r).KitchenExcluded }},
		{Header: "kitchen_exclude_reason", Value: func(r any) any { return gr(r).KitchenExcludeReason }},
		{Header: "last_fetched_at", Value: func(r any) any { return gr(r).LastFetchedAt }},
	}
}
