// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"context"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/gitkitchen"
)

// Planning status constants.
const (
	PlanningStatusPlanned        = "planned"
	PlanningStatusNoAnalysis     = "no_analysis"
	PlanningStatusExclusionError = "exclusion_error"
	PlanningStatusPlanError      = "plan_error"
)

// AnalysisLoader loads kitchen analysis data for a repo.
type AnalysisLoader interface {
	GetKitchenAnalysisResultByName(ctx context.Context, repoName string) (*datastore.KitchenAnalysisResult, error)
}

// ExclusionsLoader loads instance exclusions for a repo.
type ExclusionsLoader interface {
	LoadInstanceExclusions(ctx context.Context, repoName string) ([]gitkitchen.InstanceExclusion, error)
}

// PlanBatch expands each resolved cookbook using PlanRepo and returns an
// enriched estimate with accurate VM counts and per-cookbook breakdowns.
func PlanBatch(
	ctx context.Context,
	cookbooks []ResolvedCookbook,
	platformMap []config.PlatformMapEntry,
	analysisLoader AnalysisLoader,
	exclusionsLoader ExclusionsLoader,
) BatchEstimate {
	result := BatchEstimate{
		TotalCookbooks: len(cookbooks),
		Cookbooks:      make([]ResolvedCookbook, len(cookbooks)),
		PerPlatform:    make(map[string]int),
	}

	for i, cb := range cookbooks {
		result.Cookbooks[i] = planCookbook(ctx, cb, platformMap, analysisLoader, exclusionsLoader, result.PerPlatform)
	}

	// Compute totals.
	for _, cb := range result.Cookbooks {
		if cb.PlanningStatus != PlanningStatusPlanned {
			result.SkippedCookbooks++
		}
		result.TotalEstimatedVMs += cb.EstimatedVMs
	}

	return result
}

func planCookbook(
	ctx context.Context,
	cb ResolvedCookbook,
	platformMap []config.PlatformMapEntry,
	analysisLoader AnalysisLoader,
	exclusionsLoader ExclusionsLoader,
	perPlatform map[string]int,
) ResolvedCookbook {
	// Load analysis.
	ar, err := analysisLoader.GetKitchenAnalysisResultByName(ctx, cb.Name)
	if err != nil || ar == nil {
		cb.PlanningStatus = PlanningStatusNoAnalysis
		cb.PlanningNote = "no kitchen analysis data available"
		return cb
	}

	// Load exclusions.
	exclusions, err := exclusionsLoader.LoadInstanceExclusions(ctx, cb.Name)
	if err != nil {
		cb.PlanningStatus = PlanningStatusExclusionError
		cb.PlanningNote = "failed to load instance exclusions"
		return cb
	}

	// Plan.
	plan, err := gitkitchen.PlanRepo(*ar, platformMap, exclusions...)
	if err != nil {
		cb.PlanningStatus = PlanningStatusPlanError
		cb.PlanningNote = "planning failed: " + err.Error()
		return cb
	}

	// Populate result from plan.
	cb.PlanningStatus = PlanningStatusPlanned
	cb.TotalInstances = plan.Total
	cb.EstimatedVMs = plan.Mapped
	cb.Unmapped = plan.Unmapped
	cb.Skipped = plan.Skipped
	cb.Excluded = plan.Excluded
	cb.UserExcluded = plan.UserExcluded

	// Extract platform and suite names from plan instances.
	platSet := make(map[string]struct{})
	suiteSet := make(map[string]struct{})
	for _, inst := range plan.Instances {
		platSet[inst.PlatformName] = struct{}{}
		suiteSet[inst.SuiteName] = struct{}{}
		if inst.Status == gitkitchen.InstanceStatusMapped {
			perPlatform[inst.PlatformName]++
		}
	}
	cb.Platforms = setToSlice(platSet)
	cb.Suites = setToSlice(suiteSet)

	return cb
}

func setToSlice(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	return s
}
