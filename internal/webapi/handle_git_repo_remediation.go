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
// Git Repo Remediation Detail endpoint
//
// GET /api/v1/git-repos/:name/:version/remediation
//
// Returns a rich per-git-repo remediation view:
//   - Offenses grouped by cop name, each with remediation guidance
//     (description, migration URL, before/after patterns)
//   - Auto-correct preview with unified diffs
//   - Statistics on correctable vs. remaining offenses
//
// The :version segment is accepted for URL consistency with the cookbook
// remediation endpoint but is not used for git repos (they don't have
// per-version semantics in the same way). The most recently fetched git
// repo matching the name is used.
//
// Query parameters:
//   - target_chef_version: filter by target Chef version (optional; defaults
//     to first configured target version)
// ---------------------------------------------------------------------------

// handleGitRepoRemediation handles GET /api/v1/git-repos/:name/:version/remediation.
func (r *Router) handleGitRepoRemediation(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Extract path segments: /api/v1/git-repos/{name}/{version}/remediation
	segments := pathSegments(req.URL.Path, "/api/v1/git-repos/")
	if len(segments) < 3 || segments[len(segments)-1] != "remediation" {
		WriteNotFound(w, "Expected path: /api/v1/git-repos/:name/:version/remediation")
		return
	}

	repoName := segments[0]
	repoVersion := segments[1] // accepted for URL consistency

	if repoName == "" || repoVersion == "" {
		WriteBadRequest(w, "Git repo name and version are required.")
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

	// Find the git repo by name (most recently fetched).
	gitRepos, grErr := r.db.ListGitReposByName(ctx, repoName)
	if grErr != nil {
		r.logf("ERROR", "listing git repos for remediation detail %s: %v", repoName, grErr)
		WriteInternalError(w, "Failed to look up git repo.")
		return
	}
	if len(gitRepos) == 0 {
		WriteNotFound(w, fmt.Sprintf("Git repo %q not found.", repoName))
		return
	}

	gitRepoID := gitRepos[0].ID

	// Fetch cookstyle result.
	var cookstyleOffences []byte
	var cookstylePassed *bool
	var cookstyleScannedAt string
	var cookstyleResultID string

	csResult, csErr := r.db.GetGitRepoCookstyleResult(ctx, gitRepoID, targetVersion)
	if csErr != nil {
		r.logf("ERROR", "getting cookstyle result for git repo %s target %s: %v", repoName, targetVersion, csErr)
		WriteInternalError(w, "Failed to fetch cookstyle results.")
		return
	}
	if csResult != nil {
		cookstyleOffences = csResult.Offences
		p := csResult.Passed
		cookstylePassed = &p
		cookstyleScannedAt = csResult.ScannedAt.Format("2006-01-02T15:04:05Z")
		cookstyleResultID = csResult.ID
	}

	// Fetch complexity records for summary stats.
	var complexityScore int
	var complexityLabel string
	var autoCorrectableCount int
	var manualFixCount int
	var deprecationCount int
	var errorCount int

	complexities, cxErr := r.db.ListGitRepoComplexitiesByRepo(ctx, gitRepoID)
	if cxErr != nil {
		r.logf("WARN", "listing complexity for git repo %s: %v", gitRepoID, cxErr)
	}
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

	type copRemediationResp struct {
		CopName            string `json:"cop_name"`
		Description        string `json:"description"`
		MigrationURL       string `json:"migration_url"`
		IntroducedIn       string `json:"introduced_in,omitempty"`
		RemovedIn          string `json:"removed_in,omitempty"`
		ReplacementPattern string `json:"replacement_pattern,omitempty"`
	}

	type offenseGroup struct {
		CopName          string              `json:"cop_name"`
		Severity         string              `json:"severity"`
		Count            int                 `json:"count"`
		CorrectableCount int                 `json:"correctable_count"`
		Remediation      *copRemediationResp `json:"remediation,omitempty"`
		Offenses         []offense           `json:"offenses"`
	}

	// Parse offenses from the JSONB column. The stored format is the
	// RuboCop JSON output's file-based offense list. We normalise it
	// into a flat list, then group by cop name.
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
				r.logf("WARN", "failed to parse offenses JSON for git repo %s: %v", repoName, err)
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
				g.Remediation = &copRemediationResp{
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

	if cookstyleResultID != "" {
		preview, acErr := r.db.GetGitRepoAutocorrectPreview(ctx, cookstyleResultID)
		if acErr != nil {
			r.logf("WARN", "getting git repo autocorrect preview for cookstyle result %s: %v", cookstyleResultID, acErr)
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

	WriteJSON(w, http.StatusOK, map[string]any{
		"git_repo_name":       repoName,
		"version":             repoVersion,
		"target_chef_version": targetVersion,
		"source":              "git",
		"complexity_score":    complexityScore,
		"complexity_label":    complexityLabel,
		"cookstyle_passed":    cookstylePassed,
		"scanned_at":          cookstyleScannedAt,
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
