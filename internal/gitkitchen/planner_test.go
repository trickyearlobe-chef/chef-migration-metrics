// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package gitkitchen

import (
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func makeAnalysis(platforms []analysis.KitchenPlatform, suites []analysis.KitchenSuite) datastore.KitchenAnalysisResult {
	pJSON, _ := json.Marshal(platforms)
	sJSON, _ := json.Marshal(suites)
	return datastore.KitchenAnalysisResult{
		GitRepoName:   "test-cookbook",
		GitRepoURL:    "https://git.example.com/test-cookbook.git",
		HeadCommitSHA: "abc123",
		Platforms:     pJSON,
		Suites:        sJSON,
	}
}

func TestPlanRepo_BasicExpansion(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
		{Name: "centos-7"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default"},
		{Name: "integration"},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-img"},
		{KitchenName: "centos-7", Image: "centos-7-img"},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 4 {
		t.Errorf("expected 4 total instances, got %d", result.Total)
	}
	if result.Mapped != 4 {
		t.Errorf("expected 4 mapped, got %d", result.Mapped)
	}
	for _, inst := range result.Instances {
		if inst.Status != InstanceStatusMapped {
			t.Errorf("expected all mapped, got %s for %s", inst.Status, inst.InstanceName)
		}
	}
}

func TestPlanRepo_SuiteExcludes(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
		{Name: "centos-7"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default", Excludes: []string{"centos-7"}},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-img"},
		{KitchenName: "centos-7", Image: "centos-7-img"},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("expected 2 total instances, got %d", result.Total)
	}
	if result.Mapped != 1 {
		t.Errorf("expected 1 mapped, got %d", result.Mapped)
	}
	if result.Excluded != 1 {
		t.Errorf("expected 1 excluded, got %d", result.Excluded)
	}
}

func TestPlanRepo_SuiteIncludes(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
		{Name: "centos-7"},
		{Name: "debian-11"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default", Includes: []string{"ubuntu-22.04"}},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-img"},
		{KitchenName: "centos-7", Image: "centos-7-img"},
		{KitchenName: "debian-11", Image: "debian-11-img"},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("expected 3 total instances, got %d", result.Total)
	}
	if result.Mapped != 1 {
		t.Errorf("expected 1 mapped, got %d", result.Mapped)
	}
	if result.Excluded != 2 {
		t.Errorf("expected 2 excluded, got %d", result.Excluded)
	}
}

func TestPlanRepo_IncludesAndExcludes(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
		{Name: "centos-7"},
		{Name: "debian-11"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default", Includes: []string{"ubuntu-22.04", "centos-7"}, Excludes: []string{"centos-7"}},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-img"},
		{KitchenName: "centos-7", Image: "centos-7-img"},
		{KitchenName: "debian-11", Image: "debian-11-img"},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// debian-11 excluded by includes, centos-7 excluded by excludes, ubuntu-22.04 mapped
	if result.Mapped != 1 {
		t.Errorf("expected 1 mapped, got %d", result.Mapped)
	}
	if result.Excluded != 2 {
		t.Errorf("expected 2 excluded, got %d", result.Excluded)
	}
}

func TestPlanRepo_UnmappedPlatform(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "arch-linux"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default"},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-img"},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Unmapped != 1 {
		t.Errorf("expected 1 unmapped, got %d", result.Unmapped)
	}
	if result.Instances[0].Status != InstanceStatusUnmapped {
		t.Errorf("expected unmapped status, got %s", result.Instances[0].Status)
	}
	if result.Instances[0].StatusReason == "" {
		t.Error("expected non-empty status reason for unmapped platform")
	}
}

func TestPlanRepo_SkippedPlatform(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "windows-2022"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default"},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "windows-2022", Image: "win-img", Skip: true},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
	}
	if result.Instances[0].Status != InstanceStatusSkipped {
		t.Errorf("expected skipped status, got %s", result.Instances[0].Status)
	}
}

func TestPlanRepo_MixedStatuses(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
		{Name: "arch-linux"},
		{Name: "windows-2022"},
		{Name: "centos-7"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default", Excludes: []string{"centos-7"}},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-img"},
		{KitchenName: "windows-2022", Image: "win-img", Skip: true},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Mapped != 1 {
		t.Errorf("expected 1 mapped, got %d", result.Mapped)
	}
	if result.Unmapped != 1 {
		t.Errorf("expected 1 unmapped, got %d", result.Unmapped)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
	}
	if result.Excluded != 1 {
		t.Errorf("expected 1 excluded, got %d", result.Excluded)
	}
	if result.Total != 4 {
		t.Errorf("expected 4 total, got %d", result.Total)
	}
}

func TestPlanRepo_NoPlatforms(t *testing.T) {
	suites := []analysis.KitchenSuite{
		{Name: "default"},
	}
	ar := makeAnalysis([]analysis.KitchenPlatform{}, suites)
	result, err := PlanRepo(ar, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 total, got %d", result.Total)
	}
}

func TestPlanRepo_NoSuites(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
	}
	ar := makeAnalysis(platforms, []analysis.KitchenSuite{})
	result, err := PlanRepo(ar, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 total, got %d", result.Total)
	}
}

func TestPlanRepo_InstanceNameFormatting(t *testing.T) {
	tests := []struct {
		suite    string
		platform string
		want     string
	}{
		{"default", "ubuntu-22.04", "default-ubuntu-2204"},
		{"my_suite", "centos-7", "my-suite-centos-7"},
		{"test", "windows_2022", "test-windows-2022"},
		{"dots.here", "name.with.dots", "dotshere-namewithdots"},
	}

	// Test using the exported helper indirectly via PlanRepo
	// but also test formatInstanceName directly
	for _, tc := range tests {
		got := formatInstanceName(tc.suite, tc.platform)
		if got != tc.want {
			t.Errorf("formatInstanceName(%q, %q) = %q, want %q", tc.suite, tc.platform, got, tc.want)
		}
	}
}

func TestPlanRepo_Counts(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
		{Name: "centos-7"},
		{Name: "arch-linux"},
		{Name: "windows-2022"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default", Excludes: []string{"arch-linux"}},
		{Name: "integration"},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-img"},
		{KitchenName: "centos-7", Image: "centos-img"},
		{KitchenName: "windows-2022", Image: "win-img", Skip: true},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// suite "default": ubuntu(mapped) + centos(mapped) + arch(excluded) + windows(skipped) = 4
	// suite "integration": ubuntu(mapped) + centos(mapped) + arch(unmapped) + windows(skipped) = 4
	// Total = 8
	if result.Total != 8 {
		t.Errorf("expected 8 total, got %d", result.Total)
	}
	// mapped: default(ubuntu, centos) + integration(ubuntu, centos) = 4
	if result.Mapped != 4 {
		t.Errorf("expected 4 mapped, got %d", result.Mapped)
	}
	// unmapped: integration(arch) = 1
	if result.Unmapped != 1 {
		t.Errorf("expected 1 unmapped, got %d", result.Unmapped)
	}
	// skipped: default(windows) + integration(windows) = 2
	if result.Skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", result.Skipped)
	}
	// excluded: default(arch) = 1
	if result.Excluded != 1 {
		t.Errorf("expected 1 excluded, got %d", result.Excluded)
	}
}

func TestPlanRepo_UserExclusion(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
		{Name: "centos-7"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default"},
		{Name: "integration"},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-img"},
		{KitchenName: "centos-7", Image: "centos-7-img"},
	}

	exclusions := []InstanceExclusion{
		{SuiteName: "default", PlatformName: "centos-7", Reason: "hardcoded vagrant IP"},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap, exclusions...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 4 instances total: default-ubuntu(mapped), default-centos(user_excluded),
	// integration-ubuntu(mapped), integration-centos(mapped)
	if result.Total != 4 {
		t.Errorf("expected 4 total, got %d", result.Total)
	}
	if result.Mapped != 3 {
		t.Errorf("expected 3 mapped, got %d", result.Mapped)
	}
	if result.UserExcluded != 1 {
		t.Errorf("expected 1 user_excluded, got %d", result.UserExcluded)
	}

	// Verify the excluded instance.
	for _, inst := range result.Instances {
		if inst.SuiteName == "default" && inst.PlatformName == "centos-7" {
			if inst.Status != InstanceStatusUserExcluded {
				t.Errorf("expected user_excluded, got %s", inst.Status)
			}
			if inst.StatusReason != "hardcoded vagrant IP" {
				t.Errorf("expected reason, got %q", inst.StatusReason)
			}
			return
		}
	}
	t.Error("did not find default-centos-7 instance in plan")
}

func TestPlanRepo_UserExclusionOnUnmappedPlatformIgnored(t *testing.T) {
	platforms := []analysis.KitchenPlatform{
		{Name: "ubuntu-22.04"},
		{Name: "arch-linux"},
	}
	suites := []analysis.KitchenSuite{
		{Name: "default"},
	}
	pmap := []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-img"},
	}

	// Exclude arch-linux, but it's unmapped so exclusion has no practical effect.
	exclusions := []InstanceExclusion{
		{SuiteName: "default", PlatformName: "arch-linux", Reason: "no image available"},
	}

	result, err := PlanRepo(makeAnalysis(platforms, suites), pmap, exclusions...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UserExcluded != 0 {
		t.Errorf("expected 0 user_excluded (unmapped takes precedence), got %d", result.UserExcluded)
	}
	if result.Unmapped != 1 {
		t.Errorf("expected 1 unmapped, got %d", result.Unmapped)
	}
}
