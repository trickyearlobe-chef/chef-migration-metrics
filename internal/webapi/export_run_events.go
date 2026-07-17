// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
)

// runEventsExportSpec exports the current filtered Run events NODES rollup — the
// default tab. It reuses the SAME filter parser and datastore method as the list
// (Limit/Offset left at 0 → all matching nodes), so the export reproduces the
// on-screen list exactly, including the as-of `until` anchor when the UI sends it.
func (r *Router) runEventsExportSpec() exportSpec {
	return exportSpec{
		Filename:  "run_events",
		Columns:   runEventExportColumns(),
		NewSource: newRunEventsExportSource,
	}
}

func newRunEventsExportSource(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error) {
	if !r.liveConfig().Ingest.ShowsRunEvents() {
		return nil, fmt.Errorf("run events are not enabled")
	}
	f := convergeRunFilterFromValues(req.URL.Query()) // Limit/Offset 0 → all rows
	nodes, _, err := r.db.ListConvergeRunNodesFiltered(ctx, f)
	if err != nil {
		return nil, err
	}
	anyRows := make([]any, len(nodes))
	for i := range nodes {
		anyRows[i] = nodes[i]
	}
	return export.NewSliceSource(anyRows), nil
}

// runEventExportColumns is the single source of truth for the run-events export's
// CSV header and JSON keys. Each row is a node's latest matching run.
func runEventExportColumns() []export.Column {
	ri := func(row any) datastore.ConvergeRunListItem { return row.(datastore.ConvergeRunListItem) }
	return []export.Column{
		{Header: "organisation", Value: func(r any) any { return ri(r).Organisation }},
		{Header: "node_name", Value: func(r any) any { return ri(r).NodeName }},
		{Header: "status", Value: func(r any) any { return ri(r).Status }},
		{Header: "chef_version", Value: func(r any) any { return ri(r).ChefVersion }},
		{Header: "end_time", Value: func(r any) any { return ri(r).EndTime }},
		{Header: "run_list", Value: func(r any) any { return strings.Join(ri(r).RunList, " ") }},
		{Header: "cookbooks", Value: func(r any) any { return joinCookbooks(ri(r).Cookbooks) }},
		{Header: "failure_message", Value: func(r any) any { return errorField(ri(r).Error, "message") }},
		{Header: "failure_class", Value: func(r any) any { return errorField(ri(r).Error, "class") }},
		{Header: "failed_cookbook", Value: func(r any) any { return errorField(ri(r).FailedResource, "cookbook_name") }},
		{Header: "failed_recipe", Value: func(r any) any { return errorField(ri(r).FailedResource, "recipe_name") }},
		{Header: "run_id", Value: func(r any) any { return ri(r).RunID }},
		{Header: "shape", Value: func(r any) any { return ri(r).Shape }},
	}
}

// joinCookbooks renders the observed cookbooks map as "name=version" pairs.
func joinCookbooks(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for name, ver := range m {
		parts = append(parts, name+"="+ver)
	}
	return strings.Join(parts, " ")
}

// errorField pulls a single top-level string field out of a stored JSONB blob
// (error / failed_resource), returning "" when absent. Keeps the export flat
// without re-deriving failure detail.
func errorField(raw json.RawMessage, field string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}
