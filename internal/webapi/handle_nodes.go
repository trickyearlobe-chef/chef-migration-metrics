// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/staleness"
)

// nodeResp is the JSON response struct for a single node in list endpoints.
// Defined at package level to avoid duplication across handleNodes and
// handleNodesWithOwnerFilter.
type nodeResp struct {
	OrganisationName    string                      `json:"organisation_name"`
	NodeName            string                      `json:"node_name"`
	ChefEnvironment     string                      `json:"chef_environment,omitempty"`
	ChefVersion         string                      `json:"chef_version,omitempty"`
	Platform            string                      `json:"platform,omitempty"`
	PlatformVersion     string                      `json:"platform_version,omitempty"`
	PlatformFamily      string                      `json:"platform_family,omitempty"`
	PlatformDisplayName *string                     `json:"platform_display_name,omitempty"`
	PolicyName          string                      `json:"policy_name,omitempty"`
	PolicyGroup         string                      `json:"policy_group,omitempty"`
	IsStale             bool                        `json:"is_stale"`
	StalenesTier        string                      `json:"staleness_tier"`
	OhaiTimeAgeHours    float64                     `json:"ohai_time_age_hours,omitempty"`
	OhaiTime            float64                     `json:"ohai_time,omitempty"`
	CollectedAt         string                      `json:"collected_at"`
	Readiness           []nodeReadinessSummaryEntry `json:"readiness,omitempty"`

	// Node-level, version-invariant disk verdict (from the snapshot — migration
	// 0037), so the list disk badge is correct even when no target version is
	// configured or the node has no readiness rows yet.
	DiskStatus          string  `json:"disk_status"`
	DiskDetail          *string `json:"disk_detail,omitempty"`
	SufficientDiskSpace *bool   `json:"sufficient_disk_space,omitempty"`
	AvailableDiskMB     *int    `json:"available_disk_mb,omitempty"`
	RequiredDiskMB      *int    `json:"required_disk_mb,omitempty"`

	// Parallel deployment tracking
	MigrationState       string `json:"migration_state,omitempty"`
	TargetConvergeStatus string `json:"target_converge_status,omitempty"`
	ReadyToActivate      bool   `json:"ready_to_activate,omitempty"`
}

// buildNodeResp constructs a nodeResp from a NodeSnapshot, computing the
// staleness tier from ohai_time at response time.
func (r *Router) buildNodeResp(n datastore.NodeSnapshot, readiness []nodeReadinessSummaryEntry, platformDisplayName *string) nodeResp {
	now := time.Now()
	thresholds := staleness.Thresholds{
		WarningHours: r.liveConfig().Collection.StaleNodeWarningHours,
		CriticalDays: r.liveConfig().Collection.StaleNodeCriticalDays,
	}

	var ohaiTime time.Time
	if n.OhaiTime > 0 {
		ohaiTime = time.Unix(int64(n.OhaiTime), 0)
	}

	tier := staleness.ComputeTier(ohaiTime, now, thresholds)

	var ageHours float64
	if !ohaiTime.IsZero() {
		ageHours = now.Sub(ohaiTime).Hours()
	}

	installPath := r.installPathForNode(n.Platform)

	return nodeResp{
		OrganisationName:     n.OrganisationName,
		NodeName:             n.NodeName,
		ChefEnvironment:      n.ChefEnvironment,
		ChefVersion:          n.ChefVersion,
		Platform:             n.Platform,
		PlatformVersion:      n.PlatformVersion,
		PlatformFamily:       n.PlatformFamily,
		PlatformDisplayName:  platformDisplayName,
		PolicyName:           n.PolicyName,
		PolicyGroup:          n.PolicyGroup,
		IsStale:              n.IsStale,
		StalenesTier:         string(tier),
		OhaiTimeAgeHours:     ageHours,
		OhaiTime:             n.OhaiTime,
		CollectedAt:          n.CollectedAt.Format("2006-01-02T15:04:05Z"),
		Readiness:            readiness,
		DiskStatus:           diskStatusFor(n.SufficientDiskSpace),
		DiskDetail:           diskDetailFor(n.SufficientDiskSpace, n.AvailableDiskMB, n.RequiredDiskMB, installPath),
		SufficientDiskSpace:  n.SufficientDiskSpace,
		AvailableDiskMB:      n.AvailableDiskMB,
		RequiredDiskMB:       n.RequiredDiskMB,
		MigrationState:       migrationStateLabel(n.MigrationState),
		TargetConvergeStatus: n.TargetConvergeStatus,
		ReadyToActivate:      n.MigrationState == "hab_dormant" && n.TargetConvergeStatus == "success",
	}
}

// migrationStateLabel maps raw migration_state values to UI-friendly labels.
// Only Staged/Activated are meaningful; omnibus_only ("Current only") is retired
// to "—" (its absence can't be distinguished from a node the cookbook never
// reached, and it isn't a valid state for later hab→hab migrations). Returns ""
// only when state is empty (migration cookbook not deployed / no data).
func migrationStateLabel(raw string) string {
	switch raw {
	case "hab_dormant":
		return "Staged"
	case "hab_active":
		return "Activated"
	case "":
		return ""
	default:
		return "—"
	}
}

// handleNodes handles GET /api/v1/nodes — lists all node snapshots across
// all organisations, optionally filtered by query parameters.
//
// Filters are pushed down to SQL WHERE clauses for scalability. The in-memory
// filterNodes path is only used as a fallback when ownership filtering is
// active (which requires cross-referencing ownership assignments).
func (r *Router) handleNodes(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for nodes: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	// Build a map from organisation ID to name so we can include the
	// human-readable org name in each node response row. The node detail
	// endpoint uses org name in the URL path, so the frontend needs it
	// for constructing links.
	orgNameByID := make(map[string]string, len(orgs))
	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgNameByID[org.Name] = org.Name
		orgIDs = append(orgIDs, org.Name)
	}

	pg := ParsePagination(req)

	// Build SQL filter from query parameters.
	f := nodeSnapshotFilterFromRequest(req, orgIDs, r.liveConfig().Collection.StaleNodeWarningHours, r.liveConfig().Collection.StaleNodeCriticalDays)
	f.Limit = pg.Limit()
	f.Offset = pg.Offset()

	// When ownership filtering is active, we need to apply it post-query
	// because ownership is tracked in a separate assignments table and
	// can't easily be pushed into the node snapshot SQL. In this case,
	// we fall back to the in-memory path but still use SQL for the other
	// filters (without pagination, which we apply after ownership
	// filtering).
	ownerFilterActive := of.Active
	if ownerFilterActive {
		r.handleNodesWithOwnerFilter(w, req, orgs, orgNameByID, of, pg)
		return
	}

	// --- SQL push-down path (no ownership filter) ---

	nodes, total, err := r.db.ListNodeSnapshotsFiltered(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "listing filtered nodes: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	// Bulk-load readiness data for the page's nodes (1 query per org
	// instead of 1 per node). Index by node_name for O(1) lookup.
	readinessByNodeName := bulkLoadReadiness(req.Context(), r.db, nodes, r)

	// Load platform display name mappings (non-fatal on error).
	mappings, mErr := r.loadPlatformDisplayNames(req.Context())
	if mErr != nil {
		r.logf("WARN", "loading platform display names: %v", mErr)
	}

	result := make([]nodeResp, 0, len(nodes))
	for _, n := range nodes {
		pdn := resolvePlatformDisplayName(n.Platform, n.PlatformVersion, mappings)
		result = append(result, r.buildNodeResp(n, readinessByNodeName[n.NodeName], pdn))
	}

	WritePaginated(w, result, pg, total)
}

// handleNodesWithOwnerFilter is the fallback path for handleNodes when
// ownership filtering is active. It uses SQL push-down for node-level
// filters but applies ownership filtering in memory because the ownership
// assignments table is not directly joinable with node snapshots.
func (r *Router) handleNodesWithOwnerFilter(
	w http.ResponseWriter,
	req *http.Request,
	orgs []datastore.Organisation,
	orgNameByID map[string]string,
	of ownerFilter,
	pg PaginationParams,
) {
	ctx := req.Context()

	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "node")
	if err != nil {
		r.logf("ERROR", "resolving node ownership filter: %v", err)
		WriteInternalError(w, "Failed to resolve ownership filter.")
		return
	}

	// Use SQL push-down for node-level filters but without pagination
	// (we'll paginate after ownership filtering).
	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}
	f := nodeSnapshotFilterFromRequest(req, orgIDs, r.liveConfig().Collection.StaleNodeWarningHours, r.liveConfig().Collection.StaleNodeCriticalDays)
	// No limit/offset — we need all matching nodes for ownership filtering.

	allNodes, _, err2 := r.db.ListNodeSnapshotsFiltered(ctx, f)
	if err2 != nil {
		r.logf("ERROR", "listing filtered nodes for owner filter: %v", err2)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	// Apply ownership filter in memory.
	allNodes = filterByOwnershipKey(allNodes, ownedKeys, of, func(n datastore.NodeSnapshot) string { return n.NodeName })

	// Paginate the ownership-filtered results.
	pageNodes, total := PaginateSlice(allNodes, pg)

	// Bulk-load readiness data for the page (1 query per org instead of
	// 1 per node) — see bulkLoadReadiness.
	readinessByNodeName := bulkLoadReadiness(ctx, r.db, pageNodes, r)

	// Load platform display name mappings (non-fatal on error).
	mappings, mErr := r.loadPlatformDisplayNames(ctx)
	if mErr != nil {
		r.logf("WARN", "loading platform display names: %v", mErr)
	}

	result := make([]nodeResp, 0, len(pageNodes))
	for _, n := range pageNodes {
		pdn := resolvePlatformDisplayName(n.Platform, n.PlatformVersion, mappings)
		result = append(result, r.buildNodeResp(n, readinessByNodeName[n.NodeName], pdn))
	}

	WritePaginated(w, result, pg, total)
}

// nodeReadinessSummaryEntry is a compact readiness summary for the node list.
type nodeReadinessSummaryEntry struct {
	TargetChefVersion      string  `json:"target_chef_version"`
	Status                 string  `json:"status"` // node rollup: ready / needs_review / blocked
	IsReady                bool    `json:"is_ready"`
	AllCookbooksCompatible bool    `json:"all_cookbooks_compatible"`
	SufficientDiskSpace    *bool   `json:"sufficient_disk_space"`
	BlockingCookbookCount  int     `json:"blocking_cookbook_count"`
	ReviewCookbookCount    int     `json:"review_cookbook_count"`
	StaleData              bool    `json:"stale_data"`
	DiskStatus             string  `json:"disk_status"`
	CookstyleStatus        string  `json:"cookstyle_status"`
	KitchenStatus          string  `json:"kitchen_status"`
	DiskDetail             *string `json:"disk_detail"`
	CookstyleDetail        *string `json:"cookstyle_detail"`
	KitchenDetail          *string `json:"kitchen_detail"`
	InstallPath            string  `json:"install_path"`
}

// countBlockingCookbooks returns the number of blocking cookbooks from the
// JSONB column. It handles both the legacy string array and structured
// BlockingCookbook array formats.
func countBlockingCookbooks(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	// Try as array of any — just count the elements.
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr)
	}
	return 0
}

// handleNodeDetail handles GET /api/v1/nodes/:organisation/:name — returns
// a single node's detail including readiness information.
func (r *Router) handleNodeDetail(w http.ResponseWriter, req *http.Request) {
	// Routes like /api/v1/nodes/by-version/ and /api/v1/nodes/by-cookbook/
	// are registered with more specific prefixes and matched first by the
	// ServeMux. This handler only fires for other /api/v1/nodes/* paths.
	segs := pathSegments(req.URL.Path, "/api/v1/nodes/")
	if len(segs) < 2 {
		WriteNotFound(w, "Node detail requires /api/v1/nodes/:organisation/:name.")
		return
	}

	// Check for /nodes/:org/:name/dependency-graph
	if len(segs) >= 3 && segs[len(segs)-1] == "dependency-graph" {
		r.handleNodeDependencyGraph(w, req)
		return
	}

	if !requireGET(w, req) {
		return
	}

	orgName := segs[0]
	nodeName := strings.Join(segs[1:], "/") // node names may contain slashes

	// Resolve organisation by name.
	org, err := r.db.GetOrganisationByName(req.Context(), orgName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Organisation %q not found.", orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting organisation %s: %v", orgName, err)
		WriteInternalError(w, "Failed to get organisation.")
		return
	}

	// Get the most recent snapshot for this node.
	snapshot, err := r.db.GetNodeSnapshotByName(req.Context(), org.Name, nodeName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Node %q not found in organisation %q.", nodeName, orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting node snapshot %s/%s: %v", orgName, nodeName, err)
		WriteInternalError(w, "Failed to get node.")
		return
	}

	// Fetch readiness records by (organisation_id, node_name) rather than
	// by node_snapshot_id. The snapshot ID in node_readiness may be stale
	// if the readiness evaluator ran against an earlier snapshot that has
	// since been replaced by a newer collection run. Querying by name
	// matches the same path the dashboard uses and is resilient to
	// snapshot ID drift.
	readiness, err := r.db.ListNodeReadinessByNodeName(req.Context(), org.Name, nodeName)
	if err != nil {
		r.logf("WARN", "listing readiness for node %s/%s: %v", orgName, nodeName, err)
		// Non-fatal — we still return the snapshot.
	}

	// Load platform display name mappings (non-fatal on error).
	mappings, mErr := r.loadPlatformDisplayNames(req.Context())
	if mErr != nil {
		r.logf("WARN", "loading platform display names: %v", mErr)
	}
	pdn := resolvePlatformDisplayName(snapshot.Platform, snapshot.PlatformVersion, mappings)

	// Compute the install-path mount's total size at read time (cheap for one
	// node) so the detail page can draw the disk-usage bars. The verdict picks the
	// same mount as the stored sufficient/available verdict; we only need its
	// total. nil when the filesystem data is missing/unparseable.
	rc := r.liveConfig().Readiness
	diskVerdict := analysis.EvaluateDisk(snapshot.Filesystem, snapshot.Platform, analysis.DiskConfig{
		InstallPathLinux:        rc.InstallPathLinux,
		InstallPathWindows:      rc.InstallPathWindows,
		InstallSizeMBLinux:      rc.InstallSizeMBLinux,
		InstallSizeMBWindows:    rc.InstallSizeMBWindows,
		MinRemainingFreePercent: rc.MinRemainingFreePercent,
	})

	// Ownership, from the same helper the list uses. Failure to read it makes
	// the page less useful; refusing to render the page makes it useless.
	ownership := entityOwners{Owners: []string{}}
	if owners, oErr := r.ownersForEntities(req.Context(), "node", []string{snapshot.NodeName}); oErr != nil {
		r.logf("WARN", "looking up owners for node %s: %v", snapshot.NodeName, oErr)
	} else {
		ownership = owners[snapshot.NodeName]
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"node":                       snapshot,
		"ownership":                  ownership,
		"organisation_name":          org.Name,
		"readiness":                  readiness,
		"platform_display_name":      pdn,
		"install_path":               r.installPathForNode(snapshot.Platform),
		"min_remaining_free_percent": rc.MinRemainingFreePercent,
		"total_disk_mb":              diskVerdict.TotalMB,
	})
}

// handleNodesByVersion handles GET /api/v1/nodes/by-version/:chef_version —
// returns all nodes running the specified Chef client version.
//
// Uses SQL push-down with an exact chef_version match.
func (r *Router) handleNodesByVersion(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	chefVersion := pathParam(req, "/api/v1/nodes/by-version/")
	if chefVersion == "" {
		WriteBadRequest(w, "Chef version is required.")
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for nodes-by-version: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}

	// Use SQL push-down with exact chef_version match.
	f := datastore.NodeSnapshotFilter{
		OrganisationNames: orgIDs,
		ChefVersionExact:  chefVersion,
	}

	matched, _, err := r.db.ListNodeSnapshotsFiltered(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "listing nodes by version %s: %v", chefVersion, err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	// Apply owner filter if active.
	matched = applyOwnerFilter(req.Context(), r, matched, of)

	WriteJSON(w, http.StatusOK, map[string]any{
		"chef_version": chefVersion,
		"total":        len(matched),
		"data":         matched,
	})
}

// handleNodesByCookbook handles GET /api/v1/nodes/by-cookbook/:cookbook_name —
// returns all nodes that use the specified cookbook.
//
// This endpoint requires scanning the cookbooks JSONB field, so it uses
// SQL push-down for org filtering + IncludeHeavyJSON, then checks the
// cookbook name in memory.
func (r *Router) handleNodesByCookbook(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	cookbookName := pathParam(req, "/api/v1/nodes/by-cookbook/")
	if cookbookName == "" {
		WriteBadRequest(w, "Cookbook name is required.")
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for nodes-by-cookbook: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	orgIDs := make([]string, 0, len(orgs))
	orgNameByID := make(map[string]string, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
		orgNameByID[org.Name] = org.Name
	}

	// Use SQL push-down for org filtering. We need the cookbooks JSONB
	// for the in-memory cookbook check.
	f := datastore.NodeSnapshotFilter{
		OrganisationNames: orgIDs,
		IncludeHeavyJSON:  true,
	}

	allNodes, _, err := r.db.ListNodeSnapshotsFiltered(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "listing nodes for cookbook %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	type nodeWithOrg struct {
		OrganisationName string                 `json:"organisation_name"`
		Node             datastore.NodeSnapshot `json:"node"`
	}

	var matched []nodeWithOrg
	for _, n := range allNodes {
		if nodeUsesCookbook(n, cookbookName) {
			matched = append(matched, nodeWithOrg{
				OrganisationName: n.OrganisationName,
				Node:             n,
			})
		}
	}

	// Apply owner filter if active.
	{
		ownedKeys, oErr := r.resolveOwnershipFilter(req.Context(), of, "node")
		if oErr != nil {
			r.logf("ERROR", "resolving node ownership filter: %v", oErr)
			WriteInternalError(w, "Failed to resolve ownership filter.")
			return
		}
		matched = filterByOwnershipKey(matched, ownedKeys, of, func(n nodeWithOrg) string { return n.Node.NodeName })
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"cookbook_name": cookbookName,
		"total":         len(matched),
		"data":          matched,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nodeSnapshotFilterFromRequest constructs a NodeSnapshotFilter from the
// query-string parameters of an HTTP request. It maps the same parameters
// that were previously consumed by export.FilterNodes, so the semantics
// are identical (case-insensitive substring matching).
func nodeSnapshotFilterFromRequest(req *http.Request, orgIDs []string, warningHours, criticalDays int) datastore.NodeSnapshotFilter {
	return nodeSnapshotFilterFromValues(req.URL.Query(), orgIDs, warningHours, criticalDays)
}

// nodeSnapshotFilterFromValues builds the node list filter from raw query values.
// Both the list handler and the export path call this, so an export reproduces the
// list view's filtering exactly (see journeys/named-cohorts.md).
func nodeSnapshotFilterFromValues(q url.Values, orgIDs []string, warningHours, criticalDays int) datastore.NodeSnapshotFilter {
	f := datastore.NodeSnapshotFilter{
		OrganisationNames: orgIDs,
		NodeName:          q.Get("node_name"),
	}

	// Multi-value filters: comma-separated values use exact-match ANY($N).
	// Single values (no comma) fall through to the substring LIKE fields.
	if env := q.Get("environment"); env != "" {
		if strings.Contains(env, ",") {
			f.Environments = strings.Split(env, ",")
		} else {
			f.Environment = env
		}
	}
	if plat := q.Get("platform"); plat != "" {
		if strings.Contains(plat, ",") {
			f.Platforms = strings.Split(plat, ",")
		} else {
			f.Platform = plat
		}
	}
	if cv := q.Get("chef_version"); cv != "" {
		if strings.Contains(cv, ",") {
			f.ChefVersions = strings.Split(cv, ",")
		} else {
			f.ChefVersion = cv
		}
	}
	if pn := q.Get("policy_name"); pn != "" {
		if strings.Contains(pn, ",") {
			f.PolicyNames = strings.Split(pn, ",")
		} else {
			f.PolicyName = pn
		}
	}
	if pg := q.Get("policy_group"); pg != "" {
		if strings.Contains(pg, ",") {
			f.PolicyGroups = strings.Split(pg, ",")
		} else {
			f.PolicyGroup = pg
		}
	}
	if role := q.Get("role"); role != "" {
		if strings.Contains(role, ",") {
			f.Roles = strings.Split(role, ",")
		} else {
			f.Roles = []string{role}
		}
	}
	// Tags: repeatable (?tags=a&tags=b) and/or comma-separated (?tags=a,b),
	// OR semantics. No trimming/lowercasing — tags match Chef exactly.
	if tagVals := q["tags"]; len(tagVals) > 0 {
		var tags []string
		for _, tv := range tagVals {
			for _, t := range strings.Split(tv, ",") {
				if t != "" {
					tags = append(tags, t)
				}
			}
		}
		if len(tags) > 0 {
			f.Tags = tags
		}
	}

	// Sort parameters.
	f.Sort = q.Get("sort")
	f.SortOrder = q.Get("order")

	// Readiness filter push-down — requires target_chef_version.
	f.TargetChefVersion = q.Get("target_chef_version")
	f.ReadinessFilter = q.Get("readiness_filter")
	f.CookstyleStatusFilter = q.Get("cookstyle_status")
	f.KitchenStatusFilter = q.Get("kitchen_status")

	// Parallel deployment tracking filters.
	if ms := q.Get("migration_state"); ms != "" {
		f.MigrationStates = strings.Split(ms, ",")
	}
	if tcs := q.Get("target_converge_status"); tcs != "" {
		f.TargetConvergeStatuses = strings.Split(tcs, ",")
	}
	if tv := q.Get("target_version"); tv != "" {
		f.TargetVersions = strings.Split(tv, ",")
	}
	if rta := q.Get("ready_to_activate"); rta == "true" {
		v := true
		f.ReadyToActivate = &v
	}

	// Map the stale parameter — supports legacy bool, single tier, or comma-separated tiers.
	staleParam := q.Get("stale")
	switch staleParam {
	case "true":
		v := true
		f.Stale = &v
	case "false":
		v := false
		f.Stale = &v
	case "":
		// no filter
	default:
		valid := map[string]bool{"stale": true, "fresh": true, "warning": true, "critical": true}
		var tiers []string
		for _, t := range strings.Split(staleParam, ",") {
			t = strings.TrimSpace(t)
			if valid[t] {
				tiers = append(tiers, t)
			}
		}
		if len(tiers) > 0 {
			f.StaleTiers = tiers
			f.StaleWarningHours = warningHours
			f.StaleCriticalDays = criticalDays
		}
	}

	return f
}

// applyOwnerFilter applies ownership filtering to a slice of node snapshots
// when ownership is active. It resolves ownership keys and filters in memory.
// Returns the original slice unmodified when ownership is not active.
func applyOwnerFilter(ctx context.Context, r *Router, nodes []datastore.NodeSnapshot, of ownerFilter) []datastore.NodeSnapshot {
	ownedKeys, err := r.resolveOwnershipFilter(ctx, of, "node")
	if err != nil {
		r.logf("WARN", "resolving node ownership filter: %v", err)
		return nodes
	}
	return filterByOwnershipKey(nodes, ownedKeys, of, func(n datastore.NodeSnapshot) string { return n.NodeName })
}

// bulkLoadReadiness loads readiness data for a slice of node snapshots using
// one bulk query per organisation instead of one query per node (N+1). It
// returns a map keyed by node_name containing the summary entries ready to
// attach to nodeResp rows.
func bulkLoadReadiness(ctx context.Context, db DataStore, nodes []datastore.NodeSnapshot, r *Router) map[string][]nodeReadinessSummaryEntry {
	// Group node names by organisation ID.
	namesByOrg := make(map[string][]string)
	platformByNode := make(map[string]string, len(nodes))
	for _, n := range nodes {
		namesByOrg[n.OrganisationName] = append(namesByOrg[n.OrganisationName], n.NodeName)
		platformByNode[n.NodeName] = n.Platform
	}

	result := make(map[string][]nodeReadinessSummaryEntry, len(nodes))
	for orgID, names := range namesByOrg {
		bulk, err := db.BulkListNodeReadinessByNodeNames(ctx, orgID, names)
		if err != nil {
			r.logf("WARN", "bulk loading readiness for org %s: %v", orgID, err)
			continue // non-fatal — readiness just won't be shown
		}
		for nodeName, recs := range bulk {
			for _, rec := range recs {
				installPath := r.installPathForNode(platformByNode[nodeName])
				cs := deriveCheckStatus(rec, installPath)
				result[nodeName] = append(result[nodeName], nodeReadinessSummaryEntry{
					TargetChefVersion:      rec.TargetChefVersion,
					Status:                 rec.Status,
					IsReady:                rec.IsReady,
					AllCookbooksCompatible: rec.AllCookbooksCompatible,
					SufficientDiskSpace:    rec.SufficientDiskSpace,
					BlockingCookbookCount:  countBlockingCookbooks(rec.BlockingCookbooks),
					ReviewCookbookCount:    countBlockingCookbooks(rec.ReviewCookbooks),
					StaleData:              rec.StaleData,
					DiskStatus:             cs.DiskStatus,
					CookstyleStatus:        cs.CookstyleStatus,
					KitchenStatus:          cs.KitchenStatus,
					DiskDetail:             cs.DiskDetail,
					CookstyleDetail:        cs.CookstyleDetail,
					KitchenDetail:          cs.KitchenDetail,
					InstallPath:            installPath,
				})
			}
		}
	}
	return result
}

// nodeUsesCookbook checks whether a node snapshot's Cookbooks JSON contains
// the given cookbook name. The Cookbooks field is a JSON object mapping
// cookbook names to version info, e.g. {"apt": {"version": "7.4.0"}, ...}.
func nodeUsesCookbook(n datastore.NodeSnapshot, cookbookName string) bool {
	if len(n.Cookbooks) == 0 {
		return false
	}
	// Parse the JSONB and check for an exact top-level key match.
	// The old substring approach could false-positive when a cookbook name
	// appeared inside a value or when names were prefixes of other names
	// in certain JSON layouts.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(n.Cookbooks, &m); err != nil {
		return false
	}
	_, ok := m[cookbookName]
	return ok
}

// handleNodeDependencyGraph handles GET /api/v1/nodes/:org/:name/dependency-graph
// Returns the node's dependency tree (run_list → roles → cookbooks) with
// per-cookbook CookStyle and TK status.
func (r *Router) handleNodeDependencyGraph(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	segs := pathSegments(req.URL.Path, "/api/v1/nodes/")
	if len(segs) < 3 {
		WriteNotFound(w, "Dependency graph requires /api/v1/nodes/:org/:name/dependency-graph.")
		return
	}

	orgName := segs[0]
	// Node name is everything between org and "dependency-graph".
	nodeName := strings.Join(segs[1:len(segs)-1], "/")

	ctx := req.Context()

	targetChefVersion := queryString(req, "target_chef_version", "")
	if targetChefVersion == "" {
		targetChefVersion = r.defaultTargetVersion()
	}

	// Resolve organisation.
	org, err := r.db.GetOrganisationByName(ctx, orgName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Organisation %q not found.", orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting organisation for node dep graph %s: %v", orgName, err)
		WriteInternalError(w, "Failed to get organisation.")
		return
	}

	// Get the node snapshot (for run_list).
	snapshot, err := r.db.GetNodeSnapshotByName(ctx, org.Name, nodeName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Node %q not found in organisation %q.", nodeName, orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting node snapshot for dep graph %s/%s: %v", orgName, nodeName, err)
		WriteInternalError(w, "Failed to get node.")
		return
	}

	// Get readiness data for the target version.
	readinessRecords, err := r.db.ListNodeReadinessByNodeName(ctx, org.Name, nodeName)
	if err != nil {
		r.logf("WARN", "listing readiness for node dep graph %s/%s: %v", orgName, nodeName, err)
	}

	// Find the readiness record matching the target version.
	var readiness *datastore.NodeReadiness
	for i := range readinessRecords {
		if readinessRecords[i].TargetChefVersion == targetChefVersion {
			readiness = &readinessRecords[i]
			break
		}
	}

	// Parse the node's run_list.
	var runList []string
	if len(snapshot.RunList) > 0 {
		_ = json.Unmarshal(snapshot.RunList, &runList)
	}

	// Get all role dependencies for the org.
	deps, err := r.db.ListRoleDependenciesByOrg(ctx, org.Name)
	if err != nil {
		r.logf("ERROR", "listing role deps for node dep graph %s/%s: %v", orgName, nodeName, err)
		WriteInternalError(w, "Failed to load dependency data.")
		return
	}

	// Build adjacency map (role → dependencies).
	adj := make(map[string][]datastore.RoleDependency)
	for _, d := range deps {
		adj[d.RoleName] = append(adj[d.RoleName], d)
	}

	// Build blocking cookbook lookup from readiness data.
	blockingMap := make(map[string]blockingEntry)
	if readiness != nil {
		blocking := parseBlockingCookbooks(readiness.BlockingCookbooks)
		for _, b := range blocking {
			blockingMap[b.Name] = b
		}
	}

	// Graph node/edge types.
	type graphNode struct {
		ID                  string `json:"id"`
		Type                string `json:"type"`
		Name                string `json:"name"`
		Version             string `json:"version,omitempty"`
		CompatibilityStatus string `json:"compatibility_status,omitempty"`
		TKStatus            string `json:"tk_status,omitempty"`
		ComplexityLabel     string `json:"complexity_label,omitempty"`
		ComplexityScore     int    `json:"complexity_score,omitempty"`
		Source              string `json:"source,omitempty"`
	}
	type graphEdge struct {
		From string `json:"from"`
		To   string `json:"to"`
		Type string `json:"type"`
	}

	nodeMap := make(map[string]graphNode)
	var edges []graphEdge
	visited := make(map[string]bool)

	// Walk a role recursively, expanding its dependencies.
	var walkRole func(role, parentID string)
	walkRole = func(role, parentID string) {
		roleID := "role:" + role
		if _, ok := nodeMap[roleID]; !ok {
			nodeMap[roleID] = graphNode{ID: roleID, Type: "role", Name: role}
		}
		edges = append(edges, graphEdge{From: parentID, To: roleID, Type: "includes_role"})

		if visited[role] {
			return
		}
		visited[role] = true

		for _, d := range adj[role] {
			targetID := d.DependencyType + ":" + d.DependencyName
			edgeType := "includes_" + d.DependencyType

			if _, ok := nodeMap[targetID]; !ok {
				nodeMap[targetID] = graphNode{
					ID:   targetID,
					Type: d.DependencyType,
					Name: d.DependencyName,
				}
			}

			edges = append(edges, graphEdge{From: roleID, To: targetID, Type: edgeType})

			if d.DependencyType == "role" {
				if !visited[d.DependencyName] {
					walkRole(d.DependencyName, roleID)
				}
			}
		}
	}

	// Process run_list entries.
	for _, entry := range runList {
		entryID := "run_list_entry:" + entry
		nodeMap[entryID] = graphNode{ID: entryID, Type: "run_list_entry", Name: entry}

		// Parse run_list entry format: "role[name]" or "recipe[cookbook::recipe]"
		if strings.HasPrefix(entry, "role[") && strings.HasSuffix(entry, "]") {
			roleName := entry[5 : len(entry)-1]
			walkRole(roleName, entryID)
		} else if strings.HasPrefix(entry, "recipe[") && strings.HasSuffix(entry, "]") {
			recipeName := entry[7 : len(entry)-1]
			// Extract cookbook name from recipe (cookbook::recipe or just cookbook)
			cbName := recipeName
			if idx := strings.Index(recipeName, "::"); idx >= 0 {
				cbName = recipeName[:idx]
			}
			cbID := "cookbook:" + cbName
			if _, ok := nodeMap[cbID]; !ok {
				nodeMap[cbID] = graphNode{ID: cbID, Type: "cookbook", Name: cbName}
			}
			edges = append(edges, graphEdge{From: entryID, To: cbID, Type: "includes_cookbook"})
		}
	}

	// Annotate cookbook nodes with compatibility and TK status.
	allCompatible := readiness != nil && readiness.AllCookbooksCompatible
	var nonBlockingCookbooks []string
	for id, n := range nodeMap {
		if n.Type != "cookbook" {
			continue
		}
		if bc, blocked := blockingMap[n.Name]; blocked {
			n.Version = bc.Version
			n.ComplexityLabel = bc.ComplexityLabel
			n.ComplexityScore = bc.ComplexityScore
			// Determine status from verdicts.
			csStatus := "compatible"
			tkStatus := "untested"
			for _, v := range bc.Verdicts {
				if isCookstyleSource(v.Source) && v.Status == "incompatible" {
					csStatus = "incompatible"
				}
				if v.Source == "git_test_kitchen" {
					if v.Status == "incompatible" {
						tkStatus = "failed"
					} else if v.Status == "compatible" {
						tkStatus = "passed"
					}
				}
			}
			n.CompatibilityStatus = csStatus
			n.TKStatus = tkStatus
		} else if allCompatible {
			n.CompatibilityStatus = "compatible"
			n.TKStatus = "passed"
			nonBlockingCookbooks = append(nonBlockingCookbooks, n.Name)
		} else if readiness != nil {
			// Not blocking → CookStyle compatible; TK unknown.
			n.CompatibilityStatus = "compatible"
			n.TKStatus = "untested"
			nonBlockingCookbooks = append(nonBlockingCookbooks, n.Name)
		} else {
			n.CompatibilityStatus = "untested"
			n.TKStatus = "untested"
			nonBlockingCookbooks = append(nonBlockingCookbooks, n.Name)
		}
		nodeMap[id] = n
	}

	// Fill in complexity scores for non-blocking cookbooks from the DB.
	if len(nonBlockingCookbooks) > 0 && targetChefVersion != "" {
		complexityMap, _ := r.db.GetCookbookComplexityMap(ctx, orgName, targetChefVersion, nonBlockingCookbooks)
		for id, n := range nodeMap {
			if n.Type == "cookbook" && n.ComplexityScore == 0 {
				if score, ok := complexityMap[n.Name]; ok {
					n.ComplexityScore = score
					nodeMap[id] = n
				}
			}
		}
	}

	// Convert to sorted slices.
	nodes := make([]graphNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type < nodes[j].Type
		}
		return nodes[i].Name < nodes[j].Name
	})

	if edges == nil {
		edges = []graphEdge{}
	}

	// Compute metadata.
	roleCount := 0
	cookbookCount := 0
	incompatibleCount := 0
	tkFailedCount := 0
	for _, n := range nodes {
		switch n.Type {
		case "role":
			roleCount++
		case "cookbook":
			cookbookCount++
			if n.CompatibilityStatus == "incompatible" {
				incompatibleCount++
			}
			if n.TKStatus == "failed" {
				tkFailedCount++
			}
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"nodes": nodes,
		"edges": edges,
		"metadata": map[string]any{
			"total_roles":            roleCount,
			"total_cookbooks":        cookbookCount,
			"incompatible_cookbooks": incompatibleCount,
			"tk_failed_cookbooks":    tkFailedCount,
		},
	})
}
