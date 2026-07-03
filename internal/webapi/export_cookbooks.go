// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
)

// cookbooksExportSpec exports the current filtered Server Cookbooks list. The
// list is small (thousands of rows at most), so it is materialised in full and
// served via a SliceSource — reusing the same filter + query as the list page.
func (r *Router) cookbooksExportSpec() exportSpec {
	return exportSpec{
		Filename:  "cookbooks",
		Columns:   cookbookExportColumns(),
		NewSource: newCookbookExportSource,
	}
}

func (r *Router) cookbookExportFilter(req *http.Request) (datastore.CookbookFilter, ownerFilter, error) {
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		return datastore.CookbookFilter{}, ownerFilter{}, err
	}
	orgIDs := make([]string, 0, len(orgs))
	for _, o := range orgs {
		orgIDs = append(orgIDs, o.Name)
	}
	target := valueOr(req.URL.Query(), "target_chef_version", "")
	if target == "" {
		target = r.defaultTargetVersion()
	}
	f := cookbookFilterFromValues(req.URL.Query(), orgIDs, target) // Limit/Offset 0 → all rows
	return f, parseOwnerFilter(req), nil
}

func newCookbookExportSource(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error) {
	f, of, err := r.cookbookExportFilter(req)
	if err != nil {
		return nil, err
	}
	rows, _, err := r.db.ListCookbooksFiltered(ctx, f)
	if err != nil {
		return nil, err
	}
	if of.Active {
		ownedKeys, oErr := r.resolveOwnershipFilter(ctx, of, "cookbook")
		if oErr != nil {
			return nil, oErr
		}
		rows = filterByOwnershipKey(rows, ownedKeys, of, func(cb datastore.CookbookFilterRow) string { return cb.Name })
	}
	anyRows := make([]any, len(rows))
	for i := range rows {
		anyRows[i] = rows[i]
	}
	return export.NewSliceSource(anyRows), nil
}

// cookbookExportColumns is the single source of truth for the cookbook export's
// CSV header and JSON keys: list columns + status + migration metadata.
func cookbookExportColumns() []export.Column {
	cb := func(row any) datastore.CookbookFilterRow { return row.(datastore.CookbookFilterRow) }
	return []export.Column{
		{Header: "organisation_name", Value: func(r any) any { return cb(r).OrganisationName }},
		{Header: "name", Value: func(r any) any { return cb(r).Name }},
		{Header: "version", Value: func(r any) any { return cb(r).Version }},
		{Header: "is_active", Value: func(r any) any { return cb(r).IsActive }},
		{Header: "is_stale_cookbook", Value: func(r any) any { return cb(r).IsStaleCookbook }},
		{Header: "download_status", Value: func(r any) any { return cb(r).DownloadStatus }},
		{Header: "download_error", Value: func(r any) any { return cb(r).DownloadError }},
		{Header: "compatibility", Value: func(r any) any { return cb(r).Compatibility }},
		{Header: "cookstyle_status", Value: func(r any) any { return cb(r).CookstyleStatus }},
		{Header: "tk_status", Value: func(r any) any { return cb(r).TKStatus }},
		{Header: "is_frozen", Value: func(r any) any { return cb(r).IsFrozen }},
		{Header: "maintainer", Value: func(r any) any { return cb(r).Maintainer }},
		{Header: "description", Value: func(r any) any { return cb(r).Description }},
		{Header: "license", Value: func(r any) any { return cb(r).License }},
		{Header: "platforms", Value: func(r any) any { return cb(r).Platforms }},
		{Header: "dependencies", Value: func(r any) any { return cb(r).Dependencies }},
		{Header: "first_seen_at", Value: func(r any) any { return cb(r).FirstSeenAt }},
		{Header: "last_fetched_at", Value: func(r any) any { return cb(r).LastFetchedAt }},
	}
}
