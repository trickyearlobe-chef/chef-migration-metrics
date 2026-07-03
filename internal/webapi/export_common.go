// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
)

// exportPageSize is the number of rows fetched per page when streaming an
// export. One page is held in memory at a time.
const exportPageSize = 2000

// exportSpec describes how to export one list-view entity. There is exactly one
// spec per export type (nodes, cookbooks, roles, git_repos). All four share the
// generic streaming encoder; the spec supplies the columns, the paged row
// source, and a cheap count — each built from the SAME query params and the
// SAME datastore query as the corresponding list endpoint, so the export
// reproduces the current filtered list exactly.
type exportSpec struct {
	// Filename is the base of the download name: <Filename>_<date>.<ext>.
	Filename string

	// Columns define the CSV header / JSON keys (one source of truth for both).
	Columns []export.Column

	// NewSource builds a paged RowSource from the request's query params.
	NewSource func(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error)

	// Count returns the number of rows the export will produce, for the sync vs
	// async dispatch decision. It uses the same filter as NewSource.
	Count func(ctx context.Context, r *Router, req *http.Request) (int, error)

	// SupportsChefSearch is true only for nodes.
	SupportsChefSearch bool

	// ChefSearchName extracts the node name from a row (nodes only).
	ChefSearchName func(row any) string
}

// exportRegistry returns the export spec for each export type. It is a method so
// specs can close over the Router (db access, config, platform mappings).
func (r *Router) exportRegistry() map[string]exportSpec {
	return map[string]exportSpec{
		"nodes":     r.nodesExportSpec(),
		"cookbooks": r.cookbooksExportSpec(),
		"roles":     r.rolesExportSpec(),
		"git_repos": r.gitReposExportSpec(),
	}
}

// exportJobFilters is what we persist in export_jobs.filters for an async job:
// the raw list query string, so the background goroutine reconstructs the exact
// same request (and therefore the exact same filter) as the original call.
type exportJobFilters struct {
	Query string `json:"query"`
}

// marshalExportQuery serialises the request's raw query for async persistence.
func marshalExportQuery(req *http.Request) (json.RawMessage, error) {
	return json.Marshal(exportJobFilters{Query: req.URL.RawQuery})
}

// reconstructExportRequest rebuilds a GET request carrying the persisted query
// string, so the async path can call the same helpers (org/owner resolution,
// filter builders) as the sync path.
func reconstructExportRequest(ctx context.Context, filters json.RawMessage) (*http.Request, error) {
	var f exportJobFilters
	if len(filters) > 0 {
		if err := json.Unmarshal(filters, &f); err != nil {
			return nil, err
		}
	}
	return http.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/exports?"+f.Query, nil)
}

// streamExportTo runs the export described by spec to w in the given format,
// returning the number of rows written.
func (r *Router) streamExportTo(ctx context.Context, w io.Writer, spec exportSpec, req *http.Request, format string) (int, error) {
	src, err := spec.NewSource(ctx, r, req)
	if err != nil {
		return 0, err
	}
	switch format {
	case "json":
		return export.StreamJSON(ctx, w, spec.Columns, src)
	case "chef_search_query":
		return export.StreamChefSearchQuery(ctx, w, spec.ChefSearchName, src)
	default: // csv
		return export.StreamCSV(ctx, w, spec.Columns, src)
	}
}

// drainCount counts the rows a RowSource yields by consuming it. Used to count
// small, fully-materialised sources (roles/cookbooks under ownership) exactly.
func drainCount(ctx context.Context, src export.RowSource) (int, error) {
	n := 0
	for {
		rows, err := src.Next(ctx)
		if err != nil {
			return n, err
		}
		if len(rows) == 0 {
			return n, nil
		}
		n += len(rows)
	}
}

// countingWriter wraps an io.Writer to track the number of bytes written, so an
// async export can record file_size_bytes without buffering the whole file.
type countingWriter struct {
	w io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}
