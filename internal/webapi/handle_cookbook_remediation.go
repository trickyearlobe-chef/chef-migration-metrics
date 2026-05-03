// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
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
	if targetVersion == "" {
		targetVersion = r.defaultTargetVersion()
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	// Try to find the cookbook as a server cookbook first (which has versions),
	// then fall back to git repos.
	var cookstyleOffences []byte
	var cookstylePassed *bool
	var cookstyleScannedAt string
	var hasCookstyleResult bool
	var complexityScore int
	var complexityLabel string
	var autoCorrectableCount int
	var manualFixCount int
	var deprecationCount int
	var correctnessCount int
	var modernizeCount int
	var errorCount int
	isGitRepo := false

	serverCookbooks, err := r.db.ListServerCookbooksByName(ctx, cookbookName)
	if err != nil {
		r.logf("ERROR", "listing server cookbooks for remediation detail %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to look up cookbook.")
		return
	}

	// Find the matching version among server cookbooks.
	var serverOrgName string
	var foundServerCookbook bool
	for _, sc := range serverCookbooks {
		if sc.Version == cookbookVersion {
			serverOrgName = sc.OrganisationName
			foundServerCookbook = true
			break
		}
	}

	// Track git repo URL for autocorrect preview lookups.
	var gitRepoURL string

	if foundServerCookbook {
		// Fetch server cookbook cookstyle result.
		csResult, csErr := r.db.GetServerCookbookCookstyleResult(ctx, serverOrgName, cookbookName, cookbookVersion, targetVersion)
		if csErr != nil {
			r.logf("ERROR", "getting cookstyle result for server cookbook %s@%s target %s: %v", cookbookName, cookbookVersion, targetVersion, csErr)
			WriteInternalError(w, "Failed to fetch cookstyle results.")
			return
		}
		if csResult != nil {
			cookstyleOffences = csResult.Offences
			p := csResult.Passed
			cookstylePassed = &p
			cookstyleScannedAt = csResult.ScannedAt.Format("2006-01-02T15:04:05Z")
			hasCookstyleResult = true
		}

		// Fetch complexity records for summary stats.
		complexities, cxErr := r.db.ListServerCookbookComplexitiesByCookbook(ctx, serverOrgName, cookbookName, cookbookVersion)
		if cxErr != nil {
			r.logf("WARN", "listing complexity for server cookbook %s/%s@%s: %v", serverOrgName, cookbookName, cookbookVersion, cxErr)
		}
		for _, cc := range complexities {
			if cc.TargetChefVersion == targetVersion {
				complexityScore = cc.ComplexityScore
				complexityLabel = cc.ComplexityLabel
				autoCorrectableCount = cc.AutoCorrectableCount
				manualFixCount = cc.ManualFixCount
				deprecationCount = cc.DeprecationCount
				correctnessCount = cc.CorrectnessCount
				modernizeCount = cc.ModernizeCount
				errorCount = cc.ErrorCount
				break
			}
		}
	} else {
		// Try git repos — git repos don't have a per-version concept in the
		// same way; use the first repo matching the name.
		gitRepos, grErr := r.db.ListGitReposByName(ctx, cookbookName)
		if grErr != nil {
			r.logf("ERROR", "listing git repos for remediation detail %s: %v", cookbookName, grErr)
			WriteInternalError(w, "Failed to look up cookbook.")
			return
		}
		if len(gitRepos) == 0 {
			WriteNotFound(w, fmt.Sprintf("Cookbook %q version %q not found.", cookbookName, cookbookVersion))
			return
		}
		isGitRepo = true
		gitRepoURL = gitRepos[0].GitRepoURL

		csResult, csErr := r.db.GetGitRepoCookstyleResult(ctx, gitRepos[0].Name, gitRepoURL, targetVersion)
		if csErr != nil {
			r.logf("ERROR", "getting cookstyle result for git repo %s target %s: %v", cookbookName, targetVersion, csErr)
			WriteInternalError(w, "Failed to fetch cookstyle results.")
			return
		}
		if csResult != nil {
			cookstyleOffences = csResult.Offences
			p := csResult.Passed
			cookstylePassed = &p
			cookstyleScannedAt = csResult.ScannedAt.Format("2006-01-02T15:04:05Z")
			hasCookstyleResult = true
		}

		complexities, cxErr := r.db.ListGitRepoComplexitiesByRepo(ctx, cookbookName, gitRepoURL)
		if cxErr != nil {
			r.logf("WARN", "listing complexity for git repo %s: %v", cookbookName, cxErr)
		}
		for _, cc := range complexities {
			if cc.TargetChefVersion == targetVersion {
				complexityScore = cc.ComplexityScore
				complexityLabel = cc.ComplexityLabel
				autoCorrectableCount = cc.AutoCorrectableCount
				manualFixCount = cc.ManualFixCount
				deprecationCount = cc.DeprecationCount
				correctnessCount = cc.CorrectnessCount
				modernizeCount = cc.ModernizeCount
				errorCount = cc.ErrorCount
				break
			}
		}
	}
	// Suppress unused variable warning for isGitRepo.
	_ = isGitRepo

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

	if len(cookstyleOffences) > 0 {
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
		if err := json.Unmarshal(cookstyleOffences, &fileEntries); err == nil && len(fileEntries) > 0 && fileEntries[0].Path != "" {
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
			if err := json.Unmarshal(cookstyleOffences, &flatParsed); err == nil {
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

	if hasCookstyleResult {
		if isGitRepo {
			preview, acErr := r.db.GetGitRepoAutocorrectPreview(ctx, cookbookName, gitRepoURL, targetVersion)
			if acErr != nil {
				r.logf("WARN", "getting git repo autocorrect preview for %s target %s: %v", cookbookName, targetVersion, acErr)
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
		} else {
			preview, acErr := r.db.GetServerCookbookAutocorrectPreview(ctx, serverOrgName, cookbookName, cookbookVersion, targetVersion)
			if acErr != nil {
				r.logf("WARN", "getting server cookbook autocorrect preview for %s/%s@%s target %s: %v", serverOrgName, cookbookName, cookbookVersion, targetVersion, acErr)
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
	}

	// Build complexity breakdown — shows each scoring component with
	// count × weight = subtotal so users understand the formula.
	var tkStatus string
	var tkWeight int
	if isGitRepo && gitRepoURL != "" {
		tkResults, tkErr := r.db.ListGitKitchenResultsByRepo(ctx, cookbookName)
		if tkErr == nil {
			var passed, failed int
			for _, res := range tkResults {
				if res.Passed == nil || res.TargetChefVersion != targetVersion {
					continue
				}
				if *res.Passed {
					passed++
				} else {
					failed++
				}
			}
			tkStatus = tkstatus.ComputeTKStatus(passed, failed)
		}
	}
	switch tkStatus {
	case "failed":
		tkWeight = remediation.WeightTKFail
	case "partial":
		tkWeight = remediation.WeightTKPartial
	}

	type breakdownItem struct {
		Count    int    `json:"count"`
		Weight   int    `json:"weight"`
		Subtotal int    `json:"subtotal"`
		Status   string `json:"status,omitempty"`
	}

	breakdown := map[string]breakdownItem{
		"error_fatal": {
			Count:    errorCount,
			Weight:   remediation.WeightErrorFatal,
			Subtotal: errorCount * remediation.WeightErrorFatal,
		},
		"deprecation": {
			Count:    deprecationCount,
			Weight:   remediation.WeightDeprecation,
			Subtotal: deprecationCount * remediation.WeightDeprecation,
		},
		"correctness": {
			Count:    correctnessCount,
			Weight:   remediation.WeightCorrectness,
			Subtotal: correctnessCount * remediation.WeightCorrectness,
		},
		"manual_fix": {
			Count:    manualFixCount,
			Weight:   remediation.WeightNonAutoCorrectable,
			Subtotal: manualFixCount * remediation.WeightNonAutoCorrectable,
		},
		"modernize": {
			Count:    modernizeCount,
			Weight:   remediation.WeightModernize,
			Subtotal: modernizeCount * remediation.WeightModernize,
		},
		"tk_fail": {
			Status:   tkStatus,
			Weight:   tkWeight,
			Subtotal: tkWeight,
		},
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"cookbook_name":         cookbookName,
		"cookbook_version":      cookbookVersion,
		"target_chef_version":  targetVersion,
		"complexity_score":     complexityScore,
		"complexity_label":     complexityLabel,
		"complexity_breakdown": breakdown,
		"cookstyle_passed":     cookstylePassed,
		"scanned_at":           cookstyleScannedAt,
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
