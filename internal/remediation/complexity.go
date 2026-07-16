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
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
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

	// WeightTKFail is the flat weight applied when the aggregate Test
	// Kitchen status is "failed" (all instances failed).
	WeightTKFail = 20

	// WeightTKPartial is the flat weight applied when the aggregate Test
	// Kitchen status is "partial" (some instances passed, some failed).
	WeightTKPartial = 10
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

// TKStatus carries the aggregate Test Kitchen outcome for a single
// cookbook × target version. Values align with tkstatus.ComputeTKStatus:
// "passed", "failed", "partial", or "" (no data).
type TKStatus struct {
	// Status is the aggregate TK outcome: "passed", "failed", "partial",
	// or "" (not tested).
	Status string
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
	OrganisationName  string
	CookbookName      string
	CookbookVersion   string
	GitRepoURL        string
	TargetChefVersion string

	Cookstyle   CookstyleOffenseSummary
	TestKitchen TKStatus
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

	// Test Kitchen weight — aligned with tkstatus model.
	score += tkWeight(input.TestKitchen.Status)

	return score
}

// ---------------------------------------------------------------------------
// ComplexityResult is the output of scoring a single cookbook × target version.
// ---------------------------------------------------------------------------

// ComplexityResult holds the computed complexity score, label, breakdown
// counts, and blast radius for a single cookbook × target Chef version.
type ComplexityResult struct {
	OrganisationName  string
	CookbookName      string
	CookbookVersion   string
	GitRepoURL        string
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

	// Skipped is true when scoring was skipped because no scan results
	// (CookStyle, Test Kitchen) exist yet. A complexity record is NOT
	// persisted for skipped items, so the cookbook remains "untested"
	// rather than appearing "compatible" with zero offenses.
	Skipped bool

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

	// classifierFor returns the cop classifier to use for a given target Chef
	// version, or nil to fall back to legacy severity-based scoring. It is read
	// once per target at the start of each scoring batch (so a reclassification
	// or override change takes effect on the next run without a restart), then
	// memoised for that batch.
	classifierFor func(ctx context.Context, targetChefVersion string) CopClassifier
}

// NewComplexityScorer creates a new scorer.
func NewComplexityScorer(db *datastore.DB, logger *logging.Logger) *ComplexityScorer {
	return &ComplexityScorer{
		db:     db,
		logger: logger,
	}
}

// SetClassifierProvider wires a per-target-version cop classifier so complexity
// scoring becomes classification-weighted (the single source of truth). When
// unset, scoring falls back to the legacy severity-based aggregate weights.
func (s *ComplexityScorer) SetClassifierProvider(fn func(ctx context.Context, targetChefVersion string) CopClassifier) {
	s.classifierFor = fn
}

// classifierCache resolves one classifier per target version up front, so the
// underlying override load happens once per batch rather than once per scored
// item. Returns an empty map when no provider is wired.
func (s *ComplexityScorer) classifierCache(ctx context.Context, targets []string) map[string]CopClassifier {
	cache := make(map[string]CopClassifier, len(targets))
	if s.classifierFor == nil {
		return cache
	}
	for _, t := range targets {
		cache[t] = s.classifierFor(ctx, t)
	}
	return cache
}

// cookstyleScore returns the CookStyle+TK complexity contribution. When a
// classifier is supplied it uses the classification-weighted derivation (each
// offense once); otherwise it returns the legacy severity-based score.
func (s *ComplexityScorer) cookstyleScore(classifier CopClassifier, offencesJSON []byte, input ComplexityInput) int {
	if classifier != nil {
		classified := classifyOffensesForComplexity(offencesJSON, classifier)
		return ComputeCookstyleComplexity(classified) + tkWeight(input.TestKitchen.Status)
	}
	return ComputeComplexityScore(input)
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
// cookbooks against the active target Chef version. For each cookbook it:
//
//  1. Loads the CookStyle scan result and classifies offenses.
//  2. Loads the auto-correct preview (if any) for manual fix counts.
//  3. Computes the blast radius from usage analysis and role dependencies.
//  4. Computes the weighted score and label.
//  5. Persists to the server_cookbook_complexity table.
func (s *ComplexityScorer) ScoreServerCookbooks(
	ctx context.Context,
	cookbooks []datastore.ServerCookbook,
	targetChefVersion string,
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
		items = append(items, workItem{Cookbook: cb, TargetVersion: targetChefVersion})
	}

	classifiers := s.classifierCache(ctx, []string{targetChefVersion})

	batch := ComplexityBatchResult{
		Total:   len(items),
		Results: make([]ComplexityResult, 0, len(items)),
	}

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}

		result := s.scoreOneServerCookbook(ctx, item.Cookbook, item.TargetVersion, blastRadii, classifiers[item.TargetVersion])
		batch.Results = append(batch.Results, result)

		switch {
		case result.Skipped:
			batch.Skipped++
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
// against the active target Chef version. For each repo it:
//
//  1. Loads the CookStyle scan result and classifies offenses.
//  2. Loads the auto-correct preview (if any) for manual fix counts.
//  3. Looks up the aggregate Test Kitchen status.
//  4. Computes the blast radius from usage analysis and role dependencies.
//  5. Computes the weighted score and label.
//  6. Persists to the git_repo_complexity table.
func (s *ComplexityScorer) ScoreGitRepos(
	ctx context.Context,
	repos []datastore.GitRepo,
	targetChefVersion string,
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

	// Pre-load TK counts for the target version (bulk query).
	tkCounts, tkErr := s.db.ListGitKitchenCountsByTargetVersions(ctx, []string{targetChefVersion})
	if tkErr != nil {
		log.Error(fmt.Sprintf("failed to load TK counts: %v", tkErr))
		if tkCounts == nil {
			tkCounts = make(map[string]tkstatus.Counts)
		}
	}

	// Build work items.
	type workItem struct {
		Repo          datastore.GitRepo
		TargetVersion string
	}

	var items []workItem
	for _, repo := range repos {
		items = append(items, workItem{Repo: repo, TargetVersion: targetChefVersion})
	}

	classifiers := s.classifierCache(ctx, []string{targetChefVersion})

	batch := ComplexityBatchResult{
		Total:   len(items),
		Results: make([]ComplexityResult, 0, len(items)),
	}

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}

		result := s.scoreOneGitRepo(ctx, item.Repo, item.TargetVersion, blastRadii, tkCounts, classifiers[item.TargetVersion])
		batch.Results = append(batch.Results, result)

		switch {
		case result.Skipped:
			batch.Skipped++
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
	classifier CopClassifier,
) ComplexityResult {
	result := ComplexityResult{
		OrganisationName:  cb.OrganisationName,
		CookbookName:      cb.Name,
		CookbookVersion:   cb.Version,
		TargetChefVersion: targetChefVersion,
	}

	// Step 1: Load CookStyle result.
	csResult, csErr := s.db.GetServerCookbookCookstyleResult(ctx, cb.OrganisationName, cb.Name, cb.Version, targetChefVersion)
	if csErr != nil {
		result.Error = fmt.Errorf("loading cookstyle result: %w", csErr)
		return result
	}

	// If no CookStyle result exists, this cookbook has not been scanned yet.
	// Skip scoring so the cookbook remains "untested" rather than appearing
	// "compatible" with zero offenses.
	if csResult == nil {
		result.Skipped = true
		return result
	}

	offenseSummary := classifyOffenses(csResult.Offences, csResult.DeprecationCount, csResult.CorrectnessCount)

	// Step 2: Load auto-correct preview for manual fix count.
	preview, previewErr := s.db.GetServerCookbookAutocorrectPreview(ctx, csResult.OrganisationName, csResult.CookbookName, csResult.CookbookVersion, csResult.TargetChefVersion)
	if previewErr == nil && preview != nil {
		offenseSummary.AutoCorrectableCount = preview.CorrectableOffenses
		offenseSummary.ManualFixCount = preview.RemainingOffenses
	}

	// Step 3: Look up blast radius. Server cookbooks do not have Test
	// Kitchen results — that is a git-repo-only concept.
	blast := blastRadii[cb.Name]

	// Step 4: Compute score.
	input := ComplexityInput{
		OrganisationName:  cb.OrganisationName,
		CookbookName:      cb.Name,
		CookbookVersion:   cb.Version,
		TargetChefVersion: targetChefVersion,
		Cookstyle:         offenseSummary,
		Blast:             blast,
	}

	score := s.cookstyleScore(classifier, csResult.Offences, input)
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
	tkCounts map[string]tkstatus.Counts,
	classifier CopClassifier,
) ComplexityResult {
	result := ComplexityResult{
		CookbookName:      repo.Name,
		GitRepoURL:        repo.GitRepoURL,
		TargetChefVersion: targetChefVersion,
	}

	// Step 1: Load CookStyle result.
	csResult, csErr := s.db.GetGitRepoCookstyleResult(ctx, repo.Name, repo.GitRepoURL, targetChefVersion)
	if csErr != nil {
		result.Error = fmt.Errorf("loading cookstyle result: %w", csErr)
		return result
	}

	// If no CookStyle result exists, this repo has not been scanned yet.
	// Skip scoring so the cookbook remains "untested" rather than appearing
	// "compatible" with zero offenses.
	if csResult == nil {
		result.Skipped = true
		return result
	}

	var offenseSummary CookstyleOffenseSummary
	if csResult != nil {
		offenseSummary = classifyOffenses(csResult.Offences, csResult.DeprecationCount, csResult.CorrectnessCount)
	}

	// Step 2: Load auto-correct preview for manual fix count.
	if csResult != nil {
		preview, previewErr := s.db.GetGitRepoAutocorrectPreview(ctx, csResult.GitRepoName, csResult.GitRepoURL, csResult.TargetChefVersion)
		if previewErr == nil && preview != nil {
			offenseSummary.AutoCorrectableCount = preview.CorrectableOffenses
			offenseSummary.ManualFixCount = preview.RemainingOffenses
		}
	}

	// Step 3: Look up blast radius.
	blast := blastRadii[repo.Name]

	// Step 4: Derive TK status from pre-loaded counts.
	var tk TKStatus
	if counts, ok := tkCounts[repo.Name+"|"+targetChefVersion]; ok {
		tk.Status = counts.Status()
	}

	// Step 5: Compute score.
	input := ComplexityInput{
		CookbookName:      repo.Name,
		GitRepoURL:        repo.GitRepoURL,
		TargetChefVersion: targetChefVersion,
		Cookstyle:         offenseSummary,
		TestKitchen:       tk,
		Blast:             blast,
	}

	score := s.cookstyleScore(classifier, csResult.Offences, input)
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

	// Step 7: Persist.
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
	// Message discriminates poly-method cops during message-aware classification
	// (see specifications/cop-classification.md, Poly-method cops).
	Message string `json:"message"`
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
	if err == nil && latestAnalysis.OrganisationName != "" {
		summaries, sumErr := s.db.ListCookbookUsageSummaries(ctx, latestAnalysis.OrganisationName)
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
	if result.CookbookName == "" || result.TargetChefVersion == "" {
		return
	}

	log := s.logger.WithScope(logging.ScopeRemediation,
		logging.WithCookbook(result.CookbookName, result.CookbookVersion))

	params := datastore.UpsertServerCookbookComplexityParams{
		OrganisationName:     result.OrganisationName,
		CookbookName:         result.CookbookName,
		CookbookVersion:      result.CookbookVersion,
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
	if result.CookbookName == "" || result.TargetChefVersion == "" {
		return
	}

	log := s.logger.WithScope(logging.ScopeRemediation,
		logging.WithCookbook(result.CookbookName, ""))

	params := datastore.UpsertGitRepoComplexityParams{
		GitRepoName:          result.CookbookName,
		GitRepoURL:           result.GitRepoURL,
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
func (s *ComplexityScorer) ResetServerCookbookScores(ctx context.Context, orgName, cookbookName, cookbookVersion string) error {
	return s.db.DeleteServerCookbookComplexitiesByCookbook(ctx, orgName, cookbookName, cookbookVersion)
}

// ResetGitRepoScores deletes existing complexity scores for the given git
// repo, so they will be recomputed on the next analysis cycle.
func (s *ComplexityScorer) ResetGitRepoScores(ctx context.Context, gitRepoName, gitRepoURL string) error {
	return s.db.DeleteGitRepoComplexitiesByRepo(ctx, gitRepoName, gitRepoURL)
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
