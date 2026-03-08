// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ---------------------------------------------------------------------------
// Cookbook Remediation Detail endpoint
//
// GET /api/v1/cookbooks/:name/:version/remediation
//
// Returns a rich per-cookbook remediation view:
//   - Offenses grouped by cop name, each with remediation guidance
//     (description, migration URL, before/after patterns)
//   - Auto-correct preview with unified diffs
//   - Statistics on correctable vs. remaining offenses
//
// Query parameters:
//   - target_chef_version: filter by target Chef version (optional; defaults
//     to first configured target version)
// ---------------------------------------------------------------------------

// handleCookbookRemediation handles GET /api/v1/cookbooks/:name/:version/remediation.
func (r *Router) handleCookbookRemediation(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Extract path segments: /api/v1/cookbooks/{name}/{version}/remediation
	segments := pathSegments(req.URL.Path, "/api/v1/cookbooks/")
	if len(segments) < 3 || segments[len(segments)-1] != "remediation" {
		WriteNotFound(w, "Expected path: /api/v1/cookbooks/:name/:version/remediation")
		return
	}

	cookbookName := segments[0]
	cookbookVersion := segments[1]

	if cookbookName == "" || cookbookVersion == "" {
		WriteBadRequest(w, "Cookbook name and version are required.")
		return
	}

	ctx := req.Context()

	// Resolve target Chef version — default to the first configured one.
	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" && len(r.cfg.TargetChefVersions) > 0 {
		targetVersion = r.cfg.TargetChefVersions[0]
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	// Find the cookbook by name — we need to locate the specific version.
	cookbooks, err := r.db.ListCookbooksByName(ctx, cookbookName)
	if err != nil {
		r.logf("ERROR", "listing cookbooks for remediation detail %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to look up cookbook.")
		return
	}

	// Find the matching version.
	var cookbookID string
	for _, cb := range cookbooks {
		if cb.Version == cookbookVersion {
			cookbookID = cb.ID
			break
		}
	}
	if cookbookID == "" {
		WriteNotFound(w, fmt.Sprintf("Cookbook %q version %q not found.", cookbookName, cookbookVersion))
		return
	}

	// Fetch the cookstyle result for this cookbook + target version.
	cookstyleResult, err := r.db.GetCookstyleResult(ctx, cookbookID, targetVersion)
	if err != nil {
		r.logf("ERROR", "getting cookstyle result for %s@%s target %s: %v", cookbookName, cookbookVersion, targetVersion, err)
		WriteInternalError(w, "Failed to fetch cookstyle results.")
		return
	}

	// Also fetch the complexity record for summary stats.
	complexities, err := r.db.ListCookbookComplexitiesForCookbook(ctx, cookbookID)
	if err != nil {
		r.logf("WARN", "listing complexity for cookbook %s: %v", cookbookID, err)
	}

	// Find the matching complexity for our target version.
	var complexityScore int
	var complexityLabel string
	var autoCorrectableCount int
	var manualFixCount int
	var deprecationCount int
	var errorCount int
	for _, cc := range complexities {
		if cc.TargetChefVersion == targetVersion {
			complexityScore = cc.ComplexityScore
			complexityLabel = cc.ComplexityLabel
			autoCorrectableCount = cc.AutoCorrectableCount
			manualFixCount = cc.ManualFixCount
			deprecationCount = cc.DeprecationCount
			errorCount = cc.ErrorCount
			break
		}
	}

	// Build offense groups from the cookstyle result offenses JSON.
	type offenseLocation struct {
		File        string `json:"file"`
		StartLine   int    `json:"start_line"`
		StartColumn int    `json:"start_column"`
		LastLine    int    `json:"last_line"`
		LastColumn  int    `json:"last_column"`
	}

	type offense struct {
		CopName     string          `json:"cop_name"`
		Severity    string          `json:"severity"`
		Message     string          `json:"message"`
		Correctable bool            `json:"correctable"`
		Location    offenseLocation `json:"location"`
	}

	type copRemediation struct {
		CopName            string `json:"cop_name"`
		Description        string `json:"description"`
		MigrationURL       string `json:"migration_url"`
		IntroducedIn       string `json:"introduced_in,omitempty"`
		RemovedIn          string `json:"removed_in,omitempty"`
		ReplacementPattern string `json:"replacement_pattern,omitempty"`
	}

	type offenseGroup struct {
		CopName          string          `json:"cop_name"`
		Severity         string          `json:"severity"`
		Count            int             `json:"count"`
		CorrectableCount int             `json:"correctable_count"`
		Remediation      *copRemediation `json:"remediation,omitempty"`
		Offenses         []offense       `json:"offenses"`
	}

	// Parse offenses from the JSONB column. The stored format is the
	// RuboCop JSON output's file-based offense list. We normalise it
	// into a flat list, then group by cop name.
	//
	// Expected stored format (RuboCop JSON output):
	// [
	//   {
	//     "path": "recipes/default.rb",
	//     "offenses": [
	//       {
	//         "cop_name": "Chef/Deprecations/...",
	//         "severity": "warning",
	//         "message": "...",
	//         "correctable": true,
	//         "location": { "start_line": 1, "start_column": 1, "last_line": 1, "last_column": 10 }
	//       }
	//     ]
	//   }
	// ]
	//
	// Alternative flat format:
	// [
	//   {
	//     "cop_name": "...",
	//     "severity": "...",
	//     "message": "...",
	//     "correctable": false,
	//     "location": { "start_line": 1, ... }
	//   }
	// ]

	var flatOffenses []offense

	if cookstyleResult != nil && len(cookstyleResult.Offences) > 0 {
		// Try the file-based (RuboCop) format first.
		type fileOffense struct {
			CopName     string `json:"cop_name"`
			Severity    string `json:"severity"`
			Message     string `json:"message"`
			Correctable bool   `json:"correctable"`
			Location    struct {
				StartLine   int `json:"start_line"`
				StartColumn int `json:"start_column"`
				LastLine    int `json:"last_line"`
				LastColumn  int `json:"last_column"`
			} `json:"location"`
		}
		type fileEntry struct {
			Path     string        `json:"path"`
			Offenses []fileOffense `json:"offenses"`
		}

		var fileEntries []fileEntry
		if err := json.Unmarshal(cookstyleResult.Offences, &fileEntries); err == nil && len(fileEntries) > 0 && fileEntries[0].Path != "" {
			for _, fe := range fileEntries {
				for _, o := range fe.Offenses {
					flatOffenses = append(flatOffenses, offense{
						CopName:     o.CopName,
						Severity:    o.Severity,
						Message:     o.Message,
						Correctable: o.Correctable,
						Location: offenseLocation{
							File:        fe.Path,
							StartLine:   o.Location.StartLine,
							StartColumn: o.Location.StartColumn,
							LastLine:    o.Location.LastLine,
							LastColumn:  o.Location.LastColumn,
						},
					})
				}
			}
		} else {
			// Try flat format.
			var flatParsed []struct {
				CopName     string `json:"cop_name"`
				Severity    string `json:"severity"`
				Message     string `json:"message"`
				Correctable bool   `json:"correctable"`
				Location    struct {
					File        string `json:"file"`
					StartLine   int    `json:"start_line"`
					StartColumn int    `json:"start_column"`
					LastLine    int    `json:"last_line"`
					LastColumn  int    `json:"last_column"`
				} `json:"location"`
			}
			if err := json.Unmarshal(cookstyleResult.Offences, &flatParsed); err == nil {
				for _, o := range flatParsed {
					flatOffenses = append(flatOffenses, offense{
						CopName:     o.CopName,
						Severity:    o.Severity,
						Message:     o.Message,
						Correctable: o.Correctable,
						Location: offenseLocation{
							File:        o.Location.File,
							StartLine:   o.Location.StartLine,
							StartColumn: o.Location.StartColumn,
							LastLine:    o.Location.LastLine,
							LastColumn:  o.Location.LastColumn,
						},
					})
				}
			} else {
				r.logf("WARN", "failed to parse offenses JSON for cookbook %s@%s: %v", cookbookName, cookbookVersion, err)
			}
		}
	}

	// Group offenses by cop name.
	groupOrder := make([]string, 0)
	groupMap := make(map[string]*offenseGroup)
	for _, o := range flatOffenses {
		g, ok := groupMap[o.CopName]
		if !ok {
			g = &offenseGroup{
				CopName:  o.CopName,
				Severity: o.Severity,
			}
			// Look up remediation guidance from the embedded cop mapping.
			if cm := remediation.LookupCop(o.CopName); cm != nil {
				g.Remediation = &copRemediation{
					CopName:            cm.CopName,
					Description:        cm.Description,
					MigrationURL:       cm.MigrationURL,
					IntroducedIn:       cm.IntroducedIn,
					RemovedIn:          cm.RemovedIn,
					ReplacementPattern: cm.ReplacementPattern,
				}
			}
			groupMap[o.CopName] = g
			groupOrder = append(groupOrder, o.CopName)
		}
		g.Count++
		if o.Correctable {
			g.CorrectableCount++
		}
		g.Offenses = append(g.Offenses, o)
	}

	// Build the sorted groups slice (preserve insertion order which is
	// effectively the order offenses appear in the cookstyle output).
	groups := make([]offenseGroup, 0, len(groupOrder))
	for _, copName := range groupOrder {
		groups = append(groups, *groupMap[copName])
	}

	// Compute statistics.
	totalOffenses := len(flatOffenses)
	correctableOffenses := 0
	for _, o := range flatOffenses {
		if o.Correctable {
			correctableOffenses++
		}
	}
	remainingOffenses := totalOffenses - correctableOffenses

	// Fetch the auto-correct preview if a cookstyle result exists.
	type autocorrectPreviewResp struct {
		Available           bool   `json:"available"`
		TotalOffenses       int    `json:"total_offenses"`
		CorrectableOffenses int    `json:"correctable_offenses"`
		RemainingOffenses   int    `json:"remaining_offenses"`
		FilesModified       int    `json:"files_modified"`
		DiffOutput          string `json:"diff_output"`
		GeneratedAt         string `json:"generated_at,omitempty"`
	}

	acPreview := autocorrectPreviewResp{Available: false}

	if cookstyleResult != nil {
		preview, err := r.db.GetAutocorrectPreview(ctx, cookstyleResult.ID)
		if err != nil {
			r.logf("WARN", "getting autocorrect preview for cookstyle result %s: %v", cookstyleResult.ID, err)
		} else if preview != nil {
			acPreview = autocorrectPreviewResp{
				Available:           true,
				TotalOffenses:       preview.TotalOffenses,
				CorrectableOffenses: preview.CorrectableOffenses,
				RemainingOffenses:   preview.RemainingOffenses,
				FilesModified:       preview.FilesModified,
				DiffOutput:          preview.DiffOutput,
				GeneratedAt:         preview.GeneratedAt.Format("2006-01-02T15:04:05Z"),
			}
		}
	}

	// Build scanned_at from the cookstyle result if available.
	var scannedAt string
	if cookstyleResult != nil {
		scannedAt = cookstyleResult.ScannedAt.Format("2006-01-02T15:04:05Z")
	}

	var passed *bool
	if cookstyleResult != nil {
		p := cookstyleResult.Passed
		passed = &p
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"cookbook_name":       cookbookName,
		"cookbook_version":    cookbookVersion,
		"target_chef_version": targetVersion,
		"complexity_score":    complexityScore,
		"complexity_label":    complexityLabel,
		"cookstyle_passed":    passed,
		"scanned_at":          scannedAt,
		"statistics": map[string]any{
			"total_offenses":         totalOffenses,
			"correctable_offenses":   correctableOffenses,
			"remaining_offenses":     remainingOffenses,
			"auto_correctable_count": autoCorrectableCount,
			"manual_fix_count":       manualFixCount,
			"deprecation_count":      deprecationCount,
			"error_count":            errorCount,
			"offense_groups":         len(groups),
		},
		"offense_groups":      groups,
		"autocorrect_preview": acPreview,
	})
}
