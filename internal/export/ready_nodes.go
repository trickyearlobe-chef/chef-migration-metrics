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

// ReadyNodeExportParams holds the parameters for generating a ready node
// export. The TargetChefVersion is required — it determines which readiness
// records to evaluate.
type ReadyNodeExportParams struct {
	// TargetChefVersion is the Chef version to check readiness against.
	// Required.
	TargetChefVersion string

	// Format is the output format: "csv", "json", or "chef_search_query".
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

// readyNodeRow is the flattened row shape for ready node exports.
type readyNodeRow struct {
	NodeName        string `json:"node_name"`
	Organisation    string `json:"organisation"`
	Environment     string `json:"environment"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	ChefVersion     string `json:"chef_version"`
	PolicyName      string `json:"policy_name"`
	PolicyGroup     string `json:"policy_group"`
}

// GenerateReadyNodeExport generates an export of nodes that are ready for
// upgrade to the specified target Chef version. It iterates over all
// organisations (respecting the organisation filter), fetches node snapshots,
// applies node-level filters, evaluates readiness, and collects nodes where
// is_ready == true.
//
// The output format is determined by params.Format:
//   - "csv": RFC 4180 CSV with a header row
//   - "json": JSON array of objects
//   - "chef_search_query": plain text Chef search query string
//     ("name:node1 OR name:node2 OR ...")
func GenerateReadyNodeExport(ctx context.Context, db DataStore, params ReadyNodeExportParams) (*ExportResult, error) {
	if params.TargetChefVersion == "" {
		return nil, fmt.Errorf("export: target_chef_version is required for ready node export")
	}
	if params.Format == "" {
		return nil, fmt.Errorf("export: format is required")
	}

	rows, orgNameByID, err := collectReadyNodes(ctx, db, params)
	if err != nil {
		return nil, err
	}

	// Flatten into export rows.
	exportRows := make([]readyNodeRow, 0, len(rows))
	for _, r := range rows {
		exportRows = append(exportRows, readyNodeRow{
			NodeName:        r.node.NodeName,
			Organisation:    orgNameByID[r.node.OrganisationID],
			Environment:     r.node.ChefEnvironment,
			Platform:        r.node.Platform,
			PlatformVersion: r.node.PlatformVersion,
			ChefVersion:     r.node.ChefVersion,
			PolicyName:      r.node.PolicyName,
			PolicyGroup:     r.node.PolicyGroup,
		})
	}

	// Enforce MaxRows.
	if params.MaxRows > 0 && len(exportRows) > params.MaxRows {
		exportRows = exportRows[:params.MaxRows]
	}

	return renderReadyNodeExport(exportRows, params)
}

// nodeWithReadiness pairs a node snapshot with its readiness record.
type nodeWithReadiness struct {
	node      datastore.NodeSnapshot
	readiness datastore.NodeReadiness
}

// collectReadyNodes iterates organisations, fetches nodes, filters them,
// evaluates readiness, and returns the subset of ready nodes.
func collectReadyNodes(ctx context.Context, db DataStore, params ReadyNodeExportParams) ([]nodeWithReadiness, map[string]string, error) {
	allOrgs, err := db.ListOrganisations(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("export: listing organisations: %w", err)
	}

	orgs := FilterOrganisations(allOrgs, params.Filters.Organisation)

	orgNameByID := make(map[string]string, len(orgs))
	for _, org := range orgs {
		orgNameByID[org.ID] = org.Name
	}

	var results []nodeWithReadiness

	for _, org := range orgs {
		nodes, err := db.ListNodeSnapshotsByOrganisation(ctx, org.ID)
		if err != nil {
			// Log-worthy but non-fatal — skip this org.
			continue
		}

		// Apply node-level filters.
		nodes = FilterNodes(nodes, params.Filters)

		for _, node := range nodes {
			readinessRecords, err := db.ListNodeReadinessForSnapshot(ctx, node.ID)
			if err != nil {
				continue
			}

			for _, nr := range readinessRecords {
				if nr.TargetChefVersion != params.TargetChefVersion {
					continue
				}
				if nr.IsReady {
					results = append(results, nodeWithReadiness{
						node:      node,
						readiness: nr,
					})
				}
				break // only one record per target version
			}

			// Early exit if we've hit the max row limit with some headroom.
			if params.MaxRows > 0 && len(results) >= params.MaxRows {
				return results, orgNameByID, nil
			}
		}
	}

	return results, orgNameByID, nil
}

// renderReadyNodeExport renders the collected export rows in the requested
// format and returns the ExportResult.
func renderReadyNodeExport(rows []readyNodeRow, params ReadyNodeExportParams) (*ExportResult, error) {
	var data []byte
	var contentType string
	var ext string

	switch params.Format {
	case "csv":
		d, err := renderReadyNodeCSV(rows)
		if err != nil {
			return nil, err
		}
		data = d
		contentType = "text/csv; charset=utf-8"
		ext = "csv"

	case "json":
		d, err := renderReadyNodeJSON(rows)
		if err != nil {
			return nil, err
		}
		data = d
		contentType = "application/json; charset=utf-8"
		ext = "json"

	case "chef_search_query":
		data = renderReadyNodeChefSearchQuery(rows)
		contentType = "text/plain; charset=utf-8"
		ext = "txt"

	default:
		return nil, fmt.Errorf("export: unsupported format %q for ready node export", params.Format)
	}

	filename := fmt.Sprintf("ready_nodes_%s.%s", time.Now().UTC().Format("2006-01-02"), ext)

	result := &ExportResult{
		RowCount:    len(rows),
		ContentType: contentType,
		Filename:    filename,
	}

	if params.OutputPath != "" {
		// Write to disk for async exports.
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
		// Return in-memory for sync exports.
		result.Data = data
		result.FileSizeBytes = int64(len(data))
	}

	return result, nil
}

// renderReadyNodeCSV renders the export rows as RFC 4180 CSV with a header row.
func renderReadyNodeCSV(rows []readyNodeRow) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Header row.
	header := []string{
		"node_name", "organisation", "environment", "platform",
		"platform_version", "chef_version", "policy_name", "policy_group",
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

// renderReadyNodeJSON renders the export rows as a JSON array of objects.
func renderReadyNodeJSON(rows []readyNodeRow) ([]byte, error) {
	// Ensure we produce [] rather than null for empty slices.
	if rows == nil {
		rows = []readyNodeRow{}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("export: marshalling JSON: %w", err)
	}
	// Append trailing newline for well-formed files.
	data = append(data, '\n')
	return data, nil
}

// renderReadyNodeChefSearchQuery renders the export rows as a Chef search
// query string: "name:node1 OR name:node2 OR ...". If no rows are present,
// an empty string is returned.
func renderReadyNodeChefSearchQuery(rows []readyNodeRow) []byte {
	if len(rows) == 0 {
		return []byte("")
	}

	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		// Escape special characters in node names for Chef search syntax.
		name := strings.ReplaceAll(r.NodeName, " ", `\ `)
		parts = append(parts, "name:"+name)
	}

	return []byte(strings.Join(parts, " OR ") + "\n")
}
