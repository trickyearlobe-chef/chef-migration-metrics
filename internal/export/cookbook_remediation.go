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
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// CookbookRemediationExportParams holds the parameters for generating a
// cookbook remediation report export.
type CookbookRemediationExportParams struct {
	// Format is the output format: "csv" or "json".
	// (chef_search_query is not supported for cookbook remediation exports.)
	Format string

	// Filters contains optional filter criteria. Only Organisation,
	// TargetChefVersion, and ComplexityLabel are used for this export type.
	Filters Filters

	// MaxRows is the upper bound on the number of data rows to include in
	// the export. If <= 0, no limit is applied.
	MaxRows int

	// OutputPath is the filesystem path to write the export file. If empty,
	// the export data is returned in-memory via ExportResult.Data (used for
	// synchronous inline exports).
	OutputPath string
}

// cookbookRemediationRow is the flattened row shape for cookbook remediation
// exports. Each row represents one cookbook version evaluated against one
// target Chef version.
type cookbookRemediationRow struct {
	CookbookName         string `json:"cookbook_name"`
	Version              string `json:"version"`
	Organisation         string `json:"organisation"`
	TargetChefVersion    string `json:"target_chef_version"`
	ComplexityScore      int    `json:"complexity_score"`
	ComplexityLabel      string `json:"complexity_label"`
	AffectedNodeCount    int    `json:"affected_node_count"`
	AffectedRoleCount    int    `json:"affected_role_count"`
	AutoCorrectableCount int    `json:"auto_correctable_count"`
	ManualFixCount       int    `json:"manual_fix_count"`
	DeprecationCount     int    `json:"deprecation_count"`
	ErrorCount           int    `json:"error_count"`
}

// GenerateCookbookRemediationExport generates an export of cookbook
// remediation data. For each organisation (respecting the organisation
// filter), it fetches cookbooks and their complexity records, joins them,
// and outputs one row per cookbook-version × target-version combination.
//
// The output format is determined by params.Format:
//   - "csv": RFC 4180 CSV with a header row
//   - "json": JSON array of objects
func GenerateCookbookRemediationExport(ctx context.Context, db DataStore, params CookbookRemediationExportParams) (*ExportResult, error) {
	if params.Format == "" {
		return nil, fmt.Errorf("export: format is required")
	}
	if params.Format == "chef_search_query" {
		return nil, fmt.Errorf("export: chef_search_query format is not supported for cookbook remediation exports")
	}

	rows, err := collectCookbookRemediation(ctx, db, params)
	if err != nil {
		return nil, err
	}

	// Enforce MaxRows.
	if params.MaxRows > 0 && len(rows) > params.MaxRows {
		rows = rows[:params.MaxRows]
	}

	return renderCookbookRemediationExport(rows, params)
}

// collectCookbookRemediation iterates organisations, fetches cookbooks and
// their complexity records, and returns the joined result set.
func collectCookbookRemediation(ctx context.Context, db DataStore, params CookbookRemediationExportParams) ([]cookbookRemediationRow, error) {
	allOrgs, err := db.ListOrganisations(ctx)
	if err != nil {
		return nil, fmt.Errorf("export: listing organisations: %w", err)
	}

	orgs := FilterOrganisations(allOrgs, params.Filters.Organisation)

	orgNameByID := make(map[string]string, len(orgs))
	for _, org := range orgs {
		orgNameByID[org.ID] = org.Name
	}

	var results []cookbookRemediationRow

	for _, org := range orgs {
		cookbooks, err := db.ListServerCookbooksByOrganisation(ctx, org.ID)
		if err != nil {
			// Non-fatal — skip this org.
			continue
		}

		// Build a map from cookbook ID to cookbook metadata for joining.
		cbMap := make(map[string]datastore.ServerCookbook, len(cookbooks))
		for _, cb := range cookbooks {
			cbMap[cb.ID] = cb
		}

		complexities, err := db.ListServerCookbookComplexitiesByOrganisation(ctx, org.ID)
		if err != nil {
			// Non-fatal — skip this org's complexity data.
			continue
		}

		// Apply target version and complexity label filters.
		complexities = FilterComplexities(
			complexities,
			params.Filters.TargetChefVersion,
			params.Filters.ComplexityLabel,
		)

		for _, cc := range complexities {
			cb, ok := cbMap[cc.ServerCookbookID]
			if !ok {
				// Orphaned complexity record — skip.
				continue
			}

			results = append(results, cookbookRemediationRow{
				CookbookName:         cb.Name,
				Version:              cb.Version,
				Organisation:         orgNameByID[org.ID],
				TargetChefVersion:    cc.TargetChefVersion,
				ComplexityScore:      cc.ComplexityScore,
				ComplexityLabel:      cc.ComplexityLabel,
				AffectedNodeCount:    cc.AffectedNodeCount,
				AffectedRoleCount:    cc.AffectedRoleCount,
				AutoCorrectableCount: cc.AutoCorrectableCount,
				ManualFixCount:       cc.ManualFixCount,
				DeprecationCount:     cc.DeprecationCount,
				ErrorCount:           cc.ErrorCount,
			})

			// Early exit if we've hit the max row limit.
			if params.MaxRows > 0 && len(results) >= params.MaxRows {
				return results, nil
			}
		}
	}

	return results, nil
}

// renderCookbookRemediationExport renders the collected export rows in the
// requested format and returns the ExportResult.
func renderCookbookRemediationExport(rows []cookbookRemediationRow, params CookbookRemediationExportParams) (*ExportResult, error) {
	var data []byte
	var contentType string
	var ext string

	switch params.Format {
	case "csv":
		d, err := renderCookbookRemediationCSV(rows)
		if err != nil {
			return nil, err
		}
		data = d
		contentType = "text/csv; charset=utf-8"
		ext = "csv"

	case "json":
		d, err := renderCookbookRemediationJSON(rows)
		if err != nil {
			return nil, err
		}
		data = d
		contentType = "application/json; charset=utf-8"
		ext = "json"

	default:
		return nil, fmt.Errorf("export: unsupported format %q for cookbook remediation export", params.Format)
	}

	filename := fmt.Sprintf("cookbook_remediation_%s.%s", time.Now().UTC().Format("2006-01-02"), ext)

	result := &ExportResult{
		RowCount:    len(rows),
		ContentType: contentType,
		Filename:    filename,
	}

	if params.OutputPath != "" {
		// filepath.Clean neutralises any path traversal sequences so that
		// user-influenced values cannot escape the output directory.
		cleanPath := filepath.Clean(params.OutputPath)
		dir := filepath.Dir(cleanPath)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("export: creating output directory: %w", err)
		}
		if err := os.WriteFile(cleanPath, data, 0o640); err != nil {
			return nil, fmt.Errorf("export: writing export file: %w", err)
		}
		result.FilePath = cleanPath
		result.FileSizeBytes = int64(len(data))
	} else {
		result.Data = data
		result.FileSizeBytes = int64(len(data))
	}

	return result, nil
}

// renderCookbookRemediationCSV renders the export rows as RFC 4180 CSV with
// a header row.
func renderCookbookRemediationCSV(rows []cookbookRemediationRow) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	header := []string{
		"cookbook_name", "version", "organisation", "target_chef_version",
		"complexity_score", "complexity_label",
		"affected_node_count", "affected_role_count",
		"auto_correctable_count", "manual_fix_count",
		"deprecation_count", "error_count",
	}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("export: writing CSV header: %w", err)
	}

	for _, r := range rows {
		record := []string{
			r.CookbookName,
			r.Version,
			r.Organisation,
			r.TargetChefVersion,
			fmt.Sprintf("%d", r.ComplexityScore),
			r.ComplexityLabel,
			fmt.Sprintf("%d", r.AffectedNodeCount),
			fmt.Sprintf("%d", r.AffectedRoleCount),
			fmt.Sprintf("%d", r.AutoCorrectableCount),
			fmt.Sprintf("%d", r.ManualFixCount),
			fmt.Sprintf("%d", r.DeprecationCount),
			fmt.Sprintf("%d", r.ErrorCount),
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

// renderCookbookRemediationJSON renders the export rows as a JSON array of
// objects.
func renderCookbookRemediationJSON(rows []cookbookRemediationRow) ([]byte, error) {
	// Ensure we produce [] rather than null for empty slices.
	if rows == nil {
		rows = []cookbookRemediationRow{}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("export: marshalling JSON: %w", err)
	}
	// Append trailing newline for well-formed files.
	data = append(data, '\n')
	return data, nil
}
