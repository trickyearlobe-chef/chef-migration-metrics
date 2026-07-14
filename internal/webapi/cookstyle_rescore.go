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
	// RecomputeAllGitRepoCookstyleStatus re-materialises every git repo's
	// cookstyle/compatibility status from its latest result for the target, so
	// the materialised list columns cannot drift from the results (which the
	// dashboard summary reads directly).
	RecomputeAllGitRepoCookstyleStatus(ctx context.Context, targetChefVersion string) error
	// RecomputeAllRoleCompatStatus re-materialises every role's compatibility
	// columns in role_summary from the rescored cookstyle results, so the roles
	// list cannot drift from the results after a rules change.
	RecomputeAllRoleCompatStatus(ctx context.Context, targetChefVersion string) error
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
// current classification, updating the passed column for any row whose verdict
// has changed. It then triggers compatibility status recomputation for affected
// git repos. The logger is optional (nil-safe).
func RescoreCookstyleResults(ctx context.Context, store CookstyleRescoreStore, logger func(level, msg string)) (RescoreResult, error) {
	var result RescoreResult

	// Memoise one classification resolver per target version so the override
	// load happens once per target rather than once per result. The resolver
	// applies operator overrides, RemovedIn auto-seed, and curated defaults.
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

	serverUpdates := rescoreRows(serverRows, resolverFor, &result)
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

	gitUpdates := rescoreRows(gitRows, resolverFor, &result)
	if len(gitUpdates) > 0 {
		if err := store.BatchUpdateGitRepoCookstylePassed(ctx, gitUpdates); err != nil {
			return result, fmt.Errorf("rescore: updating git results: %w", err)
		}
	}

	// Re-materialise every git repo's status from its latest result —
	// unconditionally, and for every target present in the results, not just the
	// changed ones. A repo whose result status did not change this pass but whose
	// materialised column is stale (e.g. blanked by a prior target-version reset)
	// is only healed here; gating on gitUpdates would leave it drifted, which is
	// what made the Git Repos list disagree with the dashboard summary.
	for _, tv := range distinctRescoreTargets(gitRows) {
		if err := store.RecomputeAllGitRepoCookstyleStatus(ctx, tv); err != nil {
			rescoreLogf(logger, "ERROR", "rescore: recomputing git repo status for target %s: %v", tv, err)
		}
		// Roles derive from cookstyle results too; re-materialise their compat
		// columns for the same targets so the roles list cannot drift.
		if err := store.RecomputeAllRoleCompatStatus(ctx, tv); err != nil {
			rescoreLogf(logger, "ERROR", "rescore: recomputing role compat for target %s: %v", tv, err)
		}
	}

	rescoreLogf(logger, "INFO", "rescore: %d results evaluated, %d verdicts changed", result.Total, result.Changed)
	return result, nil
}

// rescoreRows evaluates a slice of rescore rows against the current
// classification, collecting updates for rows whose verdict has changed. Rows
// with error_message or nil/empty offences are skipped.
func rescoreRows(rows []datastore.CookstyleRescoreRow, resolverFor func(target string) *analysis.CopClassificationResolver, result *RescoreResult) []datastore.CookstylePassedUpdate {
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
		// Single source of truth: classification-derived status. Compare on the
		// full rollup status (not just passed) so a change that keeps passed but
		// moves ready↔needs_review still re-materialises.
		resolver := resolverFor(rescoreTargetFromID(row.ID))
		newStatus := analysis.DeriveCookstyleStatus(offenses, resolver)
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

// distinctRescoreTargets returns the unique target Chef versions referenced by a
// set of git rescore rows (the target is the final pipe-delimited segment of the
// row ID). Used to drive the bulk git-repo status re-materialisation per target.
func distinctRescoreTargets(rows []datastore.CookstyleRescoreRow) []string {
	seen := map[string]bool{}
	var targets []string
	for i := range rows {
		tv := rescoreTargetFromID(rows[i].ID)
		if !seen[tv] {
			seen[tv] = true
			targets = append(targets, tv)
		}
	}
	return targets
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
