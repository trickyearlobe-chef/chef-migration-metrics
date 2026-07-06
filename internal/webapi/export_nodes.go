// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/platform"
)

// nodeExportRow is one enriched node row for export: the same nodeResp the list
// endpoint builds, plus the readiness entry for the target version and the
// derived install path.
type nodeExportRow struct {
	resp        nodeResp
	readiness   nodeReadinessSummaryEntry
	installPath string
	target      string
	tags        []string
}

// nodesExportSpec exports the current filtered Nodes list. Filters, org
// resolution, ownership, and target-version default all reuse the list handler's
// helpers, so the export reproduces the Nodes page exactly.
func (r *Router) nodesExportSpec() exportSpec {
	return exportSpec{
		Filename:           "nodes",
		Columns:            nodeExportColumns(),
		NewSource:          newNodeExportSource,
		SupportsChefSearch: true,
		ChefSearchName:     func(row any) string { return row.(nodeExportRow).resp.NodeName },
	}
}

// nodeExportSource streams node rows. Without an ownership filter it keyset-pages
// the shared filtered query over the unique (organisation_name, node_name)
// tuple. With an ownership filter it mirrors the list's in-memory path: load all
// matching rows, filter by ownership, then serve them in pages.
type nodeExportSource struct {
	r        *Router
	filter   datastore.NodeSnapshotFilter
	target   string
	mappings []platform.DisplayNameMapping

	cursor datastore.NodeSnapshotCursor
	done   bool

	preloaded []datastore.NodeSnapshot
	preIndex  int
	usePre    bool
}

func newNodeExportSource(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error) {
	f, target, err := r.nodeExportFilter(req)
	if err != nil {
		return nil, err
	}

	mappings, mErr := r.loadPlatformDisplayNames(ctx)
	if mErr != nil {
		r.logf("WARN", "loading platform display names for export: %v", mErr)
	}

	src := &nodeExportSource{r: r, filter: f, target: target, mappings: mappings}

	if of := parseOwnerFilter(req); of.Active {
		all, _, err := r.db.ListNodeSnapshotsFiltered(ctx, f) // f.Limit == 0 → all rows
		if err != nil {
			return nil, err
		}
		ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "node")
		if err != nil {
			return nil, err
		}
		src.preloaded = filterByOwnershipKey(all, ownedKeys, of, func(n datastore.NodeSnapshot) string { return n.NodeName })
		src.usePre = true
	}
	return src, nil
}

// nodeExportFilter builds the shared node filter (no pagination) plus the
// resolved target version used for readiness enrichment.
func (r *Router) nodeExportFilter(req *http.Request) (datastore.NodeSnapshotFilter, string, error) {
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		return datastore.NodeSnapshotFilter{}, "", err
	}
	orgIDs := make([]string, 0, len(orgs))
	for _, o := range orgs {
		orgIDs = append(orgIDs, o.Name)
	}

	cfg := r.liveConfig()
	f := nodeSnapshotFilterFromValues(req.URL.Query(), orgIDs,
		cfg.Collection.StaleNodeWarningHours, cfg.Collection.StaleNodeCriticalDays)

	target := f.TargetChefVersion
	if target == "" {
		target = r.defaultTargetVersion()
		f.TargetChefVersion = target
	}
	return f, target, nil
}

func (s *nodeExportSource) Next(ctx context.Context) ([]any, error) {
	if s.usePre {
		if s.preIndex >= len(s.preloaded) {
			return nil, nil
		}
		end := s.preIndex + exportPageSize
		if end > len(s.preloaded) {
			end = len(s.preloaded)
		}
		page := s.preloaded[s.preIndex:end]
		s.preIndex = end
		return s.enrich(ctx, page), nil
	}

	if s.done {
		return nil, nil
	}
	page, err := s.r.db.ListNodeSnapshotsForExport(ctx, s.filter, s.cursor, exportPageSize)
	if err != nil {
		return nil, err
	}
	if len(page) == 0 {
		s.done = true
		return nil, nil
	}
	if len(page) < exportPageSize {
		s.done = true
	}
	last := page[len(page)-1]
	s.cursor = datastore.NodeSnapshotCursor{
		OrganisationName: last.OrganisationName,
		NodeName:         last.NodeName,
		Valid:            true,
	}
	return s.enrich(ctx, page), nil
}

// enrich attaches readiness + display name to a page of snapshots, reusing the
// exact helpers the Nodes list uses (bulkLoadReadiness, buildNodeResp).
func (s *nodeExportSource) enrich(ctx context.Context, page []datastore.NodeSnapshot) []any {
	readinessByNode := bulkLoadReadiness(ctx, s.r.db, page, s.r)
	out := make([]any, 0, len(page))
	for _, n := range page {
		pdn := resolvePlatformDisplayName(n.Platform, n.PlatformVersion, s.mappings)
		entries := readinessByNode[n.NodeName]
		resp := s.r.buildNodeResp(n, entries, pdn)

		var sel nodeReadinessSummaryEntry
		for _, e := range entries {
			if e.TargetChefVersion == s.target {
				sel = e
				break
			}
		}
		out = append(out, nodeExportRow{
			resp:        resp,
			readiness:   sel,
			installPath: s.r.installPathForNode(n.Platform),
			target:      s.target,
			tags:        n.Tags,
		})
	}
	return out
}

// nodeExportColumns is the single source of truth for the node export's CSV
// header and JSON keys. Column scope: list columns + full 3-state readiness +
// disk detail (see specifications/web-api-exports.md).
func nodeExportColumns() []export.Column {
	nr := func(row any) nodeExportRow { return row.(nodeExportRow) }
	return []export.Column{
		{Header: "organisation_name", Value: func(r any) any { return nr(r).resp.OrganisationName }},
		{Header: "node_name", Value: func(r any) any { return nr(r).resp.NodeName }},
		{Header: "chef_environment", Value: func(r any) any { return nr(r).resp.ChefEnvironment }},
		{Header: "chef_version", Value: func(r any) any { return nr(r).resp.ChefVersion }},
		{Header: "platform", Value: func(r any) any { return nr(r).resp.Platform }},
		{Header: "platform_version", Value: func(r any) any { return nr(r).resp.PlatformVersion }},
		{Header: "platform_family", Value: func(r any) any { return nr(r).resp.PlatformFamily }},
		{Header: "platform_display_name", Value: func(r any) any { return strOrEmpty(nr(r).resp.PlatformDisplayName) }},
		{Header: "policy_name", Value: func(r any) any { return nr(r).resp.PolicyName }},
		{Header: "policy_group", Value: func(r any) any { return nr(r).resp.PolicyGroup }},
		// Tags: JSON emits a string array ([] when none); CSV joins into one
		// cell. Coalesce nil→[]string{} so JSON is [] not null and the CSV cell
		// is empty rather than "[]"/"null".
		{Header: "tags", Value: func(r any) any {
			if t := nr(r).tags; t != nil {
				return t
			}
			return []string{}
		}},
		{Header: "is_stale", Value: func(r any) any { return nr(r).resp.IsStale }},
		{Header: "staleness_tier", Value: func(r any) any { return stalenessLabel(nr(r).resp.StalenesTier) }},
		{Header: "ohai_time", Value: func(r any) any { return ohaiTimeISO(nr(r).resp.OhaiTime) }},
		{Header: "collected_at", Value: func(r any) any { return nr(r).resp.CollectedAt }},
		{Header: "target_chef_version", Value: func(r any) any { return nr(r).target }},
		{Header: "status", Value: func(r any) any { return nr(r).readiness.Status }},
		{Header: "cookstyle_status", Value: func(r any) any { return nr(r).readiness.CookstyleStatus }},
		{Header: "kitchen_status", Value: func(r any) any { return nr(r).readiness.KitchenStatus }},
		{Header: "all_cookbooks_compatible", Value: func(r any) any { return nr(r).readiness.AllCookbooksCompatible }},
		{Header: "blocking_cookbook_count", Value: func(r any) any { return nr(r).readiness.BlockingCookbookCount }},
		{Header: "review_cookbook_count", Value: func(r any) any { return nr(r).readiness.ReviewCookbookCount }},
		{Header: "disk_status", Value: func(r any) any { return nr(r).resp.DiskStatus }},
		{Header: "sufficient_disk_space", Value: func(r any) any { return nr(r).resp.SufficientDiskSpace }},
		{Header: "available_disk_mb", Value: func(r any) any { return nr(r).resp.AvailableDiskMB }},
		{Header: "required_disk_mb", Value: func(r any) any { return nr(r).resp.RequiredDiskMB }},
		{Header: "install_path", Value: func(r any) any { return nr(r).installPath }},
		{Header: "disk_detail", Value: func(r any) any { return strOrEmpty(nr(r).resp.DiskDetail) }},
		{Header: "migration_state", Value: func(r any) any { return nr(r).resp.MigrationState }},
		{Header: "target_converge_status", Value: func(r any) any { return nr(r).resp.TargetConvergeStatus }},
		{Header: "ready_to_activate", Value: func(r any) any { return nr(r).resp.ReadyToActivate }},
	}
}

// strOrEmpty renders a *string as its value or "" (nil), for export cells.
func strOrEmpty(p *string) any {
	if p == nil {
		return ""
	}
	return *p
}

// stalenessLabel maps the raw staleness tier to the wording the UI shows in its
// stale badge (fresh→Fresh, warning→Missing, critical→Gone), so the export reads
// the same as the Nodes list. Unknown values pass through unchanged.
func stalenessLabel(tier string) string {
	switch tier {
	case "fresh":
		return "Fresh"
	case "warning":
		return "Missing"
	case "critical":
		return "Gone"
	default:
		return tier
	}
}

// ohaiTimeISO renders a node's ohai_time (a unix epoch in seconds) as a UTC
// datetime string in the same format as collected_at. Returns "" when unset.
func ohaiTimeISO(epochSeconds float64) any {
	if epochSeconds <= 0 {
		return ""
	}
	return time.Unix(int64(epochSeconds), 0).UTC().Format("2006-01-02T15:04:05Z")
}
