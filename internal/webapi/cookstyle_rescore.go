// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// CookstyleRescoreStore is the subset of the datastore needed by the re-score
// logic. Defined here to allow mock-based unit testing without a full DB.
type CookstyleRescoreStore interface {
	ListServerCookstyleResultsForRescore(ctx context.Context) ([]datastore.CookstyleRescoreRow, error)
	ListGitRepoCookstyleResultsForRescore(ctx context.Context) ([]datastore.CookstyleRescoreRow, error)
	BatchUpdateServerCookstylePassed(ctx context.Context, updates []datastore.CookstylePassedUpdate) error
	BatchUpdateGitRepoCookstylePassed(ctx context.Context, updates []datastore.CookstylePassedUpdate) error
	RecomputeGitRepoCompatibilityStatus(ctx context.Context, name, url, targetVersion string) error
	// ListCopClassifications returns operator classification overrides (keyed by
	// cop_name; single active target), used to build the single-source-of-truth
	// derivation. The resolver still applies RemovedIn auto-seed + curated
	// defaults on top.
	ListCopClassifications(ctx context.Context) ([]datastore.CopClassification, error)
}

// RescoreResult reports how many results were evaluated and how many changed.
type RescoreResult struct {
	Total   int `json:"total"`
	Changed int `json:"changed"`
}

// RescoreCookstyleResults re-evaluates all stored cookstyle results against the
// given failure rules, updating the passed column for any row whose verdict has
// changed. It then triggers compatibility status recomputation for affected git
// repos. The logger is optional (nil-safe).
func RescoreCookstyleResults(ctx context.Context, store CookstyleRescoreStore, rules analysis.CookstyleFailureRules, logger func(level, msg string)) (RescoreResult, error) {
	var result RescoreResult

	// Memoise one classification resolver per target version so the override
	// load happens once per target rather than once per result. The resolver
	// applies operator overrides, RemovedIn auto-seed, and curated defaults;
	// severity-based failure rules are only the fallback for unclassified cops.
	resolverCache := map[string]*analysis.CopClassificationResolver{}
	resolverFor := func(target string) *analysis.CopClassificationResolver {
		if r, ok := resolverCache[target]; ok {
			return r
		}
		overrides := map[string]string{}
		if rows, lerr := store.ListCopClassifications(ctx); lerr == nil {
			for _, row := range rows {
				overrides[row.CopName] = row.Classification
			}
		} else {
			rescoreLogf(logger, "WARN", "rescore: loading classifications for target %q: %v", target, lerr)
		}
		r := &analysis.CopClassificationResolver{OperatorOverrides: overrides, TargetChefVersion: target}
		resolverCache[target] = r
		return r
	}

	// --- Server cookbook results ---
	serverRows, err := store.ListServerCookstyleResultsForRescore(ctx)
	if err != nil {
		return result, fmt.Errorf("rescore: listing server results: %w", err)
	}

	serverUpdates := rescoreRows(serverRows, rules, resolverFor, &result)
	if len(serverUpdates) > 0 {
		if err := store.BatchUpdateServerCookstylePassed(ctx, serverUpdates); err != nil {
			return result, fmt.Errorf("rescore: updating server results: %w", err)
		}
	}

	// --- Git repo results ---
	gitRows, err := store.ListGitRepoCookstyleResultsForRescore(ctx)
	if err != nil {
		return result, fmt.Errorf("rescore: listing git results: %w", err)
	}

	gitUpdates := rescoreRows(gitRows, rules, resolverFor, &result)
	if len(gitUpdates) > 0 {
		if err := store.BatchUpdateGitRepoCookstylePassed(ctx, gitUpdates); err != nil {
			return result, fmt.Errorf("rescore: updating git results: %w", err)
		}
		// Recompute compatibility status for each affected git repo.
		for _, u := range gitUpdates {
			name, url, targetVersion := parseGitRescoreID(u.ID)
			if err := store.RecomputeGitRepoCompatibilityStatus(ctx, name, url, targetVersion); err != nil {
				rescoreLogf(logger, "ERROR", "rescore: recomputing git repo status for %s: %v", name, err)
			}
		}
	}

	rescoreLogf(logger, "INFO", "rescore: %d results evaluated, %d verdicts changed", result.Total, result.Changed)
	return result, nil
}

// rescoreRows evaluates a slice of rescore rows against the given rules,
// collecting updates for rows whose verdict has changed. Rows with
// error_message or nil/empty offences are skipped.
func rescoreRows(rows []datastore.CookstyleRescoreRow, rules analysis.CookstyleFailureRules, resolverFor func(target string) *analysis.CopClassificationResolver, result *RescoreResult) []datastore.CookstylePassedUpdate {
	var updates []datastore.CookstylePassedUpdate

	for i := range rows {
		row := &rows[i]

		// Skip inconclusive scans
		if row.ErrorMessage != "" {
			continue
		}
		// Skip rows with no stored offences
		if len(row.Offences) == 0 {
			continue
		}

		var offenses []analysis.CookstyleOffense
		if err := json.Unmarshal(row.Offences, &offenses); err != nil {
			// Malformed JSON — skip silently
			continue
		}

		result.Total++
		// Single source of truth: classification-derived status, with the
		// severity failure rules only as the fallback for unclassified cops.
		// Compare on the full rollup status (not just passed) so a change that
		// keeps passed but moves ready↔needs_review still re-materialises.
		resolver := resolverFor(rescoreTargetFromID(row.ID))
		newStatus := analysis.DeriveCookstyleStatus(offenses, rules, resolver)
		newPassed := newStatus != analysis.StatusBlocked
		if newStatus != row.CookstyleStatus {
			result.Changed++
			updates = append(updates, datastore.CookstylePassedUpdate{
				ID:     row.ID,
				Passed: newPassed,
				Status: newStatus,
			})
		}
	}

	return updates
}

// parseGitRescoreID extracts repo name, URL, and target version from a
// pipe-delimited git rescore ID ("name|url|target_chef_version").
func parseGitRescoreID(id string) (name, url, targetVersion string) {
	parts := strings.SplitN(id, "|", 3)
	if len(parts) == 3 {
		return parts[0], parts[1], parts[2]
	}
	return id, "", ""
}

// rescoreTargetFromID extracts the target Chef version from a rescore ID. For
// both server ("org|cb|ver|target") and git ("name|url|target") IDs the target
// version is the final pipe-delimited segment.
func rescoreTargetFromID(id string) string {
	if idx := strings.LastIndex(id, "|"); idx >= 0 {
		return id[idx+1:]
	}
	return ""
}

func rescoreLogf(logger func(level, msg string), level, format string, args ...any) {
	if logger == nil {
		return
	}
	logger(level, fmt.Sprintf(format, args...))
}
