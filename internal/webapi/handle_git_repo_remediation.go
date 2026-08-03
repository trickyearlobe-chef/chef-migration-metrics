// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
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
	if targetVersion == "" {
		targetVersion = r.defaultTargetVersion()
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

	gitRepoURL := gitRepos[0].GitRepoURL

	// Fetch cookstyle result.
	var cookstyleOffences []byte
	var cookstylePassed *bool
	cookstyleStatus := "untested" // SoT rollup; stays untested when no result exists
	var cookstyleScannedAt string
	var hasCookstyleResult bool

	csResult, csErr := r.db.GetGitRepoCookstyleResult(ctx, repoName, gitRepoURL, targetVersion)
	if csErr != nil {
		r.logf("ERROR", "getting cookstyle result for git repo %s target %s: %v", repoName, targetVersion, csErr)
		WriteInternalError(w, "Failed to fetch cookstyle results.")
		return
	}
	if csResult != nil {
		cookstyleOffences = csResult.Offences
		p := csResult.Passed
		cookstylePassed = &p
		if csResult.CookstyleStatus != "" {
			cookstyleStatus = csResult.CookstyleStatus
		}
		cookstyleScannedAt = csResult.ScannedAt.Format("2006-01-02T15:04:05Z")
		hasCookstyleResult = true
	}

	// Fetch complexity records for summary stats.
	var complexityScore int
	var complexityLabel string
	var autoCorrectableCount int
	var manualFixCount int
	var deprecationCount int
	var correctnessCount int
	var modernizeCount int
	var errorCount int

	complexities, cxErr := r.db.ListGitRepoComplexitiesByRepo(ctx, repoName, gitRepoURL)
	if cxErr != nil {
		r.logf("WARN", "listing complexity for git repo %s: %v", repoName, cxErr)
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
		// GroupKey uniquely identifies a group. For an ordinary cop it equals
		// CopName; for a poly-method cop it is CopName plus the message-selected
		// variant token, so a Blocker variant and a Review variant of the same cop
		// form distinct groups (and land in different classification sections). The
		// frontend keys React elements and collapse state on this, not cop_name.
		GroupKey             string              `json:"group_key"`
		CopName              string              `json:"cop_name"`
		Severity             string              `json:"severity"`
		Classification       string              `json:"classification"`
		ClassificationSource string              `json:"classification_source"`
		RemovedIn            string              `json:"removed_in,omitempty"`
		Count                int                 `json:"count"`
		CorrectableCount     int                 `json:"correctable_count"`
		Remediation          *copRemediationResp `json:"remediation,omitempty"`
		Offenses             []offense           `json:"offenses"`
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

	// Build classification resolver for the target version.
	overrides, classErr := r.db.ListCopClassifications(ctx)
	if classErr != nil {
		r.logf("WARN", "listing cop classifications for git repo remediation: %v", classErr)
	}
	overrideMap := make(map[string]string, len(overrides))
	for _, o := range overrides {
		overrideMap[o.CopName] = o.Classification
	}
	resolver := &analysis.CopClassificationResolver{
		OperatorOverrides: overrideMap,
		TargetChefVersion: targetVersion,
	}

	// Group offenses by effective key: cop name, plus the message-selected variant
	// token for poly-method cops (one cop_name flagging several deprecations of
	// differing impact). This keeps each group single-classification, so a Blocker
	// variant and a Review variant of the same cop section separately. Resolution
	// and remediation are message-aware (see specifications/cop-classification.md).
	groupOrder := make([]string, 0)
	groupMap := make(map[string]*offenseGroup)
	for _, o := range flatOffenses {
		groupKey := o.CopName
		if tok := remediation.OffenseVariantToken(o.CopName, o.Message); tok != "" {
			groupKey = o.CopName + "#" + tok
		}
		g, ok := groupMap[groupKey]
		if !ok {
			resolved := resolver.ResolveOffense(o.CopName, o.Message)
			g = &offenseGroup{
				GroupKey:             groupKey,
				CopName:              o.CopName,
				Severity:             o.Severity,
				Classification:       resolved.Classification,
				ClassificationSource: resolved.Source,
			}
			// Look up remediation guidance, message-aware for poly-method cops.
			if cm := remediation.LookupCopForOffense(o.CopName, o.Message); cm != nil {
				g.Remediation = &copRemediationResp{
					CopName:            cm.CopName,
					Description:        cm.Description,
					MigrationURL:       cm.MigrationURL,
					IntroducedIn:       cm.IntroducedIn,
					RemovedIn:          cm.RemovedIn,
					ReplacementPattern: cm.ReplacementPattern,
				}
				g.RemovedIn = cm.RemovedIn
			}
			groupMap[groupKey] = g
			groupOrder = append(groupOrder, groupKey)
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
	var blockerCount, reviewCount, noiseCount, unclassifiedCount int
	for _, groupKey := range groupOrder {
		g := *groupMap[groupKey]
		groups = append(groups, g)
		switch g.Classification {
		case analysis.ClassificationBlocker:
			blockerCount++
		case analysis.ClassificationReview:
			reviewCount++
		case analysis.ClassificationNoise:
			noiseCount++
		default:
			unclassifiedCount++
		}
	}

	classificationSummary := map[string]int{
		"blocker":      blockerCount,
		"review":       reviewCount,
		"noise":        noiseCount,
		"unclassified": unclassifiedCount,
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
		preview, acErr := r.db.GetGitRepoAutocorrectPreview(ctx, repoName, gitRepoURL, targetVersion)
		if acErr != nil {
			r.logf("WARN", "getting git repo autocorrect preview for %s target %s: %v", repoName, targetVersion, acErr)
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

	// Build complexity breakdown.
	var tkStatusStr string
	var tkWeight int
	tkResults, tkErr := r.db.ListGitKitchenResultsByRepo(ctx, repoName)
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
		tkStatusStr = tkstatus.ComputeTKStatus(passed, failed)
	}
	switch tkStatusStr {
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
			Status:   tkStatusStr,
			Weight:   tkWeight,
			Subtotal: tkWeight,
		},
	}

	// "Won't parse — fix first": a data-quality flag carried alongside the
	// rollup status, derived from any fatal (parse-failure) offense. Not a
	// classification blocker.
	cookstyleWontParse := false
	for i := range flatOffenses {
		if flatOffenses[i].Severity == analysis.SeverityFatal {
			cookstyleWontParse = true
			break
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"git_repo_name":        repoName,
		"version":              repoVersion,
		"target_chef_version":  targetVersion,
		"source":               "git",
		"complexity_score":     complexityScore,
		"complexity_label":     complexityLabel,
		"complexity_breakdown": breakdown,
		"cookstyle_passed":     cookstylePassed,
		"cookstyle_status":     cookstyleStatus,
		"cookstyle_wont_parse": cookstyleWontParse,
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
		"offense_groups":         groups,
		"classification_summary": classificationSummary,
		"autocorrect_preview":    acPreview,
	})
}
