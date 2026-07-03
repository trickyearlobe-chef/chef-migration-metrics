// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
)

// rolesExportSpec exports the current filtered Roles list. Roles are few
// (hundreds), so the list is materialised in full. TK status is enriched and the
// tk_status filter applied in memory, exactly as the Roles list handler does, so
// the exported row set matches the page.
func (r *Router) rolesExportSpec() exportSpec {
	return exportSpec{
		Filename:  "roles",
		Columns:   roleExportColumns(),
		NewSource: newRoleExportSource,
		Count:     countRoleExport,
	}
}

func (r *Router) roleExportSetup(req *http.Request) (datastore.RoleFilter, []string, string, error) {
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		return datastore.RoleFilter{}, nil, "", err
	}
	orgNames := make([]string, 0, len(orgs))
	for _, o := range orgs {
		orgNames = append(orgNames, o.Name)
	}
	target := valueOr(req.URL.Query(), "target_chef_version", "")
	if target == "" {
		target = r.defaultTargetVersion()
	}
	f := roleFilterFromValues(req.URL.Query(), orgNames, target) // Limit/Offset 0 → all rows
	return f, orgNames, target, nil
}

func newRoleExportSource(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error) {
	f, orgNames, target, err := r.roleExportSetup(req)
	if err != nil {
		return nil, err
	}
	rows, _, _, err := r.db.ListRolesFiltered(ctx, f)
	if err != nil {
		return nil, err
	}

	// Enrich with TK status, then apply the tk_status filter in memory — the same
	// post-query steps the Roles list handler performs (TK status is not a SQL
	// column). Order (tk sort) is intentionally not reproduced; the consumer
	// re-sorts an exported file.
	if target != "" && len(rows) > 0 {
		names := make([]string, 0, len(rows))
		for _, row := range rows {
			names = append(names, row.RoleName)
		}
		if tkMap, tkErr := r.db.GetRoleTKStatuses(ctx, names, orgNames, target); tkErr == nil {
			for i := range rows {
				if status, ok := tkMap[rows[i].RoleName]; ok {
					rows[i].TKStatus = status
				}
			}
		}
	}
	if tk := req.URL.Query().Get("tk_status"); tk != "" {
		allowed := make(map[string]bool)
		for _, v := range strings.Split(tk, ",") {
			allowed[strings.TrimSpace(v)] = true
		}
		filtered := rows[:0]
		for _, row := range rows {
			tkVal := row.TKStatus
			if tkVal == "" {
				tkVal = "untested"
			}
			if allowed[tkVal] {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}

	anyRows := make([]any, len(rows))
	for i := range rows {
		anyRows[i] = rows[i]
	}
	return export.NewSliceSource(anyRows), nil
}

func countRoleExport(ctx context.Context, r *Router, req *http.Request) (int, error) {
	// Roles are small and TK filtering is post-query, so counting the fully
	// materialised source is both exact and cheap.
	src, err := newRoleExportSource(ctx, r, req)
	if err != nil {
		return 0, err
	}
	return drainCount(ctx, src)
}

// roleExportColumns is the single source of truth for the role export's CSV
// header and JSON keys.
func roleExportColumns() []export.Column {
	rr := func(row any) datastore.RoleFilterRow { return row.(datastore.RoleFilterRow) }
	return []export.Column{
		{Header: "role_name", Value: func(r any) any { return rr(r).RoleName }},
		{Header: "organisations", Value: func(r any) any { return strings.Join(rr(r).Organisations, ";") }},
		{Header: "node_count", Value: func(r any) any { return rr(r).NodeCount }},
		{Header: "direct_cookbook_count", Value: func(r any) any { return rr(r).DirectCookbookCount }},
		{Header: "transitive_cookbook_count", Value: func(r any) any { return rr(r).TransitiveCookbookCount }},
		{Header: "total_cookbook_count", Value: func(r any) any { return rr(r).TotalCookbookCount }},
		{Header: "compatible_count", Value: func(r any) any { return rr(r).CompatibleCount }},
		{Header: "incompatible_count", Value: func(r any) any { return rr(r).IncompatibleCount }},
		{Header: "untested_count", Value: func(r any) any { return rr(r).UntestedCount }},
		{Header: "compatibility_status", Value: func(r any) any { return rr(r).CompatibilityStatus }},
		{Header: "tk_status", Value: func(r any) any { return rr(r).TKStatus }},
	}
}
