// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// cookstyleViolationItem is the per-row response shape for the violations
// browser endpoint. It includes pre-computed summary counts derived from the
// offences JSONB.
type cookstyleViolationItem struct {
	Source            string         `json:"source"`
	Name              string         `json:"name"`
	Version           string         `json:"version"`
	Organisation      string         `json:"organisation,omitempty"`
	TargetChefVersion string         `json:"target_chef_version"`
	Passed            bool           `json:"passed"`
	OffenceCount      int            `json:"offence_count"`
	DeprecationCount  int            `json:"deprecation_count"`
	CorrectnessCount  int            `json:"correctness_count"`
	ErrorMessage      string         `json:"error_message,omitempty"`
	ScannedAt         time.Time      `json:"scanned_at"`
	NamespaceCounts   map[string]int `json:"namespace_counts"`
	SeverityCounts    map[string]int `json:"severity_counts"`
	TopCops           []string       `json:"top_cops"`

	// allCops holds every unique cop name found in this item's offenses.
	// Used for cop name filtering; excluded from JSON output.
	allCops []string `json:"-"`
}

// handleCookstyleViolations handles GET /api/v1/cookstyle/violations.
// Returns a paginated list of cookstyle results with derived offense
// summaries.
func (r *Router) handleCookstyleViolations(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Resolve target Chef version — default to the highest configured.
	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" {
		targetVersion = r.defaultTargetVersion()
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	source := queryString(req, "source", "server")
	status := queryString(req, "status", "")
	namespace := queryString(req, "namespace", "")
	severity := queryString(req, "severity", "")
	cop := queryString(req, "cop", "")

	pg := ParsePagination(req)
	if pg.PerPage > 200 {
		pg.PerPage = 200
	}
	sp := ParseSort(req, "name", []string{"name", "offence_count", "deprecation_count"})

	var items []cookstyleViolationItem

	switch source {
	case "git":
		results, err := r.db.ListGitRepoCookstyleResultsByTargetVersion(ctx, targetVersion)
		if err != nil {
			r.logf("ERROR", "listing git repo cookstyle violations: %v", err)
			WriteInternalError(w, "Failed to list cookstyle violations.")
			return
		}
		items = buildGitRepoViolationItems(results)
	default:
		results, err := r.db.ListAllServerCookbookCookstyleResultsByTargetVersion(ctx, targetVersion)
		if err != nil {
			r.logf("ERROR", "listing server cookbook cookstyle violations: %v", err)
			WriteInternalError(w, "Failed to list cookstyle violations.")
			return
		}
		items = buildServerViolationItems(results)
	}

	// Apply filters.
	items = filterViolations(items, status, namespace, severity, cop)

	// Sort.
	sortViolations(items, sp)

	// Paginate.
	page, total := PaginateSlice(items, pg)

	WritePaginated(w, page, pg, total)
}

// buildServerViolationItems converts server cookbook cookstyle results into
// violation items with derived offense summaries.
func buildServerViolationItems(results []datastore.ServerCookbookCookstyleResult) []cookstyleViolationItem {
	items := make([]cookstyleViolationItem, 0, len(results))
	for _, r := range results {
		nsCounts, sevCounts, topCops, cops := deriveOffenseSummary(r.Offences)
		items = append(items, cookstyleViolationItem{
			Source:            "server",
			Name:              r.CookbookName,
			Version:           r.CookbookVersion,
			Organisation:      r.OrganisationName,
			TargetChefVersion: r.TargetChefVersion,
			Passed:            r.Passed,
			OffenceCount:      r.OffenceCount,
			DeprecationCount:  r.DeprecationCount,
			CorrectnessCount:  r.CorrectnessCount,
			ErrorMessage:      r.ErrorMessage,
			ScannedAt:         r.ScannedAt,
			NamespaceCounts:   nsCounts,
			SeverityCounts:    sevCounts,
			TopCops:           topCops,
			allCops:           cops,
		})
	}
	return items
}

// buildGitRepoViolationItems converts git repo cookstyle results into
// violation items.
func buildGitRepoViolationItems(results []datastore.GitRepoCookstyleResult) []cookstyleViolationItem {
	items := make([]cookstyleViolationItem, 0, len(results))
	for _, r := range results {
		nsCounts, sevCounts, topCops, cops := deriveOffenseSummary(r.Offences)
		items = append(items, cookstyleViolationItem{
			Source:            "git",
			Name:              r.GitRepoName,
			Version:           r.CommitSHA,
			TargetChefVersion: r.TargetChefVersion,
			Passed:            r.Passed,
			OffenceCount:      r.OffenceCount,
			DeprecationCount:  r.DeprecationCount,
			CorrectnessCount:  r.CorrectnessCount,
			ErrorMessage:      r.ErrorMessage,
			ScannedAt:         r.ScannedAt,
			NamespaceCounts:   nsCounts,
			SeverityCounts:    sevCounts,
			TopCops:           topCops,
			allCops:           cops,
		})
	}
	return items
}

// deriveOffenseSummary parses the offences JSONB and computes namespace
// counts, severity counts, top cops (by frequency), and all unique cop names.
func deriveOffenseSummary(offencesJSON []byte) (namespaceCounts map[string]int, severityCounts map[string]int, topCops []string, allCops []string) {
	namespaceCounts = make(map[string]int)
	severityCounts = make(map[string]int)
	copCounts := make(map[string]int)

	if len(offencesJSON) == 0 {
		return namespaceCounts, severityCounts, nil, nil
	}

	offenses := parseOffensesFlat(offencesJSON)
	for _, o := range offenses {
		if o.CopName != "" {
			ns := copNamespace(o.CopName)
			namespaceCounts[ns]++
			copCounts[o.CopName]++
		}
		if o.Severity != "" {
			severityCounts[o.Severity]++
		}
	}

	// Collect all unique cop names.
	allCops = make([]string, 0, len(copCounts))
	for name := range copCounts {
		allCops = append(allCops, name)
	}

	// Sort cops by count descending, take top 3.
	type copEntry struct {
		Name  string
		Count int
	}
	entries := make([]copEntry, 0, len(copCounts))
	for name, count := range copCounts {
		entries = append(entries, copEntry{name, count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Name < entries[j].Name
	})

	limit := 3
	if len(entries) < limit {
		limit = len(entries)
	}
	topCops = make([]string, limit)
	for i := 0; i < limit; i++ {
		topCops[i] = entries[i].Name
	}

	return namespaceCounts, severityCounts, topCops, allCops
}

// parsedOffense is a minimal struct for offense parsing.
type parsedOffense struct {
	CopName  string `json:"cop_name"`
	Severity string `json:"severity"`
}

// parseOffensesFlat parses the offences JSONB into a flat list of offenses.
// Handles both the file-based RuboCop format and the flat format.
func parseOffensesFlat(data []byte) []parsedOffense {
	// Try file-based format first.
	type fileOffense struct {
		CopName  string `json:"cop_name"`
		Severity string `json:"severity"`
	}
	type fileEntry struct {
		Path     string        `json:"path"`
		Offenses []fileOffense `json:"offenses"`
	}

	var fileEntries []fileEntry
	if err := json.Unmarshal(data, &fileEntries); err == nil && len(fileEntries) > 0 && fileEntries[0].Path != "" {
		var result []parsedOffense
		for _, fe := range fileEntries {
			for _, o := range fe.Offenses {
				result = append(result, parsedOffense{CopName: o.CopName, Severity: o.Severity})
			}
		}
		return result
	}

	// Try flat format.
	var flat []parsedOffense
	if err := json.Unmarshal(data, &flat); err == nil {
		return flat
	}

	return nil
}

// copNamespace extracts the namespace prefix from a cop name.
// e.g. "Chef/Deprecations/NodeSet" → "Chef/Deprecations/"
func copNamespace(copName string) string {
	parts := strings.Split(copName, "/")
	if len(parts) < 3 {
		return copName
	}
	return strings.Join(parts[:2], "/") + "/"
}

// filterViolations applies in-memory filters to the violation items.
func filterViolations(items []cookstyleViolationItem, status, namespace, severity, cop string) []cookstyleViolationItem {
	if status == "" && namespace == "" && severity == "" && cop == "" {
		return items
	}

	filtered := make([]cookstyleViolationItem, 0, len(items))
	for _, item := range items {
		if !matchesStatus(item, status) {
			continue
		}
		if namespace != "" && item.NamespaceCounts[namespace] == 0 {
			continue
		}
		if severity != "" && item.SeverityCounts[severity] == 0 {
			continue
		}
		if cop != "" && !hasCop(item, cop) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// matchesStatus checks if an item matches the requested status filter.
func matchesStatus(item cookstyleViolationItem, status string) bool {
	switch status {
	case "":
		return true
	case "passed":
		return item.Passed && item.ErrorMessage == ""
	case "failed":
		return !item.Passed && item.ErrorMessage == ""
	case "error":
		return item.ErrorMessage != ""
	default:
		return true
	}
}

// hasCop checks whether any offense in the item matches the given cop name
// using the allCops field which stores every unique cop found in the
// item's offences JSONB.
func hasCop(item cookstyleViolationItem, cop string) bool {
	for _, c := range item.allCops {
		if c == cop {
			return true
		}
	}
	return false
}

// sortViolations sorts violation items by the given sort parameters.
func sortViolations(items []cookstyleViolationItem, sp SortParams) {
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch sp.Field {
		case "offence_count":
			less = items[i].OffenceCount < items[j].OffenceCount
		case "deprecation_count":
			less = items[i].DeprecationCount < items[j].DeprecationCount
		default: // "name"
			less = items[i].Name < items[j].Name
		}
		if sp.Order == "desc" {
			return !less
		}
		return less
	})
}
