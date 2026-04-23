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
	Name         string   `json:"name"`
	GitRepoURL   string   `json:"git_repo_url"`
	Platforms    []string `json:"platforms,omitempty"`
	Suites       []string `json:"suites,omitempty"`
	EstimatedVMs int      `json:"estimated_vms"`
}

// BatchEstimate summarises a resolved batch.
type BatchEstimate struct {
	TotalCookbooks    int                `json:"total_cookbooks"`
	TotalEstimatedVMs int                `json:"total_estimated_vms"`
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
	repos GitRepoLister
}

// NewResolver creates a Resolver.
func NewResolver(repos GitRepoLister) *Resolver {
	return &Resolver{repos: repos}
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
