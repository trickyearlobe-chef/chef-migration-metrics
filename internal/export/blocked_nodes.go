// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// BlockedNodeExportParams holds the parameters for generating a blocked node
// export. The TargetChefVersion is required — it determines which readiness
// records to evaluate.
type BlockedNodeExportParams struct {
	// TargetChefVersion is the Chef version to check readiness against.
	// Required.
	TargetChefVersion string

	// Format is the output format: "csv" or "json".
	// (chef_search_query is not supported for blocked node exports.)
	Format string

	// Filters contains optional filter criteria (organisation, environment,
	// platform, etc.) applied to the node list before readiness evaluation.
	Filters Filters

	// MaxRows is the upper bound on the number of data rows to include in
	// the export. If <= 0, no limit is applied.
	MaxRows int

	// OutputPath is the filesystem path to write the export file. If empty,
	// the export data is returned in-memory via ExportResult.Data (used for
	// synchronous inline exports).
	OutputPath string
}

// blockedNodeRow is the flattened row shape for blocked node exports.
// It extends the basic node info with blocking reasons and complexity scores.
type blockedNodeRow struct {
	NodeName          string   `json:"node_name"`
	Organisation      string   `json:"organisation"`
	Environment       string   `json:"environment"`
	Platform          string   `json:"platform"`
	PlatformVersion   string   `json:"platform_version"`
	ChefVersion       string   `json:"chef_version"`
	PolicyName        string   `json:"policy_name"`
	PolicyGroup       string   `json:"policy_group"`
	TargetChefVersion string   `json:"target_chef_version"`
	BlockingCookbooks []string `json:"blocking_cookbooks"`
	BlockingReasons   []string `json:"blocking_reasons"`
	ComplexityScore   int      `json:"complexity_score"`
}

// GenerateBlockedNodeExport generates an export of nodes that are blocked
// from upgrading to the specified target Chef version. It iterates over all
// organisations (respecting the organisation filter), fetches node snapshots,
// applies node-level filters, evaluates readiness, and collects nodes where
// is_ready == false.
//
// The output includes blocking cookbook names, blocking reasons, and an
// aggregate complexity score (sum of blocking cookbook complexity scores).
//
// The output format is determined by params.Format:
//   - "csv": RFC 4180 CSV with a header row. Array fields are semicolon-delimited.
//   - "json": JSON array of objects with full array fields.
func GenerateBlockedNodeExport(ctx context.Context, db DataStore, params BlockedNodeExportParams) (*ExportResult, error) {
	if params.TargetChefVersion == "" {
		return nil, fmt.Errorf("export: target_chef_version is required for blocked node export")
	}
	if params.Format == "" {
		return nil, fmt.Errorf("export: format is required")
	}
	if params.Format == "chef_search_query" {
		return nil, fmt.Errorf("export: chef_search_query format is not supported for blocked node exports")
	}

	rows, err := collectBlockedNodes(ctx, db, params)
	if err != nil {
		return nil, err
	}

	// Enforce MaxRows.
	if params.MaxRows > 0 && len(rows) > params.MaxRows {
		rows = rows[:params.MaxRows]
	}

	return renderBlockedNodeExport(rows, params)
}

// collectBlockedNodes iterates organisations, fetches nodes, filters them,
// evaluates readiness, and returns the subset of blocked nodes with their
// blocking details and complexity scores.
func collectBlockedNodes(ctx context.Context, db DataStore, params BlockedNodeExportParams) ([]blockedNodeRow, error) {
	allOrgs, err := db.ListOrganisations(ctx)
	if err != nil {
		return nil, fmt.Errorf("export: listing organisations: %w", err)
	}

	orgs := FilterOrganisations(allOrgs, params.Filters.Organisation)

	orgNameByID := make(map[string]string, len(orgs))
	for _, org := range orgs {
		orgNameByID[org.ID] = org.Name
	}

	var results []blockedNodeRow

	for _, org := range orgs {
		nodes, err := db.ListNodeSnapshotsByOrganisation(ctx, org.ID)
		if err != nil {
			continue
		}

		// Apply node-level filters.
		nodes = FilterNodes(nodes, params.Filters)

		// Pre-load complexity scores for this organisation so we can look
		// them up by cookbook ID without repeated DB calls.
		complexities, err := db.ListCookbookComplexitiesForOrganisation(ctx, org.ID)
		if err != nil {
			// Non-fatal — complexity scores will be zero.
			complexities = nil
		}

		// Build a map from cookbook ID to complexity score for the target version.
		complexityByCookbookID := buildComplexityMap(complexities, params.TargetChefVersion)

		// Also build a map from cookbook name to cookbook ID so we can resolve
		// blocking cookbook names to complexity scores.
		cookbooks, err := db.ListCookbooksByOrganisation(ctx, org.ID)
		if err != nil {
			cookbooks = nil
		}
		cookbookIDByName := make(map[string]string, len(cookbooks))
		for _, cb := range cookbooks {
			// If there are multiple versions of the same cookbook, any ID
			// will do — complexity is per-cookbook-version but the blocking
			// cookbooks list typically uses just the name.
			cookbookIDByName[cb.Name] = cb.ID
		}

		for _, node := range nodes {
			readinessRecords, err := db.ListNodeReadinessForSnapshot(ctx, node.ID)
			if err != nil {
				continue
			}

			for _, nr := range readinessRecords {
				if nr.TargetChefVersion != params.TargetChefVersion {
					continue
				}
				if !nr.IsReady {
					row := buildBlockedNodeRow(
						node, nr, orgNameByID[node.OrganisationID],
						params.TargetChefVersion,
						complexityByCookbookID, cookbookIDByName,
					)
					results = append(results, row)
				}
				break // only one record per target version
			}

			// Early exit if we've hit the max row limit.
			if params.MaxRows > 0 && len(results) >= params.MaxRows {
				return results, nil
			}
		}
	}

	return results, nil
}

// buildComplexityMap creates a map from cookbook ID to complexity score
// for the specified target Chef version.
func buildComplexityMap(complexities []datastore.CookbookComplexity, targetVersion string) map[string]int {
	m := make(map[string]int, len(complexities))
	for _, cc := range complexities {
		if cc.TargetChefVersion == targetVersion {
			m[cc.CookbookID] = cc.ComplexityScore
		}
	}
	return m
}

// buildBlockedNodeRow constructs a blockedNodeRow from a node snapshot and
// its readiness record, resolving blocking cookbook names to complexity scores.
func buildBlockedNodeRow(
	node datastore.NodeSnapshot,
	nr datastore.NodeReadiness,
	orgName, targetVersion string,
	complexityByCookbookID map[string]int,
	cookbookIDByName map[string]string,
) blockedNodeRow {
	// Parse blocking cookbooks from the JSONB field.
	blockingCookbooks := parseBlockingCookbooks(nr.BlockingCookbooks)

	// Derive blocking reasons from the readiness record.
	blockingReasons := deriveBlockingReasons(nr)

	// Sum complexity scores for all blocking cookbooks.
	totalComplexity := 0
	for _, cbName := range blockingCookbooks {
		if cbID, ok := cookbookIDByName[cbName]; ok {
			totalComplexity += complexityByCookbookID[cbID]
		}
	}

	return blockedNodeRow{
		NodeName:          node.NodeName,
		Organisation:      orgName,
		Environment:       node.ChefEnvironment,
		Platform:          node.Platform,
		PlatformVersion:   node.PlatformVersion,
		ChefVersion:       node.ChefVersion,
		PolicyName:        node.PolicyName,
		PolicyGroup:       node.PolicyGroup,
		TargetChefVersion: targetVersion,
		BlockingCookbooks: blockingCookbooks,
		BlockingReasons:   blockingReasons,
		ComplexityScore:   totalComplexity,
	}
}

// parseBlockingCookbooks extracts blocking cookbook names from a JSONB field.
// The field can be a JSON array of strings (["cb1", "cb2"]) or a JSON array
// of objects ([{"name": "cb1", ...}]). Returns an empty slice on parse failure.
func parseBlockingCookbooks(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}
	}

	// Try parsing as array of strings first.
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		return names
	}

	// Try parsing as array of objects with a "name" field.
	var objs []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &objs); err == nil {
		result := make([]string, 0, len(objs))
		for _, o := range objs {
			if o.Name != "" {
				if o.Version != "" {
					result = append(result, o.Name+"@"+o.Version)
				} else {
					result = append(result, o.Name)
				}
			}
		}
		return result
	}

	return []string{}
}

// deriveBlockingReasons builds a human-readable list of reasons the node is
// blocked based on the readiness record's boolean fields.
func deriveBlockingReasons(nr datastore.NodeReadiness) []string {
	var reasons []string

	if !nr.AllCookbooksCompatible {
		reasons = append(reasons, "incompatible cookbooks")
	}

	if nr.SufficientDiskSpace != nil && !*nr.SufficientDiskSpace {
		reasons = append(reasons, "insufficient disk space")
	}

	if nr.StaleData {
		reasons = append(reasons, "stale node data")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "blocked (reason unspecified)")
	}

	return reasons
}

// renderBlockedNodeExport renders the collected export rows in the requested
// format and returns the ExportResult.
func renderBlockedNodeExport(rows []blockedNodeRow, params BlockedNodeExportParams) (*ExportResult, error) {
	var data []byte
	var contentType string
	var ext string

	switch params.Format {
	case "csv":
		d, err := renderBlockedNodeCSV(rows)
		if err != nil {
			return nil, err
		}
		data = d
		contentType = "text/csv; charset=utf-8"
		ext = "csv"

	case "json":
		d, err := renderBlockedNodeJSON(rows)
		if err != nil {
			return nil, err
		}
		data = d
		contentType = "application/json; charset=utf-8"
		ext = "json"

	default:
		return nil, fmt.Errorf("export: unsupported format %q for blocked node export", params.Format)
	}

	filename := fmt.Sprintf("blocked_nodes_%s.%s", time.Now().UTC().Format("2006-01-02"), ext)

	result := &ExportResult{
		RowCount:    len(rows),
		ContentType: contentType,
		Filename:    filename,
	}

	if params.OutputPath != "" {
		dir := filepath.Dir(params.OutputPath)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("export: creating output directory: %w", err)
		}
		if err := os.WriteFile(params.OutputPath, data, 0o640); err != nil {
			return nil, fmt.Errorf("export: writing export file: %w", err)
		}
		result.FilePath = params.OutputPath
		result.FileSizeBytes = int64(len(data))
	} else {
		result.Data = data
		result.FileSizeBytes = int64(len(data))
	}

	return result, nil
}

// renderBlockedNodeCSV renders the export rows as RFC 4180 CSV with a header
// row. Array fields (blocking_cookbooks, blocking_reasons) are joined with
// semicolons for CSV compatibility.
func renderBlockedNodeCSV(rows []blockedNodeRow) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	header := []string{
		"node_name", "organisation", "environment", "platform",
		"platform_version", "chef_version", "policy_name", "policy_group",
		"target_chef_version", "blocking_cookbooks", "blocking_reasons",
		"complexity_score",
	}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("export: writing CSV header: %w", err)
	}

	for _, r := range rows {
		record := []string{
			r.NodeName,
			r.Organisation,
			r.Environment,
			r.Platform,
			r.PlatformVersion,
			r.ChefVersion,
			r.PolicyName,
			r.PolicyGroup,
			r.TargetChefVersion,
			strings.Join(r.BlockingCookbooks, ";"),
			strings.Join(r.BlockingReasons, ";"),
			fmt.Sprintf("%d", r.ComplexityScore),
		}
		if err := w.Write(record); err != nil {
			return nil, fmt.Errorf("export: writing CSV row: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("export: flushing CSV: %w", err)
	}

	return buf.Bytes(), nil
}

// renderBlockedNodeJSON renders the export rows as a JSON array of objects.
func renderBlockedNodeJSON(rows []blockedNodeRow) ([]byte, error) {
	if rows == nil {
		rows = []blockedNodeRow{}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("export: marshalling JSON: %w", err)
	}
	data = append(data, '\n')
	return data, nil
}
