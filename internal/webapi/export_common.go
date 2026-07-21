// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
)

// exportPageSize is the number of rows fetched per page when streaming an
// export. One page is held in memory at a time.
const exportPageSize = 2000

// exportSpec describes how to export one list-view entity. There is exactly one
// spec per export type (nodes, cookbooks, roles, git_repos). All four share the
// generic streaming encoder; the spec supplies the columns and the paged row
// source, each built from the SAME query params and the SAME datastore query as
// the corresponding list endpoint, so the export reproduces the current filtered
// list exactly.
type exportSpec struct {
	// Filename is the base of the download name: <Filename>_<date>.<ext>.
	Filename string

	// Columns define the CSV header / JSON keys (one source of truth for both).
	Columns []export.Column

	// NewSource builds a paged RowSource from the request's query params.
	NewSource func(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error)

	// SupportsChefSearch is true only for nodes.
	SupportsChefSearch bool

	// ChefSearchName extracts the node name from a row (nodes only).
	ChefSearchName func(row any) string
}

// exportRegistry returns the export spec for each export type. It is a method so
// specs can close over the Router (db access, config, platform mappings).
func (r *Router) exportRegistry() map[string]exportSpec {
	return map[string]exportSpec{
		"nodes":      r.nodesExportSpec(),
		"cookbooks":  r.cookbooksExportSpec(),
		"roles":      r.rolesExportSpec(),
		"git_repos":  r.gitReposExportSpec(),
		"run_events": r.runEventsExportSpec(),
	}
}
