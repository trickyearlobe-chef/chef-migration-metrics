// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/platform"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/syshealth"
)

// handleDiagnosticBundle handles GET /api/v1/admin/diagnostic-bundle.
// It streams a ZIP archive containing diagnostic data from multiple sources.
func (r *Router) handleDiagnosticBundle(w http.ResponseWriter, req *http.Request) {
	if r.authMiddleware == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Diagnostic bundle requires authentication to be configured.")
		return
	}
	if !requireGET(w, req) {
		return
	}

	params := parseBundleParams(req)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="cmm-diagnostic-%s.zip"`,
		time.Now().UTC().Format("20060102-150405")))
	w.Header().Set("Cache-Control", "no-store")

	zw := zip.NewWriter(w)
	errs := make(map[string]string)

	// --- bundle_info.json ---
	host := hostname()
	if err := writeZipJSON(zw, "bundle_info.json", map[string]any{
		"timestamp":            time.Now().UTC(),
		"app_version":          r.version,
		"hostname":             host,
		"go_version":           runtime.Version(),
		"include_identifiers":  params.includeIdentifiers,
		"include_depth_stats":  params.includeDepthStats,
		"log_days":             params.logDays,
	}); err != nil {
		errs["bundle_info"] = err.Error()
	}

	// --- config_summary.json ---
	cfg := r.liveConfig()
	if err := writeZipJSON(zw, "config_summary.json", DiagnosticConfigSummary(*cfg)); err != nil {
		errs["config_summary"] = err.Error()
	}

	// --- performance.json ---
	if r.recorder != nil {
		snap := r.recorder.Snapshot()
		endpoints := make([]endpointStat, 0, len(snap))
		for _, ks := range snap {
			method, path := splitEndpointKey(ks.Key)
			endpoints = append(endpoints, endpointStat{
				Method:     method,
				Path:       path,
				Count:      ks.Count,
				ErrorCount: ks.ErrorCount,
				P50Ms:      durationMs(ks.P50),
				P95Ms:      durationMs(ks.P95),
				P99Ms:      durationMs(ks.P99),
				MaxMs:      durationMs(ks.Max),
			})
		}
		perfData := performanceResponse{
			WindowSeconds: cfg.Performance.WindowSeconds,
			Endpoints:     endpoints,
		}
		if err := writeZipJSON(zw, "performance.json", perfData); err != nil {
			errs["performance"] = err.Error()
		}
	}

	// --- system_health.json ---
	{
		sh := cfg.SystemHealth
		th := syshealth.Thresholds{
			DiskUsedWarningPercent:  sh.DiskUsedWarningPercent,
			DiskUsedCriticalPercent: sh.DiskUsedCriticalPercent,
			CPULoadWarningPerCPU:    sh.CPULoadWarningPerCPU,
			CPULoadCriticalPerCPU:   sh.CPULoadCriticalPerCPU,
			MemUsedWarningPercent:   sh.MemUsedWarningPercent,
			MemUsedCriticalPercent:  sh.MemUsedCriticalPercent,
		}
		stats := syshealth.Snapshot(sh.DiskPaths, th)

		var dbSizeBytes int64
		var tableSizes []datastore.TableSize
		if r.db != nil {
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()
			size, err := r.db.DatabaseSize(ctx)
			if err == nil {
				dbSizeBytes = size
			}
			ts, err := r.db.DatabaseTableSizes(ctx)
			if err == nil {
				tableSizes = ts
			}
		}
		if tableSizes == nil {
			tableSizes = []datastore.TableSize{}
		}
		alerts := stats.Alerts
		if alerts == nil {
			alerts = []syshealth.Alert{}
		}
		disks := stats.Disks
		if disks == nil {
			disks = []syshealth.DiskStats{}
		}
		shData := map[string]any{
			"timestamp":           stats.Timestamp,
			"uptime":              stats.Uptime,
			"disks":               disks,
			"cpu_count":           stats.CPUCount,
			"load_avg_1":          stats.LoadAvg1,
			"load_per_cpu":        stats.LoadPerCPU,
			"mem_total_bytes":     stats.MemTotalBytes,
			"mem_avail_bytes":     stats.MemAvailBytes,
			"mem_used_percent":    stats.MemUsedPercent,
			"go_heap_bytes":       stats.GoHeapBytes,
			"go_goroutines":       stats.GoGoroutines,
			"database_size_bytes": dbSizeBytes,
			"table_sizes":         tableSizes,
			"alerts":              alerts,
		}
		if err := writeZipJSON(zw, "system_health.json", shData); err != nil {
			errs["system_health"] = err.Error()
		}
	}

	// Build org key map once, reused across all sources that anonymise org names.
	orgKeyMap := r.buildDiagnosticOrgKeyMap(req.Context())

	// --- migrations.json ---
	{
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		migrations, err := r.db.ListAppliedMigrations(ctx)
		cancel()
		if err != nil {
			errs["migrations"] = err.Error()
		} else {
			if migrations == nil {
				migrations = []datastore.AppliedMigration{}
			}
			if err := writeZipJSON(zw, "migrations.json", migrations); err != nil {
				errs["migrations"] = err.Error()
			}
		}
	}

	// --- organisations.json ---
	{
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		orgs, err := r.db.ListOrganisations(ctx)
		cancel()
		if err != nil {
			errs["organisations"] = err.Error()
		} else {
			var orgData any
			if params.includeIdentifiers {
				type orgSummary struct {
					Name string `json:"name"`
				}
				summaries := make([]orgSummary, len(orgs))
				for i, o := range orgs {
					summaries[i] = orgSummary{Name: o.Name}
				}
				orgData = summaries
			} else {
				orgData = map[string]any{"count": len(orgs)}
			}
			if err := writeZipJSON(zw, "organisations.json", orgData); err != nil {
				errs["organisations"] = err.Error()
			}
		}
	}

	// --- collection_run_status.json ---
	{
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		runs, err := r.db.ListCollectionRunsFiltered(ctx, datastore.CollectionRunFilter{})
		cancel()
		if err != nil {
			errs["collection_run_status"] = err.Error()
		} else {
			var runData any
			if params.includeIdentifiers {
				runData = runs
			} else {
				runData = anonymiseCollectionRuns(runs, orgKeyMap)
			}
			if err := writeZipJSON(zw, "collection_run_status.json", runData); err != nil {
				errs["collection_run_status"] = err.Error()
			}
		}
	}

	// --- performance_db.json ---
	{
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		available := r.db.PgStatStatementsAvailable(ctx)
		topQueries, qErr := r.db.TopQueryStats(ctx, 20)
		tableStats, tErr := r.db.TableStats(ctx)
		indexStats, iErr := r.db.IndexStats(ctx)
		cancel()

		if qErr != nil {
			errs["performance_db_top_queries"] = qErr.Error()
		} else if tErr != nil {
			errs["performance_db_table_stats"] = tErr.Error()
		} else if iErr != nil {
			errs["performance_db_index_stats"] = iErr.Error()
		} else {
			perfDB := map[string]any{
				"pg_stat_statements_available": available,
				"top_queries":                  emptySliceIfNil(topQueries),
				"table_stats":                  emptySliceIfNil(tableStats),
				"index_stats":                  emptySliceIfNil(indexStats),
			}
			if err := writeZipJSON(zw, "performance_db.json", perfDB); err != nil {
				errs["performance_db"] = err.Error()
			}
		}
	}

	// --- inventory_stats.json ---
	{
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		inv, err := r.db.InventoryStats(ctx, params.includeIdentifiers)
		cancel()
		if err != nil {
			errs["inventory_stats"] = err.Error()
		} else {
			var invData any
			if params.includeIdentifiers {
				invData = inv
			} else {
				invData = anonymiseInventoryStats(inv, orgKeyMap)
			}
			if err := writeZipJSON(zw, "inventory_stats.json", invData); err != nil {
				errs["inventory_stats"] = err.Error()
			}
		}
	}

	// --- logs_recent.json ---
	{
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		entries, err := r.db.ListLogEntries(ctx, datastore.LogEntryFilter{
			Since: time.Now().UTC().Add(-time.Duration(params.logDays) * 24 * time.Hour),
			Limit: 5000,
		})
		cancel()
		if err != nil {
			errs["logs_recent"] = err.Error()
		} else {
			sanitized := sanitizeLogEntries(entries, orgKeyMap, params.includeIdentifiers)
			if err := writeZipJSON(zw, "logs_recent.json", sanitized); err != nil {
				errs["logs_recent"] = err.Error()
			}
		}
	}

	// --- logs_errors.json ---
	{
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		entries, err := r.db.ListLogEntries(ctx, datastore.LogEntryFilter{
			MinSeverity: "ERROR",
			Since:       time.Now().UTC().Add(-time.Duration(params.logDays) * 24 * time.Hour),
			Limit:       1000,
		})
		cancel()
		if err != nil {
			errs["logs_errors"] = err.Error()
		} else {
			sanitized := sanitizeLogEntries(entries, orgKeyMap, params.includeIdentifiers)
			if err := writeZipJSON(zw, "logs_errors.json", sanitized); err != nil {
				errs["logs_errors"] = err.Error()
			}
		}
	}

	// --- platform_distribution.json ---
	{
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		distRows, totalNodes, err := r.db.CountNodePlatformDistributionDetailed(ctx, datastore.NodeSnapshotFilter{})
		cancel()
		if err != nil {
			errs["platform_distribution"] = err.Error()
		} else {
			pd := buildBundlePlatformDistributionDetailed(distRows, totalNodes)
			if err := writeZipJSON(zw, "platform_distribution.json", pd); err != nil {
				errs["platform_distribution"] = err.Error()
			}
		}
	}

	// --- dependency_depth_stats.json (optional) ---
	if params.includeDepthStats {
		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
		depth, err := r.db.DependencyDepthStats(ctx, params.includeIdentifiers)
		cancel()
		if err != nil {
			errs["dependency_depth_stats"] = err.Error()
		} else {
			if err := writeZipJSON(zw, "dependency_depth_stats.json", depth); err != nil {
				errs["dependency_depth_stats"] = err.Error()
			}
		}
	}

	// --- errors.json (always written) ---
	if err := writeZipJSON(zw, "errors.json", errs); err != nil {
		// Nothing more we can do here.
		_ = err
	}

	if err := zw.Close(); err != nil {
		// Close flushes the remaining buffered zip data; a failure here means
		// the bundle the client received is truncated/corrupt. Headers are
		// already sent, so log it rather than attempting an HTTP error.
		r.logf("ERROR", "admin/diagnostic: finalise bundle: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Bundle parameter parsing
// ---------------------------------------------------------------------------

type bundleParams struct {
	logDays            int
	includeIdentifiers bool
	includeDepthStats  bool
}

func parseBundleParams(req *http.Request) bundleParams {
	p := bundleParams{logDays: 7}

	if v := req.URL.Query().Get("log_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.logDays = n
		}
	}
	if p.logDays < 1 {
		p.logDays = 1
	}
	if p.logDays > 30 {
		p.logDays = 30
	}

	if v := req.URL.Query().Get("include_identifiers"); v != "" {
		p.includeIdentifiers, _ = strconv.ParseBool(v)
	}
	if v := req.URL.Query().Get("include_depth_stats"); v != "" {
		p.includeDepthStats, _ = strconv.ParseBool(v)
	}
	return p
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeZipJSON writes v as JSON into a new file named name inside the ZIP.
func writeZipJSON(zw *zip.Writer, name string, data any) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	return json.NewEncoder(f).Encode(data)
}

// hostname returns the machine hostname, or an empty string on error.
func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

// buildDiagnosticOrgKeyMap fetches the org list (best-effort) and builds a
// stable anonymous key mapping for it.
func (r *Router) buildDiagnosticOrgKeyMap(ctx context.Context) map[string]string {
	if r.db == nil {
		return map[string]string{}
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	orgs, err := r.db.ListOrganisations(fetchCtx)
	if err != nil || len(orgs) == 0 {
		return map[string]string{}
	}
	names := make([]string, len(orgs))
	for i, o := range orgs {
		names[i] = o.Name
	}
	return buildOrgKeyMap(names)
}

// buildOrgKeyMap returns a stable mapping from real org name to "org-N" key.
// It sorts org names alphabetically for stability across requests.
func buildOrgKeyMap(names []string) map[string]string {
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)
	m := make(map[string]string, len(sorted))
	for i, name := range sorted {
		m[name] = fmt.Sprintf("org-%d", i+1)
	}
	return m
}

// anonymiseCollectionRuns replaces OrganisationName with opaque keys.
func anonymiseCollectionRuns(runs []datastore.CollectionRunWithOrg, orgKeyMap map[string]string) []map[string]any {
	result := make([]map[string]any, len(runs))
	for i, run := range runs {
		key, ok := orgKeyMap[run.OrganisationName]
		if !ok {
			key = "org-?"
		}
		result[i] = map[string]any{
			"organisation_name": key,
			"run":               run.Run,
		}
	}
	return result
}

// anonymiseInventoryStats replaces map keys (org names) with opaque keys.
func anonymiseInventoryStats(inv datastore.InventoryStatsResult, orgKeyMap map[string]string) map[string]any {
	return map[string]any{
		"nodes_by_org":          anonymiseIntMap(inv.NodesByOrg, orgKeyMap),
		"cookbooks_by_org":      anonymiseIntMap(inv.CookbooksByOrg, orgKeyMap),
		"roles_by_org":          anonymiseIntMap(inv.RolesByOrg, orgKeyMap),
		"role_dep_edges_by_org": anonymiseIntMap(inv.RoleDepEdgesByOrg, orgKeyMap),
		"git_repo_count":        inv.GitRepoCount,
	}
}

func anonymiseIntMap(m map[string]int, orgKeyMap map[string]string) map[string]int {
	result := make(map[string]int, len(m))
	for org, v := range m {
		key, ok := orgKeyMap[org]
		if !ok {
			key = "org-?"
		}
		result[key] = v
	}
	return result
}

// sanitizeLogEntries converts a slice of LogEntry values to maps with
// ProcessOutput removed. Org names are anonymised when includeIdentifiers is false.
func sanitizeLogEntries(entries []datastore.LogEntry, orgKeyMap map[string]string, includeIdentifiers bool) []map[string]any {
	result := make([]map[string]any, len(entries))
	for i, le := range entries {
		result[i] = sanitizeLogEntry(le, orgKeyMap, includeIdentifiers)
	}
	return result
}

// sanitizeLogEntry converts a LogEntry to a map, omitting ProcessOutput and
// optionally anonymising org references.
func sanitizeLogEntry(le datastore.LogEntry, orgKeyMap map[string]string, includeIdentifiers bool) map[string]any {
	m := map[string]any{
		"id":        le.ID,
		"timestamp": le.Timestamp,
		"severity":  le.Severity,
		"scope":     le.Scope,
		"message":   le.Message,
	}
	if le.CookbookName != "" {
		m["cookbook_name"] = le.CookbookName
	}
	if le.CookbookVersion != "" {
		m["cookbook_version"] = le.CookbookVersion
	}
	if le.CommitSHA != "" {
		m["commit_sha"] = le.CommitSHA
	}
	if le.ChefClientVersion != "" {
		m["chef_client_version"] = le.ChefClientVersion
	}
	if le.NotificationChannel != "" {
		m["notification_channel"] = le.NotificationChannel
	}
	if le.ExportJobID != "" {
		m["export_job_id"] = le.ExportJobID
	}
	if le.TLSDomain != "" {
		m["tls_domain"] = le.TLSDomain
	}

	org := le.Organisation
	colOrg := le.CollectionRunOrg
	if !includeIdentifiers {
		if org != "" {
			if k, ok := orgKeyMap[org]; ok {
				org = k
			} else {
				org = "org-?"
			}
		}
		if colOrg != "" {
			if k, ok := orgKeyMap[colOrg]; ok {
				colOrg = k
			} else {
				colOrg = "org-?"
			}
		}
	}
	if org != "" {
		m["organisation"] = org
	}
	if colOrg != "" {
		m["collection_run_org"] = colOrg
	}

	return m
}

// ---------------------------------------------------------------------------
// Platform distribution for diagnostic bundle
// ---------------------------------------------------------------------------

type bundlePlatformEntry struct {
	Platform         string  `json:"platform"`
	Version          string  `json:"version"`
	DisplayName      string  `json:"display_name"`
	GroupKey         string  `json:"group_key"`
	GroupDisplayName string  `json:"group_display_name"`
	Count            int     `json:"count"`
	Percent          float64 `json:"percent"`
}

type bundlePlatformDistribution struct {
	TotalNodes   int                   `json:"total_nodes"`
	Distribution []bundlePlatformEntry `json:"distribution"`
}

// buildBundlePlatformDistribution resolves display names and groups using
// built-in default mappings only (not admin-configured mappings) to avoid
// leaking customer-specific identifiers in the always-on bundle file.
func buildBundlePlatformDistribution(dist map[string]int, totalNodes int) bundlePlatformDistribution {
	entries := make([]bundlePlatformEntry, 0, len(dist))

	for label, count := range dist {
		plat, ver := splitPlatformLabel(label)
		family := platform.DetectOSFamilyFromPlatform(plat)
		info := platform.ResolveInfo(plat, ver, family, "", platform.DefaultMappings)

		pct := float64(0)
		if totalNodes > 0 {
			pct = float64(count) / float64(totalNodes) * 100
		}

		entries = append(entries, bundlePlatformEntry{
			Platform:         plat,
			Version:          ver,
			DisplayName:      info.DisplayName,
			GroupKey:         info.GroupKey,
			GroupDisplayName: info.GroupDisplayName,
			Count:            count,
			Percent:          pct,
		})
	}

	// Sort by count descending for readability.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].DisplayName < entries[j].DisplayName
	})

	return bundlePlatformDistribution{
		TotalNodes:   totalNodes,
		Distribution: entries,
	}
}

// buildBundlePlatformDistributionDetailed resolves display names using the
// detailed rows that include caption for accurate resolution.
func buildBundlePlatformDistributionDetailed(rows []datastore.PlatformDistributionRow, totalNodes int) bundlePlatformDistribution {
	entries := make([]bundlePlatformEntry, 0, len(rows))

	for _, row := range rows {
		family := row.PlatformFamily
		if family == "" {
			family = platform.DetectOSFamilyFromPlatform(row.Platform)
		}
		info := platform.ResolveInfo(row.Platform, row.PlatformVersion, family, row.PlatformCaption, platform.DefaultMappings)

		pct := float64(0)
		if totalNodes > 0 {
			pct = float64(row.Count) / float64(totalNodes) * 100
		}

		entries = append(entries, bundlePlatformEntry{
			Platform:         row.Platform,
			Version:          row.PlatformVersion,
			DisplayName:      info.DisplayName,
			GroupKey:         info.GroupKey,
			GroupDisplayName: info.GroupDisplayName,
			Count:            row.Count,
			Percent:          pct,
		})
	}

	// Sort by count descending for readability.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].DisplayName < entries[j].DisplayName
	})

	return bundlePlatformDistribution{
		TotalNodes:   totalNodes,
		Distribution: entries,
	}
}

// splitPlatformLabel splits "platform version" back into components.
func splitPlatformLabel(label string) (plat, ver string) {
	idx := strings.IndexByte(label, ' ')
	if idx > 0 {
		return label[:idx], label[idx+1:]
	}
	return label, ""
}
