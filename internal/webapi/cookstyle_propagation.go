// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ---------------------------------------------------------------------------
// Re-evaluation propagation (cop reclassification / custom-cop change)
// ---------------------------------------------------------------------------
//
// A cop reclassification (operator override PUT/DELETE) or a custom-cop change
// alters the single-source-of-truth derivation `status = f(offences,
// classification)` for every result containing that cop. This propagator runs
// the scoped recompute closure synchronously:
//
//	affected results → re-derive passed → recompute git compat
//	  → re-score complexity (classification-weighted) for affected units
//	  → re-evaluate readiness for dependent (affected) organisations
//
// Nothing global is touched: the closure is exactly the cop's affected targets.
// See specifications/cop-classification.md (Re-evaluation & Propagation) and the
// derivation/invalidation dependency graph in
// plans/cookstyle-status-consistency.md.

// CookstylePropagationStore is the datastore subset the propagator needs.
// *datastore.DB satisfies it; declared as an interface for mock-based testing.
type CookstylePropagationStore interface {
	ListCopClassifications(ctx context.Context) ([]datastore.CopClassification, error)
	ListServerCookbookCookstyleResultsWithCop(ctx context.Context, copName, targetChefVersion string) ([]datastore.CookstyleResultRef, error)
	ListGitRepoCookstyleResultsWithCop(ctx context.Context, copName, targetChefVersion string) ([]datastore.CookstyleResultRef, error)
	UpdateServerCookbookCookstyleVerdict(ctx context.Context, organisationName, cookbookName, cookbookVersion, targetChefVersion string, passed bool, status string) error
	UpdateGitRepoCookstyleVerdict(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string, passed bool, status string) error
	RecomputeGitRepoCompatibilityStatus(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) error
	RecomputeAllRoleCompatStatus(ctx context.Context, targetChefVersion string) error
	ListOrganisations(ctx context.Context) ([]datastore.Organisation, error)
}

// ComplexityRescorer re-scores classification-weighted complexity for a scoped
// set of cookbooks/repos. *remediation.ComplexityScorer satisfies it.
type ComplexityRescorer interface {
	ScoreServerCookbooks(ctx context.Context, cookbooks []datastore.ServerCookbook, targetChefVersion, organisationID string) remediation.ComplexityBatchResult
	ScoreGitRepos(ctx context.Context, repos []datastore.GitRepo, targetChefVersion, organisationID string) remediation.ComplexityBatchResult
}

// ReadinessRecomputer re-evaluates node readiness for an organisation.
// *analysis.ReadinessEvaluator satisfies it.
type ReadinessRecomputer interface {
	EvaluateOrganisation(ctx context.Context, organisationID, orgName, targetChefVersion string) ([]analysis.ReadinessResult, error)
}

// CookstylePropagator runs the scoped recompute closure after a classification
// or custom-cop change. The scorer and readiness recomputer are optional
// (nil-safe) — when unset, that stage of the closure is skipped.
type CookstylePropagator struct {
	store     CookstylePropagationStore
	scorer    ComplexityRescorer
	readiness ReadinessRecomputer
	rulesFn   func() analysis.CookstyleFailureRules
	logger    func(level, msg string)
}

// NewCookstylePropagator builds a propagator. rulesFn must return the live
// failure rules (fallback for unclassified cops); a nil rulesFn defaults to
// analysis.DefaultFailureRules.
func NewCookstylePropagator(
	store CookstylePropagationStore,
	scorer ComplexityRescorer,
	readiness ReadinessRecomputer,
	rulesFn func() analysis.CookstyleFailureRules,
	logger func(level, msg string),
) *CookstylePropagator {
	if rulesFn == nil {
		rulesFn = analysis.DefaultFailureRules
	}
	return &CookstylePropagator{
		store:     store,
		scorer:    scorer,
		readiness: readiness,
		rulesFn:   rulesFn,
		logger:    logger,
	}
}

// PropagationResult reports what the recompute closure touched.
type PropagationResult struct {
	Target                  string `json:"target_chef_version"`
	ServerResultsChanged    int    `json:"server_results_changed"`
	GitResultsChanged       int    `json:"git_results_changed"`
	CookbooksRescored       int    `json:"cookbooks_rescored"`
	GitReposRescored        int    `json:"git_repos_rescored"`
	OrgsReadinessRecomputed int    `json:"orgs_readiness_recomputed"`
}

// PropagateReclassification runs the full scoped recompute closure for a single
// cop × target version. It is best-effort: per-unit errors are logged and do not
// abort the closure, so a partial recompute is preferred to none.
func (p *CookstylePropagator) PropagateReclassification(ctx context.Context, copName, targetChefVersion string) (PropagationResult, error) {
	result := PropagationResult{Target: targetChefVersion}
	if p == nil || p.store == nil {
		return result, nil
	}

	resolver := p.buildResolver(ctx, targetChefVersion)
	rules := p.rulesFn()

	// --- Affected server cookbook results: re-derive passed. ---
	serverRefs, err := p.store.ListServerCookbookCookstyleResultsWithCop(ctx, copName, targetChefVersion)
	if err != nil {
		return result, fmt.Errorf("propagation: listing server results for cop %q: %w", copName, err)
	}

	// Collect affected cookbooks per organisation (for complexity) and the set
	// of organisations whose node readiness must be re-evaluated.
	cbsByOrg := map[string][]datastore.ServerCookbook{}
	readinessOrgs := map[string]bool{}

	for i := range serverRefs {
		ref := &serverRefs[i]
		if ref.ErrorMessage != "" {
			continue // inconclusive scan — not a verdict
		}
		newStatus, newPassed := deriveRefStatus(ref, rules, resolver)
		if newStatus != ref.CookstyleStatus {
			if uerr := p.store.UpdateServerCookbookCookstyleVerdict(ctx, ref.OrganisationName, ref.CookbookName, ref.CookbookVersion, ref.TargetChefVersion, newPassed, newStatus); uerr != nil {
				p.logf("ERROR", "propagation: updating server verdict for %s/%s: %v", ref.OrganisationName, ref.CookbookName, uerr)
				continue
			}
			result.ServerResultsChanged++
		}
		cbsByOrg[ref.OrganisationName] = append(cbsByOrg[ref.OrganisationName], datastore.ServerCookbook{
			OrganisationName: ref.OrganisationName,
			Name:             ref.CookbookName,
			Version:          ref.CookbookVersion,
		})
		readinessOrgs[ref.OrganisationName] = true
	}

	// --- Affected git repo results: re-derive passed + recompute compat. ---
	gitRefs, err := p.store.ListGitRepoCookstyleResultsWithCop(ctx, copName, targetChefVersion)
	if err != nil {
		return result, fmt.Errorf("propagation: listing git results for cop %q: %w", copName, err)
	}

	var gitRepos []datastore.GitRepo
	for i := range gitRefs {
		ref := &gitRefs[i]
		if ref.ErrorMessage != "" {
			continue
		}
		newStatus, newPassed := deriveRefStatus(ref, rules, resolver)
		if newStatus != ref.CookstyleStatus {
			if uerr := p.store.UpdateGitRepoCookstyleVerdict(ctx, ref.GitRepoName, ref.GitRepoURL, ref.TargetChefVersion, newPassed, newStatus); uerr != nil {
				p.logf("ERROR", "propagation: updating git verdict for %s: %v", ref.GitRepoName, uerr)
				continue
			}
			result.GitResultsChanged++
			if rerr := p.store.RecomputeGitRepoCompatibilityStatus(ctx, ref.GitRepoName, ref.GitRepoURL, ref.TargetChefVersion); rerr != nil {
				p.logf("ERROR", "propagation: recomputing git compat for %s: %v", ref.GitRepoName, rerr)
			}
		}
		gitRepos = append(gitRepos, datastore.GitRepo{Name: ref.GitRepoName, GitRepoURL: ref.GitRepoURL})
	}

	// --- Re-score classification-weighted complexity for the affected units. ---
	p.rescoreComplexity(ctx, targetChefVersion, cbsByOrg, gitRepos, &result)

	// --- Re-materialise role compatibility. Roles derive from the same cookstyle
	// results whose verdicts just changed; a single bulk pass keeps the roles
	// list from drifting after a reclassification. ---
	if rerr := p.store.RecomputeAllRoleCompatStatus(ctx, targetChefVersion); rerr != nil {
		p.logf("ERROR", "propagation: recomputing role compat for target %q: %v", targetChefVersion, rerr)
	}

	// --- Re-evaluate readiness for the affected (dependent) organisations. ---
	// Git repos do not participate in node run-lists, so readiness recompute is
	// scoped to organisations owning affected server cookbooks.
	if p.readiness != nil {
		for org := range readinessOrgs {
			if _, rerr := p.readiness.EvaluateOrganisation(ctx, org, org, targetChefVersion); rerr != nil {
				p.logf("ERROR", "propagation: recomputing readiness for org %q: %v", org, rerr)
				continue
			}
			result.OrgsReadinessRecomputed++
		}
	}

	return result, nil
}

// rescoreComplexity re-scores complexity for the affected cookbooks (grouped by
// organisation for blast-radius context) and git repos. Git repos are not
// organisation-scoped; to give them blast-radius context they are scored once
// per affected organisation (matching the collector's per-org pass). When only
// git repos are affected, all organisations are used.
func (p *CookstylePropagator) rescoreComplexity(ctx context.Context, target string, cbsByOrg map[string][]datastore.ServerCookbook, gitRepos []datastore.GitRepo, result *PropagationResult) {
	if p.scorer == nil {
		return
	}

	complexityOrgs := make(map[string]bool, len(cbsByOrg))
	for org := range cbsByOrg {
		complexityOrgs[org] = true
	}
	if len(gitRepos) > 0 && len(complexityOrgs) == 0 {
		// Git-only change: re-score git complexity against every organisation.
		orgs, err := p.store.ListOrganisations(ctx)
		if err != nil {
			p.logf("ERROR", "propagation: listing organisations for git complexity: %v", err)
		}
		for _, o := range orgs {
			complexityOrgs[o.Name] = true
		}
	}

	for org := range complexityOrgs {
		if cbs := cbsByOrg[org]; len(cbs) > 0 {
			p.scorer.ScoreServerCookbooks(ctx, cbs, target, org)
			result.CookbooksRescored += len(cbs)
		}
		if len(gitRepos) > 0 {
			p.scorer.ScoreGitRepos(ctx, gitRepos, target, org)
		}
	}
	if len(gitRepos) > 0 && len(complexityOrgs) > 0 {
		result.GitReposRescored = len(gitRepos)
	}
}

// buildResolver constructs a classification resolver for a target version,
// loading operator overrides from the store. RemovedIn auto-seed and curated
// defaults still apply, so classification works without operator input.
func (p *CookstylePropagator) buildResolver(ctx context.Context, target string) *analysis.CopClassificationResolver {
	overrides := map[string]string{}
	if rows, err := p.store.ListCopClassifications(ctx); err == nil {
		for _, row := range rows {
			overrides[row.CopName] = row.Classification
		}
	} else {
		p.logf("WARN", "propagation: loading classifications for target %q: %v", target, err)
	}
	return &analysis.CopClassificationResolver{OperatorOverrides: overrides, TargetChefVersion: target}
}

// deriveRefStatus re-derives the classification rollup status for a result from
// its stored offences using the single-source-of-truth derivation. The
// back-compat passed boolean is status != blocked.
func deriveRefStatus(ref *datastore.CookstyleResultRef, rules analysis.CookstyleFailureRules, resolver *analysis.CopClassificationResolver) (status string, passed bool) {
	offenses := refOffenses(ref.Offences)
	status = analysis.DeriveCookstyleStatus(offenses, rules, resolver)
	return status, status != analysis.StatusBlocked
}

// refOffenses parses the stored enriched offences JSON into the minimal offense
// shape the derivation needs (cop name + severity). Handles both the flat and
// file-grouped formats via parseFullOffenses.
func refOffenses(offencesJSON []byte) []analysis.CookstyleOffense {
	full := parseFullOffenses(offencesJSON)
	offenses := make([]analysis.CookstyleOffense, len(full))
	for i, o := range full {
		offenses[i] = analysis.CookstyleOffense{CopName: o.CopName, Severity: o.Severity}
	}
	return offenses
}

func (p *CookstylePropagator) logf(level, format string, args ...any) {
	if p == nil || p.logger == nil {
		return
	}
	p.logger(level, fmt.Sprintf(format, args...))
}
