// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package gitkitchen

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// InstanceStatus describes why an instance can or cannot run.
type InstanceStatus string

const (
	InstanceStatusMapped   InstanceStatus = "mapped"
	InstanceStatusUnmapped InstanceStatus = "unmapped"
	InstanceStatusSkipped  InstanceStatus = "skipped"
	InstanceStatusExcluded InstanceStatus = "excluded"
)

// PlannedInstance represents one expandable instance from a kitchen config.
type PlannedInstance struct {
	InstanceName string         `json:"instance_name"`
	SuiteName    string         `json:"suite_name"`
	PlatformName string         `json:"platform_name"`
	Status       InstanceStatus `json:"status"`
	StatusReason string         `json:"status_reason"`
	ImageName    string         `json:"image_name,omitempty"`
}

// PlanResult holds the full expansion of a repo's kitchen config.
type PlanResult struct {
	GitRepoName string            `json:"git_repo_name"`
	GitRepoURL  string            `json:"git_repo_url"`
	CommitSHA   string            `json:"commit_sha"`
	Instances   []PlannedInstance `json:"instances"`
	Total       int               `json:"total"`
	Mapped      int               `json:"mapped"`
	Unmapped    int               `json:"unmapped"`
	Skipped     int               `json:"skipped"`
	Excluded    int               `json:"excluded"`
}

// formatInstanceName forms a Test Kitchen style instance name from suite and
// platform. Dots are removed and underscores become hyphens.
func formatInstanceName(suite, platform string) string {
	s := strings.ReplaceAll(suite, ".", "")
	s = strings.ReplaceAll(s, "_", "-")
	p := strings.ReplaceAll(platform, ".", "")
	p = strings.ReplaceAll(p, "_", "-")
	return s + "-" + p
}

// PlanRepo expands a kitchen analysis result into planned instances.
func PlanRepo(ar datastore.KitchenAnalysisResult, platformMap []config.PlatformMapEntry) (*PlanResult, error) {
	var platforms []analysis.KitchenPlatform
	if err := json.Unmarshal(ar.Platforms, &platforms); err != nil {
		return nil, fmt.Errorf("gitkitchen: unmarshal platforms: %w", err)
	}

	var suites []analysis.KitchenSuite
	if err := json.Unmarshal(ar.Suites, &suites); err != nil {
		return nil, fmt.Errorf("gitkitchen: unmarshal suites: %w", err)
	}

	result := &PlanResult{
		GitRepoName: ar.GitRepoName,
		GitRepoURL:  ar.GitRepoURL,
		CommitSHA:   ar.HeadCommitSHA,
	}

	if len(suites) == 0 || len(platforms) == 0 {
		return result, nil
	}

	for _, suite := range suites {
		includeSet := toSet(suite.Includes)
		excludeSet := toSet(suite.Excludes)

		for _, plat := range platforms {
			if isExcluded(plat.Name, includeSet, excludeSet) {
				result.Instances = append(result.Instances, PlannedInstance{
					InstanceName: formatInstanceName(suite.Name, plat.Name),
					SuiteName:    suite.Name,
					PlatformName: plat.Name,
					Status:       InstanceStatusExcluded,
					StatusReason: fmt.Sprintf("platform %q excluded by suite %q", plat.Name, suite.Name),
				})
				result.Excluded++
				continue
			}

			m := config.MatchPlatform(plat.Name, platformMap)
			var inst PlannedInstance
			inst.InstanceName = formatInstanceName(suite.Name, plat.Name)
			inst.SuiteName = suite.Name
			inst.PlatformName = plat.Name

			switch {
			case m.Entry == nil:
				inst.Status = InstanceStatusUnmapped
				inst.StatusReason = fmt.Sprintf("no mapping for platform %q", plat.Name)
			case m.Entry.Skip:
				inst.Status = InstanceStatusSkipped
				inst.StatusReason = fmt.Sprintf("platform %q mapping has skip=true", plat.Name)
				result.Skipped++
			default:
				inst.Status = InstanceStatusMapped
				inst.ImageName = m.Entry.Image
				result.Mapped++
			}

			if inst.Status == InstanceStatusUnmapped {
				result.Unmapped++
			}

			result.Instances = append(result.Instances, inst)
		}
	}

	result.Total = len(result.Instances)
	return result, nil
}

func toSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

// isExcluded returns true if the platform should be excluded based on the
// suite's includes/excludes configuration.
func isExcluded(name string, includes, excludes map[string]bool) bool {
	if len(includes) > 0 && !includes[name] {
		return true
	}
	if excludes[name] {
		return true
	}
	return false
}
