// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// buildNodeMetricsPayload tests
// ---------------------------------------------------------------------------

func TestBuildNodeMetricsPayload_BasicAggregation(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	params := []datastore.InsertNodeSnapshotParams{
		{NodeName: "fresh-ready", ChefVersion: "18.5.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-1 * time.Hour).Unix())},
		{NodeName: "fresh-blocked-cs", ChefVersion: "17.0.0", PlatformFamily: "rhel", OhaiTime: float64(now.Add(-2 * time.Hour).Unix())},
		{NodeName: "fresh-blocked-tk", ChefVersion: "17.0.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-3 * time.Hour).Unix())},
		{NodeName: "fresh-blocked-disk", ChefVersion: "18.5.0", PlatformFamily: "windows", OhaiTime: float64(now.Add(-30 * time.Minute).Unix())},
		{NodeName: "warning-node", ChefVersion: "16.0.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-4 * 24 * time.Hour).Unix())},
		{NodeName: "critical-node", ChefVersion: "15.0.0", PlatformFamily: "rhel", OhaiTime: float64(now.Add(-10 * 24 * time.Hour).Unix())},
	}

	diskOK := true
	diskBad := false
	results := []analysis.ReadinessResult{
		{NodeName: "fresh-ready", IsReady: true, AllCookbooksCompatible: true, SufficientDiskSpace: &diskOK},
		{NodeName: "fresh-blocked-cs", IsReady: false, AllCookbooksCompatible: false, SufficientDiskSpace: &diskOK,
			BlockingCookbooks: []analysis.BlockingCookbook{{Name: "cb1", Source: analysis.SourceGitCookstyle}}},
		{NodeName: "fresh-blocked-tk", IsReady: false, AllCookbooksCompatible: false, SufficientDiskSpace: &diskOK,
			BlockingCookbooks: []analysis.BlockingCookbook{{Name: "cb2", Source: analysis.SourceGitTestKitchen}}},
		{NodeName: "fresh-blocked-disk", IsReady: false, AllCookbooksCompatible: true, SufficientDiskSpace: &diskBad},
		{NodeName: "warning-node", IsReady: false, AllCookbooksCompatible: true, SufficientDiskSpace: &diskOK},
		{NodeName: "critical-node", IsReady: false, AllCookbooksCompatible: false, SufficientDiskSpace: nil},
	}

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    params,
		ReadinessResults:  results,
		TargetChefVersion: "18.5.0",
		WarningHours:      72,
		CriticalDays:      7,
		RequiredDiskMB:    3000,
		Now:               now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nodeMetricsPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Total nodes.
	if got.TotalNodes != 6 {
		t.Errorf("total_nodes = %d, want 6", got.TotalNodes)
	}

	// Staleness breakdown.
	if got.ByStaleness.Fresh != 4 {
		t.Errorf("by_staleness.fresh = %d, want 4", got.ByStaleness.Fresh)
	}
	if got.ByStaleness.Warning != 1 {
		t.Errorf("by_staleness.warning = %d, want 1", got.ByStaleness.Warning)
	}
	if got.ByStaleness.Critical != 1 {
		t.Errorf("by_staleness.critical = %d, want 1", got.ByStaleness.Critical)
	}

	// Fresh breakdown.
	if got.Fresh.Total != 4 {
		t.Errorf("fresh.total = %d, want 4", got.Fresh.Total)
	}
	if got.Fresh.Ready != 1 {
		t.Errorf("fresh.ready = %d, want 1", got.Fresh.Ready)
	}
	if got.Fresh.BlockedTotal != 3 {
		t.Errorf("fresh.blocked_total = %d, want 3", got.Fresh.BlockedTotal)
	}

	// Blocking reasons.
	if got.Fresh.BlockedBy.Cookstyle != 1 {
		t.Errorf("blocked_by.cookstyle = %d, want 1", got.Fresh.BlockedBy.Cookstyle)
	}
	if got.Fresh.BlockedBy.TestKitchen != 1 {
		t.Errorf("blocked_by.test_kitchen = %d, want 1", got.Fresh.BlockedBy.TestKitchen)
	}
	if got.Fresh.BlockedBy.Disk != 1 {
		t.Errorf("blocked_by.disk = %d, want 1", got.Fresh.BlockedBy.Disk)
	}
	if got.Fresh.BlockedBy.FoodCritic != 0 {
		t.Errorf("blocked_by.foodcritic = %d, want 0", got.Fresh.BlockedBy.FoodCritic)
	}
	if got.Fresh.BlockedBy.ChefSpec != 0 {
		t.Errorf("blocked_by.chefspec = %d, want 0", got.Fresh.BlockedBy.ChefSpec)
	}

	// Version distribution (fresh only).
	if got.Fresh.ByVersion["18.5.0"] != 2 {
		t.Errorf("by_version[18.5.0] = %d, want 2", got.Fresh.ByVersion["18.5.0"])
	}
	if got.Fresh.ByVersion["17.0.0"] != 2 {
		t.Errorf("by_version[17.0.0] = %d, want 2", got.Fresh.ByVersion["17.0.0"])
	}

	// Platform family distribution (fresh only).
	if got.Fresh.ByPlatformFamily["debian"] != 2 {
		t.Errorf("by_platform_family[debian] = %d, want 2", got.Fresh.ByPlatformFamily["debian"])
	}
	if got.Fresh.ByPlatformFamily["rhel"] != 1 {
		t.Errorf("by_platform_family[rhel] = %d, want 1", got.Fresh.ByPlatformFamily["rhel"])
	}
	if got.Fresh.ByPlatformFamily["windows"] != 1 {
		t.Errorf("by_platform_family[windows] = %d, want 1", got.Fresh.ByPlatformFamily["windows"])
	}

	// Target version.
	if got.TargetChefVer != "18.5.0" {
		t.Errorf("target_chef_version = %q, want %q", got.TargetChefVer, "18.5.0")
	}

	// Thresholds.
	if got.Thresholds.WarningHours != 72 {
		t.Errorf("thresholds.warning_hours = %d, want 72", got.Thresholds.WarningHours)
	}
	if got.Thresholds.CriticalDays != 7 {
		t.Errorf("thresholds.critical_days = %d, want 7", got.Thresholds.CriticalDays)
	}
	if got.Thresholds.RequiredDiskMB != 3000 {
		t.Errorf("thresholds.required_disk_mb = %d, want 3000", got.Thresholds.RequiredDiskMB)
	}
}

func TestBuildNodeMetricsPayload_EmptyInput(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    []datastore.InsertNodeSnapshotParams{},
		ReadinessResults:  []analysis.ReadinessResult{},
		TargetChefVersion: "18.5.0",
		WarningHours:      72,
		CriticalDays:      7,
		RequiredDiskMB:    3000,
		Now:               now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nodeMetricsPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got.TotalNodes != 0 {
		t.Errorf("total_nodes = %d, want 0", got.TotalNodes)
	}
	if got.Fresh.Total != 0 {
		t.Errorf("fresh.total = %d, want 0", got.Fresh.Total)
	}
	if got.ByStaleness.Fresh != 0 {
		t.Errorf("by_staleness.fresh = %d, want 0", got.ByStaleness.Fresh)
	}
	if got.Fresh.ByVersion == nil {
		t.Error("fresh.by_version should be empty map, not nil")
	}
	if got.Fresh.ByPlatformFamily == nil {
		t.Error("fresh.by_platform_family should be empty map, not nil")
	}
}

func TestBuildNodeMetricsPayload_AllFreshAllReady(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	diskOK := true

	params := []datastore.InsertNodeSnapshotParams{
		{NodeName: "n1", ChefVersion: "18.5.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-1 * time.Hour).Unix())},
		{NodeName: "n2", ChefVersion: "18.5.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-2 * time.Hour).Unix())},
		{NodeName: "n3", ChefVersion: "18.5.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-3 * time.Hour).Unix())},
	}
	results := []analysis.ReadinessResult{
		{NodeName: "n1", IsReady: true, SufficientDiskSpace: &diskOK},
		{NodeName: "n2", IsReady: true, SufficientDiskSpace: &diskOK},
		{NodeName: "n3", IsReady: true, SufficientDiskSpace: &diskOK},
	}

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    params,
		ReadinessResults:  results,
		TargetChefVersion: "18.5.0",
		WarningHours:      72,
		CriticalDays:      7,
		RequiredDiskMB:    3000,
		Now:               now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nodeMetricsPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got.TotalNodes != 3 {
		t.Errorf("total_nodes = %d, want 3", got.TotalNodes)
	}
	if got.ByStaleness.Fresh != 3 {
		t.Errorf("by_staleness.fresh = %d, want 3", got.ByStaleness.Fresh)
	}
	if got.Fresh.Ready != 3 {
		t.Errorf("fresh.ready = %d, want 3", got.Fresh.Ready)
	}
	if got.Fresh.BlockedTotal != 0 {
		t.Errorf("fresh.blocked_total = %d, want 0", got.Fresh.BlockedTotal)
	}
}

func TestBuildNodeMetricsPayload_MultipleBlockingReasons(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	diskBad := false

	params := []datastore.InsertNodeSnapshotParams{
		{NodeName: "multi-blocked", ChefVersion: "17.0.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-1 * time.Hour).Unix())},
	}
	results := []analysis.ReadinessResult{
		{
			NodeName:               "multi-blocked",
			IsReady:                false,
			AllCookbooksCompatible: false,
			SufficientDiskSpace:    &diskBad,
			BlockingCookbooks: []analysis.BlockingCookbook{
				{Name: "cb1", Source: analysis.SourceGitCookstyle},
				{Name: "cb2", Source: analysis.SourceGitTestKitchen},
			},
		},
	}

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    params,
		ReadinessResults:  results,
		TargetChefVersion: "18.5.0",
		WarningHours:      72,
		CriticalDays:      7,
		RequiredDiskMB:    3000,
		Now:               now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nodeMetricsPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got.Fresh.BlockedTotal != 1 {
		t.Errorf("blocked_total = %d, want 1", got.Fresh.BlockedTotal)
	}
	if got.Fresh.BlockedBy.Cookstyle != 1 {
		t.Errorf("blocked_by.cookstyle = %d, want 1", got.Fresh.BlockedBy.Cookstyle)
	}
	if got.Fresh.BlockedBy.TestKitchen != 1 {
		t.Errorf("blocked_by.test_kitchen = %d, want 1", got.Fresh.BlockedBy.TestKitchen)
	}
	if got.Fresh.BlockedBy.Disk != 1 {
		t.Errorf("blocked_by.disk = %d, want 1", got.Fresh.BlockedBy.Disk)
	}
}

func TestBuildNodeMetricsPayload_EmptyVersionAndPlatform(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	diskOK := true

	params := []datastore.InsertNodeSnapshotParams{
		{NodeName: "bare", ChefVersion: "", PlatformFamily: "", OhaiTime: float64(now.Add(-1 * time.Hour).Unix())},
	}
	results := []analysis.ReadinessResult{
		{NodeName: "bare", IsReady: true, SufficientDiskSpace: &diskOK},
	}

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    params,
		ReadinessResults:  results,
		TargetChefVersion: "18.5.0",
		WarningHours:      72,
		CriticalDays:      7,
		RequiredDiskMB:    3000,
		Now:               now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nodeMetricsPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got.Fresh.ByVersion["unknown"] != 1 {
		t.Errorf("by_version[unknown] = %d, want 1", got.Fresh.ByVersion["unknown"])
	}
	if got.Fresh.ByPlatformFamily["unknown"] != 1 {
		t.Errorf("by_platform_family[unknown] = %d, want 1", got.Fresh.ByPlatformFamily["unknown"])
	}
}

func TestBuildNodeMetricsPayload_NoReadinessResultForNode(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	params := []datastore.InsertNodeSnapshotParams{
		{NodeName: "orphan", ChefVersion: "18.5.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-1 * time.Hour).Unix())},
	}

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    params,
		ReadinessResults:  []analysis.ReadinessResult{},
		TargetChefVersion: "18.5.0",
		WarningHours:      72,
		CriticalDays:      7,
		RequiredDiskMB:    3000,
		Now:               now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nodeMetricsPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Node without readiness result is treated as blocked.
	if got.Fresh.BlockedTotal != 1 {
		t.Errorf("blocked_total = %d, want 1", got.Fresh.BlockedTotal)
	}
	if got.Fresh.Ready != 0 {
		t.Errorf("ready = %d, want 0", got.Fresh.Ready)
	}
}

func TestBuildNodeMetricsPayload_ZeroOhaiTimeIsCritical(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	params := []datastore.InsertNodeSnapshotParams{
		{NodeName: "no-ohai", ChefVersion: "18.5.0", PlatformFamily: "debian", OhaiTime: 0},
	}

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    params,
		ReadinessResults:  []analysis.ReadinessResult{},
		TargetChefVersion: "18.5.0",
		WarningHours:      72,
		CriticalDays:      7,
		RequiredDiskMB:    3000,
		Now:               now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nodeMetricsPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got.ByStaleness.Critical != 1 {
		t.Errorf("by_staleness.critical = %d, want 1", got.ByStaleness.Critical)
	}
	if got.Fresh.Total != 0 {
		t.Errorf("fresh.total = %d, want 0 (node should be critical)", got.Fresh.Total)
	}
}

func TestBuildNodeMetricsPayload_UntestedCookbookCountsAsCookstyle(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	diskOK := true

	params := []datastore.InsertNodeSnapshotParams{
		{NodeName: "untested", ChefVersion: "17.0.0", PlatformFamily: "debian", OhaiTime: float64(now.Add(-1 * time.Hour).Unix())},
	}
	results := []analysis.ReadinessResult{
		{
			NodeName:               "untested",
			IsReady:                false,
			AllCookbooksCompatible: false,
			SufficientDiskSpace:    &diskOK,
			BlockingCookbooks: []analysis.BlockingCookbook{
				{Name: "cb-untested", Source: analysis.SourceNone},
			},
		},
	}

	raw, err := buildNodeMetricsPayload(nodeMetricsInput{
		SnapshotParams:    params,
		ReadinessResults:  results,
		TargetChefVersion: "18.5.0",
		WarningHours:      72,
		CriticalDays:      7,
		RequiredDiskMB:    3000,
		Now:               now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nodeMetricsPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got.Fresh.BlockedBy.Cookstyle != 1 {
		t.Errorf("blocked_by.cookstyle = %d, want 1 (untested should count as cookstyle)", got.Fresh.BlockedBy.Cookstyle)
	}
	if got.Fresh.BlockedBy.TestKitchen != 0 {
		t.Errorf("blocked_by.test_kitchen = %d, want 0", got.Fresh.BlockedBy.TestKitchen)
	}
}

// ---------------------------------------------------------------------------
// classifyBlockingCookbooks tests
// ---------------------------------------------------------------------------

func TestClassifyBlockingCookbooks_Mixed(t *testing.T) {
	blocking := []analysis.BlockingCookbook{
		{Name: "a", Source: analysis.SourceGitCookstyle},
		{Name: "b", Source: analysis.SourceGitTestKitchen},
	}
	cs, tk := classifyBlockingCookbooks(blocking)
	if !cs {
		t.Error("expected cookstyle = true")
	}
	if !tk {
		t.Error("expected test_kitchen = true")
	}
}

func TestClassifyBlockingCookbooks_CookstyleOnly(t *testing.T) {
	blocking := []analysis.BlockingCookbook{
		{Name: "a", Source: analysis.SourceServerCookstyle},
	}
	cs, tk := classifyBlockingCookbooks(blocking)
	if !cs {
		t.Error("expected cookstyle = true")
	}
	if tk {
		t.Error("expected test_kitchen = false")
	}
}

func TestClassifyBlockingCookbooks_Empty(t *testing.T) {
	cs, tk := classifyBlockingCookbooks(nil)
	if cs {
		t.Error("expected cookstyle = false for nil input")
	}
	if tk {
		t.Error("expected test_kitchen = false for nil input")
	}
}
