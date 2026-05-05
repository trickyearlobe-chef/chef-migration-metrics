// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"context"
	"path"
)

// GitRepoLister provides read access to git repos for batch resolution.
type GitRepoLister interface {
	ListGitRepos(ctx context.Context) ([]GitRepo, error)
}

// GitRepo is the minimal git repo info needed for batch resolution.
type GitRepo struct {
	Name            string
	GitRepoURL      string
	HasTestSuite    bool
	KitchenExcluded bool
}

// KitchenAnalysisProvider provides kitchen analysis data for platform filtering.
type KitchenAnalysisProvider interface {
	GetKitchenAnalysisPlatforms(ctx context.Context, repoName string) ([]string, error)
}

// TestKitchenResultProvider provides previous TK results for status filtering.
type TestKitchenResultProvider interface {
	GetLatestTestKitchenStatus(ctx context.Context, repoName string) (string, error)
}

// ResolvedCookbook represents a cookbook that matched batch filters.
type ResolvedCookbook struct {
	Name           string   `json:"name"`
	GitRepoURL     string   `json:"git_repo_url"`
	Platforms      []string `json:"platforms,omitempty"`
	Suites         []string `json:"suites,omitempty"`
	EstimatedVMs   int      `json:"estimated_vms"`
	PlanningStatus string   `json:"planning_status,omitempty"`
	PlanningNote   string   `json:"planning_note,omitempty"`
	TotalInstances int      `json:"total_instances,omitempty"`
	Unmapped       int      `json:"unmapped,omitempty"`
	Skipped        int      `json:"skipped,omitempty"`
	Excluded       int      `json:"excluded,omitempty"`
	UserExcluded   int      `json:"user_excluded,omitempty"`
}

// BatchEstimate summarises a resolved batch.
type BatchEstimate struct {
	TotalCookbooks    int                `json:"total_cookbooks"`
	TotalEstimatedVMs int                `json:"total_estimated_vms"`
	SkippedCookbooks  int                `json:"skipped_cookbooks,omitempty"`
	PerPlatform       map[string]int     `json:"per_platform,omitempty"`
	Cookbooks         []ResolvedCookbook `json:"cookbooks"`
}

// Filters mirrors datastore.BatchFilters for use in the batch package.
type Filters struct {
	CookbookNames      []string `json:"cookbook_names,omitempty"`
	Platforms          []string `json:"platforms,omitempty"`
	ExcludeCookbooks   []string `json:"exclude_cookbooks,omitempty"`
	HasTestSuite       *bool    `json:"has_test_suite,omitempty"`
	PreviousStatus     string   `json:"previous_status,omitempty"`
	TargetChefVersions []string `json:"target_chef_versions,omitempty"`
	IncludeExcluded    bool     `json:"include_excluded,omitempty"`
}

// Resolver resolves batch filters into a list of matching cookbooks.
type Resolver struct {
	repos    GitRepoLister
	analysis KitchenAnalysisProvider   // optional, nil = skip platform filter
	results  TestKitchenResultProvider // optional, nil = skip status filter
}

// ResolverOption configures optional Resolver dependencies.
type ResolverOption func(*Resolver)

// WithAnalysisProvider sets an optional KitchenAnalysisProvider on the Resolver.
func WithAnalysisProvider(p KitchenAnalysisProvider) ResolverOption {
	return func(r *Resolver) { r.analysis = p }
}

// WithResultProvider sets an optional TestKitchenResultProvider on the Resolver.
func WithResultProvider(p TestKitchenResultProvider) ResolverOption {
	return func(r *Resolver) { r.results = p }
}

// NewResolver creates a Resolver with required repos dependency and optional providers.
func NewResolver(repos GitRepoLister, opts ...ResolverOption) *Resolver {
	r := &Resolver{repos: repos}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ResolveBatch applies the given filters to the git repo list and returns
// the matching cookbooks. Filters are AND-combined.
func (r *Resolver) ResolveBatch(ctx context.Context, filters Filters, maxCount *int) (BatchEstimate, error) {
	repos, err := r.repos.ListGitRepos(ctx)
	if err != nil {
		return BatchEstimate{}, err
	}

	remaining := repos

	// Exclusion filter: skip repos where KitchenExcluded == true unless IncludeExcluded.
	if !filters.IncludeExcluded {
		remaining = filterRepos(remaining, func(repo GitRepo) bool {
			return !repo.KitchenExcluded
		})
	}

	// HasTestSuite filter.
	if filters.HasTestSuite != nil {
		want := *filters.HasTestSuite
		remaining = filterRepos(remaining, func(repo GitRepo) bool {
			return repo.HasTestSuite == want
		})
	}

	// CookbookNames filter: keep only repos matching any pattern.
	if len(filters.CookbookNames) > 0 {
		remaining = filterRepos(remaining, func(repo GitRepo) bool {
			return matchAnyGlob(filters.CookbookNames, repo.Name)
		})
	}

	// ExcludeCookbooks filter: remove repos matching any pattern.
	if len(filters.ExcludeCookbooks) > 0 {
		remaining = filterRepos(remaining, func(repo GitRepo) bool {
			return !matchAnyGlob(filters.ExcludeCookbooks, repo.Name)
		})
	}

	// Platform filter: keep only repos that have at least one matching platform
	// in their kitchen analysis data. Requires analysis provider.
	if len(filters.Platforms) > 0 && r.analysis != nil {
		remaining = filterRepos(remaining, func(repo GitRepo) bool {
			platforms, err := r.analysis.GetKitchenAnalysisPlatforms(ctx, repo.Name)
			if err != nil || len(platforms) == 0 {
				return false // No analysis data = no match
			}
			for _, p := range platforms {
				if matchAnyGlob(filters.Platforms, p) {
					return true
				}
			}
			return false
		})
	}

	// Previous status filter: keep only repos whose last result matches.
	// Requires result provider.
	if filters.PreviousStatus != "" && r.results != nil {
		remaining = filterRepos(remaining, func(repo GitRepo) bool {
			status, err := r.results.GetLatestTestKitchenStatus(ctx, repo.Name)
			if err != nil {
				return false
			}
			return status == filters.PreviousStatus
		})
	}

	// MaxCount cap.
	if maxCount != nil && *maxCount > 0 && len(remaining) > *maxCount {
		remaining = remaining[:*maxCount]
	}

	cookbooks := make([]ResolvedCookbook, len(remaining))
	totalVMs := 0
	for i, repo := range remaining {
		cookbooks[i] = ResolvedCookbook{
			Name:       repo.Name,
			GitRepoURL: repo.GitRepoURL,
		}
		totalVMs += cookbooks[i].EstimatedVMs
	}

	// Populate platforms from analysis data when available.
	if r.analysis != nil {
		for i := range cookbooks {
			platforms, err := r.analysis.GetKitchenAnalysisPlatforms(ctx, cookbooks[i].Name)
			if err == nil {
				cookbooks[i].Platforms = platforms
				cookbooks[i].EstimatedVMs = len(platforms)
			}
		}
		// Recompute total estimated VMs.
		totalVMs = 0
		for _, c := range cookbooks {
			totalVMs += c.EstimatedVMs
		}
	}

	return BatchEstimate{
		TotalCookbooks:    len(cookbooks),
		TotalEstimatedVMs: totalVMs,
		Cookbooks:         cookbooks,
	}, nil
}

// filterRepos returns repos for which keep returns true.
func filterRepos(repos []GitRepo, keep func(GitRepo) bool) []GitRepo {
	result := make([]GitRepo, 0, len(repos))
	for _, r := range repos {
		if keep(r) {
			result = append(result, r)
		}
	}
	return result
}

// matchGlob returns true if name matches pattern. Supports * as wildcard.
// An empty pattern matches nothing. A pattern without * requires exact match.
func matchGlob(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	matched, err := path.Match(pattern, name)
	if err != nil {
		return false
	}
	return matched
}

// matchAnyGlob returns true if name matches any of the patterns.
func matchAnyGlob(patterns []string, name string) bool {
	for _, p := range patterns {
		if matchGlob(p, name) {
			return true
		}
	}
	return false
}
