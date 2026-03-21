// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// ---------------------------------------------------------------------------
// Scoring weights — per the analysis specification § 4.3
// ---------------------------------------------------------------------------

const (
	// WeightErrorFatal is the per-offense weight for CookStyle offenses
	// with severity "error" or "fatal".
	WeightErrorFatal = 5

	// WeightDeprecation is the per-offense weight for Chef/Deprecations/*
	// offenses.
	WeightDeprecation = 3

	// WeightCorrectness is the per-offense weight for Chef/Correctness/*
	// offenses.
	WeightCorrectness = 3

	// WeightNonAutoCorrectable is the per-offense weight for offenses
	// that remain after auto-correct (manual intervention required).
	WeightNonAutoCorrectable = 4

	// WeightModernize is the per-offense weight for Chef/Modernize/*
	// offenses.
	WeightModernize = 1

	// WeightTKConvergeFail is the flat weight applied when Test Kitchen
	// converge fails.
	WeightTKConvergeFail = 20

	// WeightTKTestFail is the flat weight applied when Test Kitchen
	// converge passes but tests fail.
	WeightTKTestFail = 10
)

// ---------------------------------------------------------------------------
// Complexity labels
// ---------------------------------------------------------------------------

const (
	LabelNone     = "none"
	LabelLow      = "low"
	LabelMedium   = "medium"
	LabelHigh     = "high"
	LabelCritical = "critical"
)

// ScoreToLabel converts a numeric complexity score to its label per the
// specification:
//
//	0       → none
//	1-10    → low
//	11-30   → medium
//	31-60   → high
//	61+     → critical
func ScoreToLabel(score int) string {
	switch {
	case score <= 0:
		return LabelNone
	case score <= 10:
		return LabelLow
	case score <= 30:
		return LabelMedium
	case score <= 60:
		return LabelHigh
	default:
		return LabelCritical
	}
}

// ---------------------------------------------------------------------------
// Scoring input types
// ---------------------------------------------------------------------------

// CookstyleOffenseSummary carries the classified offense counts extracted
// from a CookStyle scan result. The caller is responsible for parsing the
// JSONB offenses column and classifying each offense.
type CookstyleOffenseSummary struct {
	// ErrorFatalCount is the number of offenses with severity "error" or "fatal".
	ErrorFatalCount int

	// DeprecationCount is the number of Chef/Deprecations/* offenses.
	DeprecationCount int

	// CorrectnessCount is the number of Chef/Correctness/* offenses.
	CorrectnessCount int

	// ModernizeCount is the number of Chef/Modernize/* offenses.
	ModernizeCount int

	// AutoCorrectableCount is the number of offenses fixable by auto-correct.
	// Sourced from the autocorrect_previews table's correctable_offenses column.
	AutoCorrectableCount int

	// ManualFixCount is the number of offenses requiring manual intervention.
	// Sourced from the autocorrect_previews table's remaining_offenses column.
	ManualFixCount int
}

// TestKitchenSummary carries the test outcome for a single cookbook ×
// target version from the Test Kitchen results table.
type TestKitchenSummary struct {
	// HasResult is true if a Test Kitchen result exists for this cookbook
	// and target version.
	HasResult bool

	// ConvergePassed is true if the converge phase succeeded.
	ConvergePassed bool

	// TestsPassed is true if the verify phase succeeded.
	TestsPassed bool
}

// BlastRadius carries the impact metrics for a single cookbook.
type BlastRadius struct {
	// AffectedNodeCount is the number of nodes running this cookbook.
	AffectedNodeCount int

	// AffectedRoleCount is the number of roles that include this cookbook
	// (directly or transitively via the role dependency graph).
	AffectedRoleCount int

	// AffectedPolicyCount is the number of Policyfile policy names that
	// include this cookbook.
	AffectedPolicyCount int
}

// ComplexityInput gathers all the data needed to compute a single
// complexity score for one cookbook × target Chef version.
type ComplexityInput struct {
	CookbookID        string
	CookbookName      string
	CookbookVersion   string
	TargetChefVersion string

	Cookstyle   CookstyleOffenseSummary
	TestKitchen TestKitchenSummary
	Blast       BlastRadius
}

// ---------------------------------------------------------------------------
// Score computation (pure function — no side effects)
// ---------------------------------------------------------------------------

// ComputeComplexityScore calculates the weighted complexity score from the
// input data. This is a pure function with no side effects and is safe to
// call from tests.
func ComputeComplexityScore(input ComplexityInput) int {
	score := 0

	// CookStyle offense weights.
	score += input.Cookstyle.ErrorFatalCount * WeightErrorFatal
	score += input.Cookstyle.DeprecationCount * WeightDeprecation
	score += input.Cookstyle.CorrectnessCount * WeightCorrectness
	score += input.Cookstyle.ManualFixCount * WeightNonAutoCorrectable
	score += input.Cookstyle.ModernizeCount * WeightModernize

	// Test Kitchen weights.
	if input.TestKitchen.HasResult {
		if !input.TestKitchen.ConvergePassed {
			score += WeightTKConvergeFail
		} else if !input.TestKitchen.TestsPassed {
			score += WeightTKTestFail
		}
	}

	return score
}

// ---------------------------------------------------------------------------
// ComplexityResult is the output of scoring a single cookbook × target version.
// ---------------------------------------------------------------------------

// ComplexityResult holds the computed complexity score, label, breakdown
// counts, and blast radius for a single cookbook × target Chef version.
type ComplexityResult struct {
	CookbookID        string
	CookbookName      string
	CookbookVersion   string
	TargetChefVersion string

	ComplexityScore int
	ComplexityLabel string

	ErrorCount           int
	DeprecationCount     int
	CorrectnessCount     int
	ModernizeCount       int
	AutoCorrectableCount int
	ManualFixCount       int

	AffectedNodeCount   int
	AffectedRoleCount   int
	AffectedPolicyCount int

	EvaluatedAt time.Time

	// Error is non-nil when scoring failed (e.g. data retrieval error).
	Error error
}

// ---------------------------------------------------------------------------
// Complexity scorer
// ---------------------------------------------------------------------------

// ComplexityScorer computes and persists cookbook complexity scores. It
// reads CookStyle results, auto-correct previews, Test Kitchen results,
// and usage/role data from the datastore, computes a weighted score, and
// persists the result to the appropriate complexity table.
type ComplexityScorer struct {
	db     *datastore.DB
	logger *logging.Logger
}

// NewComplexityScorer creates a new scorer.
func NewComplexityScorer(db *datastore.DB, logger *logging.Logger) *ComplexityScorer {
	return &ComplexityScorer{
		db:     db,
		logger: logger,
	}
}

// ---------------------------------------------------------------------------
// Batch scoring
// ---------------------------------------------------------------------------

// ComplexityBatchResult summarises the outcome of scoring a batch of
// cookbooks.
type ComplexityBatchResult struct {
	Total    int
	Scored   int
	Skipped  int
	Errors   int
	Duration time.Duration
	Results  []ComplexityResult
}

// ScoreServerCookbooks computes complexity scores for all provided server
// cookbooks against all provided target Chef versions. For each combination
// it:
//
//  1. Loads the CookStyle scan result and classifies offenses.
//  2. Loads the auto-correct preview (if any) for manual fix counts.
//  3. Computes the blast radius from usage analysis and role dependencies.
//  4. Computes the weighted score and label.
//  5. Persists to the server_cookbook_complexity table.
func (s *ComplexityScorer) ScoreServerCookbooks(
	ctx context.Context,
	cookbooks []datastore.ServerCookbook,
	targetChefVersions []string,
	organisationID string,
) ComplexityBatchResult {
	start := time.Now()
	log := s.logger.WithScope(logging.ScopeRemediation)

	// Pre-load blast radius data for the organisation.
	blastRadii, blastErr := s.loadBlastRadii(ctx, organisationID)
	if blastErr != nil {
		log.Error(fmt.Sprintf("failed to load blast radius data: %v", blastErr))
		// Continue with empty blast radii — scoring still works, just without
		// impact metrics.
		if blastRadii == nil {
			blastRadii = make(map[string]BlastRadius)
		}
	}

	// Build work items.
	type workItem struct {
		Cookbook      datastore.ServerCookbook
		TargetVersion string
	}

	var items []workItem
	for _, cb := range cookbooks {
		for _, tv := range targetChefVersions {
			items = append(items, workItem{Cookbook: cb, TargetVersion: tv})
		}
	}

	batch := ComplexityBatchResult{
		Total:   len(items),
		Results: make([]ComplexityResult, 0, len(items)),
	}

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}

		result := s.scoreOneServerCookbook(ctx, item.Cookbook, item.TargetVersion, blastRadii)
		batch.Results = append(batch.Results, result)

		switch {
		case result.Error != nil:
			batch.Errors++
			log.Error(fmt.Sprintf("complexity scoring error: %s/%s target %s: %v",
				result.CookbookName, result.CookbookVersion, result.TargetChefVersion, result.Error))
		default:
			batch.Scored++
		}
	}

	batch.Duration = time.Since(start)
	log.Info(fmt.Sprintf(
		"server cookbook complexity scoring complete: %d total, %d scored, %d skipped, %d errors in %s",
		batch.Total, batch.Scored, batch.Skipped, batch.Errors,
		batch.Duration.Round(time.Millisecond)))
	return batch
}

// ScoreGitRepos computes complexity scores for all provided git repos
// against all provided target Chef versions. For each combination it:
//
//  1. Loads the CookStyle scan result and classifies offenses.
//  2. Loads the auto-correct preview (if any) for manual fix counts.
//  3. Loads the Test Kitchen result (if any).
//  4. Computes the blast radius from usage analysis and role dependencies.
//  5. Computes the weighted score and label.
//  6. Persists to the git_repo_complexity table.
func (s *ComplexityScorer) ScoreGitRepos(
	ctx context.Context,
	repos []datastore.GitRepo,
	targetChefVersions []string,
	organisationID string,
) ComplexityBatchResult {
	start := time.Now()
	log := s.logger.WithScope(logging.ScopeRemediation)

	// Pre-load blast radius data for the organisation.
	blastRadii, blastErr := s.loadBlastRadii(ctx, organisationID)
	if blastErr != nil {
		log.Error(fmt.Sprintf("failed to load blast radius data: %v", blastErr))
		if blastRadii == nil {
			blastRadii = make(map[string]BlastRadius)
		}
	}

	// Build work items.
	type workItem struct {
		Repo          datastore.GitRepo
		TargetVersion string
	}

	var items []workItem
	for _, repo := range repos {
		for _, tv := range targetChefVersions {
			items = append(items, workItem{Repo: repo, TargetVersion: tv})
		}
	}

	batch := ComplexityBatchResult{
		Total:   len(items),
		Results: make([]ComplexityResult, 0, len(items)),
	}

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}

		result := s.scoreOneGitRepo(ctx, item.Repo, item.TargetVersion, blastRadii)
		batch.Results = append(batch.Results, result)

		switch {
		case result.Error != nil:
			batch.Errors++
			log.Error(fmt.Sprintf("complexity scoring error: %s target %s: %v",
				result.CookbookName, result.TargetChefVersion, result.Error))
		default:
			batch.Scored++
		}
	}

	batch.Duration = time.Since(start)
	log.Info(fmt.Sprintf(
		"git repo complexity scoring complete: %d total, %d scored, %d skipped, %d errors in %s",
		batch.Total, batch.Scored, batch.Skipped, batch.Errors,
		batch.Duration.Round(time.Millisecond)))
	return batch
}

// ---------------------------------------------------------------------------
// Per-item scoring
// ---------------------------------------------------------------------------

// scoreOneServerCookbook computes the complexity score for a single server
// cookbook × target Chef version.
func (s *ComplexityScorer) scoreOneServerCookbook(
	ctx context.Context,
	cb datastore.ServerCookbook,
	targetChefVersion string,
	blastRadii map[string]BlastRadius,
) ComplexityResult {
	result := ComplexityResult{
		CookbookID:        cb.ID,
		CookbookName:      cb.Name,
		CookbookVersion:   cb.Version,
		TargetChefVersion: targetChefVersion,
	}

	// Step 1: Load CookStyle result.
	csResult, csErr := s.db.GetServerCookbookCookstyleResult(ctx, cb.ID, targetChefVersion)
	if csErr != nil {
		result.Error = fmt.Errorf("loading cookstyle result: %w", csErr)
		return result
	}

	var offenseSummary CookstyleOffenseSummary
	if csResult != nil {
		offenseSummary = classifyOffenses(csResult.Offences, csResult.DeprecationCount, csResult.CorrectnessCount)
	}

	// Step 2: Load auto-correct preview for manual fix count.
	if csResult != nil {
		preview, previewErr := s.db.GetServerCookbookAutocorrectPreview(ctx, csResult.ID)
		if previewErr == nil && preview != nil {
			offenseSummary.AutoCorrectableCount = preview.CorrectableOffenses
			offenseSummary.ManualFixCount = preview.RemainingOffenses
		}
	}

	// Step 3: Look up blast radius. Server cookbooks do not have Test
	// Kitchen results — that is a git-repo-only concept.
	blast := blastRadii[cb.Name]

	// Step 4: Compute score.
	input := ComplexityInput{
		CookbookID:        cb.ID,
		CookbookName:      cb.Name,
		CookbookVersion:   cb.Version,
		TargetChefVersion: targetChefVersion,
		Cookstyle:         offenseSummary,
		Blast:             blast,
	}

	score := ComputeComplexityScore(input)
	label := ScoreToLabel(score)

	result.ComplexityScore = score
	result.ComplexityLabel = label
	result.ErrorCount = offenseSummary.ErrorFatalCount
	result.DeprecationCount = offenseSummary.DeprecationCount
	result.CorrectnessCount = offenseSummary.CorrectnessCount
	result.ModernizeCount = offenseSummary.ModernizeCount
	result.AutoCorrectableCount = offenseSummary.AutoCorrectableCount
	result.ManualFixCount = offenseSummary.ManualFixCount
	result.AffectedNodeCount = blast.AffectedNodeCount
	result.AffectedRoleCount = blast.AffectedRoleCount
	result.AffectedPolicyCount = blast.AffectedPolicyCount
	result.EvaluatedAt = time.Now().UTC()

	// Step 5: Persist.
	s.persistServerCookbookComplexity(ctx, result)

	return result
}

// scoreOneGitRepo computes the complexity score for a single git repo ×
// target Chef version.
func (s *ComplexityScorer) scoreOneGitRepo(
	ctx context.Context,
	repo datastore.GitRepo,
	targetChefVersion string,
	blastRadii map[string]BlastRadius,
) ComplexityResult {
	result := ComplexityResult{
		CookbookID:        repo.ID,
		CookbookName:      repo.Name,
		TargetChefVersion: targetChefVersion,
	}

	// Step 1: Load CookStyle result.
	csResult, csErr := s.db.GetGitRepoCookstyleResult(ctx, repo.ID, targetChefVersion)
	if csErr != nil {
		result.Error = fmt.Errorf("loading cookstyle result: %w", csErr)
		return result
	}

	var offenseSummary CookstyleOffenseSummary
	if csResult != nil {
		offenseSummary = classifyOffenses(csResult.Offences, csResult.DeprecationCount, csResult.CorrectnessCount)
	}

	// Step 2: Load auto-correct preview for manual fix count.
	if csResult != nil {
		preview, previewErr := s.db.GetGitRepoAutocorrectPreview(ctx, csResult.ID)
		if previewErr == nil && preview != nil {
			offenseSummary.AutoCorrectableCount = preview.CorrectableOffenses
			offenseSummary.ManualFixCount = preview.RemainingOffenses
		}
	}

	// Step 3: Load Test Kitchen result (git repos only).
	var tkSummary TestKitchenSummary
	tkResult, tkErr := s.db.GetLatestGitRepoTestKitchenResult(ctx, repo.ID, targetChefVersion)
	if tkErr == nil && tkResult != nil {
		tkSummary.HasResult = true
		tkSummary.ConvergePassed = tkResult.ConvergePassed
		tkSummary.TestsPassed = tkResult.TestsPassed
	}

	// Step 4: Look up blast radius.
	blast := blastRadii[repo.Name]

	// Step 5: Compute score.
	input := ComplexityInput{
		CookbookID:        repo.ID,
		CookbookName:      repo.Name,
		TargetChefVersion: targetChefVersion,
		Cookstyle:         offenseSummary,
		TestKitchen:       tkSummary,
		Blast:             blast,
	}

	score := ComputeComplexityScore(input)
	label := ScoreToLabel(score)

	result.ComplexityScore = score
	result.ComplexityLabel = label
	result.ErrorCount = offenseSummary.ErrorFatalCount
	result.DeprecationCount = offenseSummary.DeprecationCount
	result.CorrectnessCount = offenseSummary.CorrectnessCount
	result.ModernizeCount = offenseSummary.ModernizeCount
	result.AutoCorrectableCount = offenseSummary.AutoCorrectableCount
	result.ManualFixCount = offenseSummary.ManualFixCount
	result.AffectedNodeCount = blast.AffectedNodeCount
	result.AffectedRoleCount = blast.AffectedRoleCount
	result.AffectedPolicyCount = blast.AffectedPolicyCount
	result.EvaluatedAt = time.Now().UTC()

	// Step 6: Persist.
	s.persistGitRepoComplexity(ctx, result)

	return result
}

// ---------------------------------------------------------------------------
// Offense classification
// ---------------------------------------------------------------------------

// storedOffense is the minimal subset of the JSONB offense record needed
// for classification. The offences column in cookstyle_results stores
// the full CookStyle offense JSON array.
type storedOffense struct {
	CopName   string `json:"cop_name"`
	Severity  string `json:"severity"`
	Corrected bool   `json:"corrected"`
}

// classifyOffenses parses the JSONB offenses byte slice and counts offenses
// by category. The fallbackDeprecationCount and fallbackCorrectnessCount are
// used when the JSONB cannot be parsed — they should come from the
// pre-aggregated counts on the cookstyle result row.
func classifyOffenses(offencesJSON []byte, fallbackDeprecationCount, fallbackCorrectnessCount int) CookstyleOffenseSummary {
	var summary CookstyleOffenseSummary

	if len(offencesJSON) == 0 {
		return summary
	}

	var offenses []storedOffense
	if err := json.Unmarshal(offencesJSON, &offenses); err != nil {
		// If we can't parse the JSONB, fall back to the pre-aggregated
		// counts already on the result row.
		summary.DeprecationCount = fallbackDeprecationCount
		summary.CorrectnessCount = fallbackCorrectnessCount
		return summary
	}

	for _, off := range offenses {
		if isErrorOrFatal(off.Severity) {
			summary.ErrorFatalCount++
		}
		if isDeprecation(off.CopName) {
			summary.DeprecationCount++
		}
		if isCorrectness(off.CopName) {
			summary.CorrectnessCount++
		}
		if isModernize(off.CopName) {
			summary.ModernizeCount++
		}
	}

	return summary
}

// Cop namespace prefixes — mirror the constants in analysis/cookstyle.go.
const (
	nsDeprecations = "Chef/Deprecations/"
	nsCorrectness  = "Chef/Correctness/"
	nsModernize    = "Chef/Modernize/"
)

func isDeprecation(copName string) bool { return strings.HasPrefix(copName, nsDeprecations) }
func isCorrectness(copName string) bool { return strings.HasPrefix(copName, nsCorrectness) }
func isModernize(copName string) bool   { return strings.HasPrefix(copName, nsModernize) }
func isErrorOrFatal(severity string) bool {
	return severity == "error" || severity == "fatal"
}

// ---------------------------------------------------------------------------
// Blast radius computation
// ---------------------------------------------------------------------------

// loadBlastRadii computes the blast radius for every cookbook in the given
// organisation. It combines data from:
//   - cookbook_usage_detail (latest analysis) for node counts and policy counts
//   - role_dependencies for role counts
//
// Returns a map keyed by cookbook name (not ID, because usage analysis
// stores cookbook_name and role dependencies store dependency_name).
func (s *ComplexityScorer) loadBlastRadii(ctx context.Context, organisationID string) (map[string]BlastRadius, error) {
	radii := make(map[string]BlastRadius)

	// 1. Get node and policy counts from the latest usage analysis.
	latestAnalysis, err := s.db.GetLatestCookbookUsageAnalysis(ctx, organisationID)
	if err == nil && latestAnalysis.ID != "" {
		summaries, sumErr := s.db.ListCookbookUsageSummaries(ctx, latestAnalysis.ID)
		if sumErr == nil {
			for _, d := range summaries {
				r := radii[d.CookbookName]
				// Accumulate across versions — blast radius is per-cookbook-name,
				// not per-version.
				r.AffectedNodeCount += d.NodeCount

				// Parse policy_names JSONB to count distinct policies.
				policyCount := countJSONBStringArray(d.PolicyNames)
				r.AffectedPolicyCount += policyCount

				radii[d.CookbookName] = r
			}

			// De-duplicate: for cookbooks with multiple versions, node counts
			// are already per-version (each node runs exactly one version),
			// so summing is correct. But policy counts might overlap across
			// versions; for simplicity we accept the slight over-count here
			// because it's a ballpark metric.
		}
	}

	// 2. Get role counts from role_dependencies.
	roleCounts, roleErr := s.db.CountRolesPerCookbook(ctx, organisationID)
	if roleErr == nil {
		for _, rc := range roleCounts {
			r := radii[rc.CookbookName]
			r.AffectedRoleCount = rc.RoleCount
			radii[rc.CookbookName] = r
		}
	}

	return radii, nil
}

// countJSONBStringArray parses a JSONB byte slice as a JSON array of strings
// and returns its length. Returns 0 on any parse failure.
func countJSONBStringArray(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return 0
	}
	return len(arr)
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (s *ComplexityScorer) persistServerCookbookComplexity(ctx context.Context, result ComplexityResult) {
	if result.CookbookID == "" || result.TargetChefVersion == "" {
		return
	}

	log := s.logger.WithScope(logging.ScopeRemediation,
		logging.WithCookbook(result.CookbookName, result.CookbookVersion))

	params := datastore.UpsertServerCookbookComplexityParams{
		ServerCookbookID:     result.CookbookID,
		TargetChefVersion:    result.TargetChefVersion,
		ComplexityScore:      result.ComplexityScore,
		ComplexityLabel:      result.ComplexityLabel,
		ErrorCount:           result.ErrorCount,
		DeprecationCount:     result.DeprecationCount,
		CorrectnessCount:     result.CorrectnessCount,
		ModernizeCount:       result.ModernizeCount,
		AutoCorrectableCount: result.AutoCorrectableCount,
		ManualFixCount:       result.ManualFixCount,
		AffectedNodeCount:    result.AffectedNodeCount,
		AffectedRoleCount:    result.AffectedRoleCount,
		AffectedPolicyCount:  result.AffectedPolicyCount,
		EvaluatedAt:          result.EvaluatedAt,
	}

	if _, persistErr := s.db.UpsertServerCookbookComplexity(ctx, params); persistErr != nil {
		log.Error(fmt.Sprintf("failed to persist server cookbook complexity score: %v", persistErr))
	}
}

func (s *ComplexityScorer) persistGitRepoComplexity(ctx context.Context, result ComplexityResult) {
	if result.CookbookID == "" || result.TargetChefVersion == "" {
		return
	}

	log := s.logger.WithScope(logging.ScopeRemediation,
		logging.WithCookbook(result.CookbookName, ""))

	params := datastore.UpsertGitRepoComplexityParams{
		GitRepoID:            result.CookbookID,
		TargetChefVersion:    result.TargetChefVersion,
		ComplexityScore:      result.ComplexityScore,
		ComplexityLabel:      result.ComplexityLabel,
		ErrorCount:           result.ErrorCount,
		DeprecationCount:     result.DeprecationCount,
		CorrectnessCount:     result.CorrectnessCount,
		ModernizeCount:       result.ModernizeCount,
		AutoCorrectableCount: result.AutoCorrectableCount,
		ManualFixCount:       result.ManualFixCount,
		AffectedNodeCount:    result.AffectedNodeCount,
		AffectedRoleCount:    result.AffectedRoleCount,
		AffectedPolicyCount:  result.AffectedPolicyCount,
		EvaluatedAt:          result.EvaluatedAt,
	}

	if _, persistErr := s.db.UpsertGitRepoComplexity(ctx, params); persistErr != nil {
		log.Error(fmt.Sprintf("failed to persist git repo complexity score: %v", persistErr))
	}
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

// ResetServerCookbookScores deletes existing complexity scores for the given
// server cookbook, so they will be recomputed on the next analysis cycle.
func (s *ComplexityScorer) ResetServerCookbookScores(ctx context.Context, serverCookbookID string) error {
	return s.db.DeleteServerCookbookComplexitiesByCookbook(ctx, serverCookbookID)
}

// ResetGitRepoScores deletes existing complexity scores for the given git
// repo, so they will be recomputed on the next analysis cycle.
func (s *ComplexityScorer) ResetGitRepoScores(ctx context.Context, gitRepoID string) error {
	return s.db.DeleteGitRepoComplexitiesByRepo(ctx, gitRepoID)
}

// ResetAllScores deletes all complexity scores from both the
// server_cookbook_complexity and git_repo_complexity tables.
func (s *ComplexityScorer) ResetAllScores(ctx context.Context) error {
	if err := s.db.DeleteAllServerCookbookComplexities(ctx); err != nil {
		return fmt.Errorf("deleting all server cookbook complexities: %w", err)
	}
	if err := s.db.DeleteAllGitRepoComplexities(ctx); err != nil {
		return fmt.Errorf("deleting all git repo complexities: %w", err)
	}
	return nil
}
