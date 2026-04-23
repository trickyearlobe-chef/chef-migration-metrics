// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// validateKitchenAnalysisParams — pure function tests
// ---------------------------------------------------------------------------

func TestValidateKitchenAnalysisParams_Valid(t *testing.T) {
	err := validateKitchenAnalysisParams(UpsertKitchenAnalysisResultParams{
		GitRepoName:   "my-cookbook",
		GitRepoURL:    "https://example.com/my-cookbook.git",
		HeadCommitSHA: "abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateKitchenAnalysisParams_MissingName(t *testing.T) {
	err := validateKitchenAnalysisParams(UpsertKitchenAnalysisResultParams{
		GitRepoURL:    "https://example.com/my-cookbook.git",
		HeadCommitSHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for missing git_repo_name")
	}
	if got := err.Error(); got != "datastore: git_repo_name is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateKitchenAnalysisParams_MissingURL(t *testing.T) {
	err := validateKitchenAnalysisParams(UpsertKitchenAnalysisResultParams{
		GitRepoName:   "my-cookbook",
		HeadCommitSHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for missing git_repo_url")
	}
	if got := err.Error(); got != "datastore: git_repo_url is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateKitchenAnalysisParams_MissingCommitSHA(t *testing.T) {
	err := validateKitchenAnalysisParams(UpsertKitchenAnalysisResultParams{
		GitRepoName: "my-cookbook",
		GitRepoURL:  "https://example.com/my-cookbook.git",
	})
	if err == nil {
		t.Fatal("expected error for missing head_commit_sha")
	}
	if got := err.Error(); got != "datastore: head_commit_sha is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateKitchenAnalysisParams_ValidationOrder(t *testing.T) {
	// All fields missing — should fail on git_repo_name first.
	err := validateKitchenAnalysisParams(UpsertKitchenAnalysisResultParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: git_repo_name is required" {
		t.Errorf("expected git_repo_name error first, got: %v", got)
	}

	// Name present — should fail on git_repo_url.
	err = validateKitchenAnalysisParams(UpsertKitchenAnalysisResultParams{
		GitRepoName: "my-cookbook",
	})
	if err == nil {
		t.Fatal("expected error for missing git_repo_url")
	}
	if got := err.Error(); got != "datastore: git_repo_url is required" {
		t.Errorf("expected git_repo_url error, got: %v", got)
	}

	// Name + URL present — should fail on head_commit_sha.
	err = validateKitchenAnalysisParams(UpsertKitchenAnalysisResultParams{
		GitRepoName: "my-cookbook",
		GitRepoURL:  "https://example.com/my-cookbook.git",
	})
	if err == nil {
		t.Fatal("expected error for missing head_commit_sha")
	}
	if got := err.Error(); got != "datastore: head_commit_sha is required" {
		t.Errorf("expected head_commit_sha error, got: %v", got)
	}
}

// ---------------------------------------------------------------------------
// KitchenAnalysisResult — JSON marshalling
// ---------------------------------------------------------------------------

func TestKitchenAnalysisResult_MarshalJSON(t *testing.T) {
	omnibus := true
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	r := KitchenAnalysisResult{
		GitRepoName:        "my-cookbook",
		GitRepoURL:         "https://example.com/my-cookbook.git",
		AnalysedAt:         now,
		HeadCommitSHA:      "abc123def456",
		KitchenFiles:       json.RawMessage(`[".kitchen.yml"]`),
		HasLocalOverride:   true,
		LocalOverrideKeys:  json.RawMessage(`["driver","transport"]`),
		DriverName:         "vagrant",
		ProvisionerName:    "chef_zero",
		RequireChefOmnibus: &omnibus,
		Platforms:          json.RawMessage(`[{"name":"ubuntu-20.04"}]`),
		Suites:             json.RawMessage(`[{"name":"default"}]`),
		TransportType:      "ssh",
		Extensions:         json.RawMessage(`{"x-custom":"value"}`),
		VariantFiles:       json.RawMessage(`[".kitchen.vagrant.yml"]`),
		ErrorMessage:       "",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if m["git_repo_name"] != "my-cookbook" {
		t.Errorf("git_repo_name = %v, want my-cookbook", m["git_repo_name"])
	}
	if m["git_repo_url"] != "https://example.com/my-cookbook.git" {
		t.Errorf("git_repo_url = %v, want https://example.com/my-cookbook.git", m["git_repo_url"])
	}
	if m["head_commit_sha"] != "abc123def456" {
		t.Errorf("head_commit_sha = %v, want abc123def456", m["head_commit_sha"])
	}
	if m["has_local_override"] != true {
		t.Errorf("has_local_override = %v, want true", m["has_local_override"])
	}
	if m["driver_name"] != "vagrant" {
		t.Errorf("driver_name = %v, want vagrant", m["driver_name"])
	}
	if m["provisioner_name"] != "chef_zero" {
		t.Errorf("provisioner_name = %v, want chef_zero", m["provisioner_name"])
	}
	if m["require_chef_omnibus"] != true {
		t.Errorf("require_chef_omnibus = %v, want true", m["require_chef_omnibus"])
	}
	if m["transport_type"] != "ssh" {
		t.Errorf("transport_type = %v, want ssh", m["transport_type"])
	}

	// JSONB fields should be arrays/objects, not strings.
	if files, ok := m["kitchen_files"].([]any); !ok || len(files) != 1 {
		t.Errorf("kitchen_files = %v, want array with 1 element", m["kitchen_files"])
	}
	if platforms, ok := m["platforms"].([]any); !ok || len(platforms) != 1 {
		t.Errorf("platforms = %v, want array with 1 element", m["platforms"])
	}
	if suites, ok := m["suites"].([]any); !ok || len(suites) != 1 {
		t.Errorf("suites = %v, want array with 1 element", m["suites"])
	}
	if keys, ok := m["local_override_keys"].([]any); !ok || len(keys) != 2 {
		t.Errorf("local_override_keys = %v, want array with 2 elements", m["local_override_keys"])
	}
	if ext, ok := m["extensions"].(map[string]any); !ok || ext["x-custom"] != "value" {
		t.Errorf("extensions = %v, want {x-custom: value}", m["extensions"])
	}
}

func TestKitchenAnalysisResult_MarshalJSON_OmitEmpty(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	r := KitchenAnalysisResult{
		GitRepoName:   "minimal",
		GitRepoURL:    "https://example.com/minimal.git",
		AnalysedAt:    now,
		HeadCommitSHA: "deadbeef",
		KitchenFiles:  json.RawMessage(`[]`),
		Platforms:     json.RawMessage(`[]`),
		Suites:        json.RawMessage(`[]`),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Fields with omitempty should be absent when zero/nil.
	for _, key := range []string{"local_override_keys", "extensions", "variant_files", "error_message"} {
		if v, ok := m[key]; ok && v != nil && v != "" {
			t.Errorf("%s should be omitted or empty, got %v", key, v)
		}
	}

	// require_chef_omnibus should be omitted when nil.
	if _, ok := m["require_chef_omnibus"]; ok {
		t.Errorf("require_chef_omnibus should be omitted when nil, got %v", m["require_chef_omnibus"])
	}
}

func TestKitchenAnalysisResult_MarshalJSON_NilOmnibus(t *testing.T) {
	r := KitchenAnalysisResult{
		GitRepoName:        "test",
		GitRepoURL:         "https://example.com/test.git",
		HeadCommitSHA:      "abc",
		KitchenFiles:       json.RawMessage(`[]`),
		RequireChefOmnibus: nil,
		Platforms:          json.RawMessage(`[]`),
		Suites:             json.RawMessage(`[]`),
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if _, exists := m["require_chef_omnibus"]; exists {
		t.Errorf("require_chef_omnibus should be omitted when nil")
	}
}

func TestKitchenAnalysisResult_UnmarshalJSON(t *testing.T) {
	input := `{
		"git_repo_name": "test",
		"git_repo_url": "https://example.com/test.git",
		"analysed_at": "2025-01-15T10:30:00Z",
		"head_commit_sha": "abc123",
		"kitchen_files": [".kitchen.yml"],
		"has_local_override": false,
		"driver_name": "dokken",
		"platforms": [{"name":"centos-7"}],
		"suites": [{"name":"default","run_list":["recipe[test]"]}],
		"require_chef_omnibus": false,
		"created_at": "2025-01-15T10:30:00Z",
		"updated_at": "2025-01-15T10:30:00Z"
	}`

	var r KitchenAnalysisResult
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if r.GitRepoName != "test" {
		t.Errorf("GitRepoName = %q, want test", r.GitRepoName)
	}
	if r.DriverName != "dokken" {
		t.Errorf("DriverName = %q, want dokken", r.DriverName)
	}
	if r.RequireChefOmnibus == nil {
		t.Fatal("RequireChefOmnibus should not be nil")
	}
	if *r.RequireChefOmnibus != false {
		t.Errorf("RequireChefOmnibus = %v, want false", *r.RequireChefOmnibus)
	}
	if string(r.Platforms) != `[{"name":"centos-7"}]` {
		t.Errorf("Platforms = %s, want [{\"name\":\"centos-7\"}]", r.Platforms)
	}
}

// ---------------------------------------------------------------------------
// KitchenDiscoveredPlatform — JSON marshalling
// ---------------------------------------------------------------------------

func TestKitchenDiscoveredPlatform_MarshalJSON(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	p := KitchenDiscoveredPlatform{
		PlatformName:     "ubuntu-20.04",
		NormalisedName:   "ubuntu-20.04",
		OSFamily:         "debian",
		OSVersion:        "20.04",
		CookbookCount:    15,
		HasExtensions:    true,
		CommonExtensions: json.RawMessage(`{"x-custom":"value"}`),
		TransportType:    "ssh",
		UpdatedAt:        now,
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if m["platform_name"] != "ubuntu-20.04" {
		t.Errorf("platform_name = %v, want ubuntu-20.04", m["platform_name"])
	}
	if m["normalised_name"] != "ubuntu-20.04" {
		t.Errorf("normalised_name = %v, want ubuntu-20.04", m["normalised_name"])
	}
	if m["os_family"] != "debian" {
		t.Errorf("os_family = %v, want debian", m["os_family"])
	}
	if m["os_version"] != "20.04" {
		t.Errorf("os_version = %v, want 20.04", m["os_version"])
	}
	if m["cookbook_count"] != float64(15) {
		t.Errorf("cookbook_count = %v, want 15", m["cookbook_count"])
	}
	if m["has_extensions"] != true {
		t.Errorf("has_extensions = %v, want true", m["has_extensions"])
	}
	if m["transport_type"] != "ssh" {
		t.Errorf("transport_type = %v, want ssh", m["transport_type"])
	}
	if ext, ok := m["common_extensions"].(map[string]any); !ok || ext["x-custom"] != "value" {
		t.Errorf("common_extensions = %v, want {x-custom: value}", m["common_extensions"])
	}
}

func TestKitchenDiscoveredPlatform_MarshalJSON_OmitEmpty(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	p := KitchenDiscoveredPlatform{
		PlatformName:   "custom-platform",
		NormalisedName: "custom-platform",
		OSFamily:       "other",
		CookbookCount:  1,
		UpdatedAt:      now,
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	for _, key := range []string{"os_version", "common_extensions", "transport_type"} {
		if v, ok := m[key]; ok && v != nil && v != "" {
			t.Errorf("%s should be omitted or empty, got %v", key, v)
		}
	}
}

// ---------------------------------------------------------------------------
// KitchenAnalysisSummary — JSON marshalling
// ---------------------------------------------------------------------------

func TestKitchenAnalysisSummary_MarshalJSON(t *testing.T) {
	s := KitchenAnalysisSummary{
		TotalScanned:           50,
		TotalWithoutKitchen:    10,
		TotalWithLocalOverride: 5,
		TotalWithConflicts:     2,
		DriverCounts: map[string]int{
			"vagrant": 30,
			"dokken":  15,
			"ec2":     5,
		},
		TransportCounts: map[string]int{
			"ssh":   40,
			"winrm": 10,
		},
		ProvisionerCounts: map[string]int{
			"chef_zero": 45,
			"chef_solo": 5,
		},
		PlatformCount: 25,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if m["total_scanned"] != float64(50) {
		t.Errorf("total_scanned = %v, want 50", m["total_scanned"])
	}
	if m["total_without_kitchen"] != float64(10) {
		t.Errorf("total_without_kitchen = %v, want 10", m["total_without_kitchen"])
	}
	if m["total_with_local_override"] != float64(5) {
		t.Errorf("total_with_local_override = %v, want 5", m["total_with_local_override"])
	}
	if m["total_with_conflicts"] != float64(2) {
		t.Errorf("total_with_conflicts = %v, want 2", m["total_with_conflicts"])
	}
	if m["platform_count"] != float64(25) {
		t.Errorf("platform_count = %v, want 25", m["platform_count"])
	}

	drivers, ok := m["driver_counts"].(map[string]any)
	if !ok {
		t.Fatalf("driver_counts is not a map: %T", m["driver_counts"])
	}
	if drivers["vagrant"] != float64(30) {
		t.Errorf("driver_counts[vagrant] = %v, want 30", drivers["vagrant"])
	}
	if drivers["dokken"] != float64(15) {
		t.Errorf("driver_counts[dokken] = %v, want 15", drivers["dokken"])
	}
	if drivers["ec2"] != float64(5) {
		t.Errorf("driver_counts[ec2] = %v, want 5", drivers["ec2"])
	}

	transports, ok := m["transport_counts"].(map[string]any)
	if !ok {
		t.Fatalf("transport_counts is not a map: %T", m["transport_counts"])
	}
	if transports["ssh"] != float64(40) {
		t.Errorf("transport_counts[ssh] = %v, want 40", transports["ssh"])
	}
	if transports["winrm"] != float64(10) {
		t.Errorf("transport_counts[winrm] = %v, want 10", transports["winrm"])
	}

	provisioners, ok := m["provisioner_counts"].(map[string]any)
	if !ok {
		t.Fatalf("provisioner_counts is not a map: %T", m["provisioner_counts"])
	}
	if provisioners["chef_zero"] != float64(45) {
		t.Errorf("provisioner_counts[chef_zero] = %v, want 45", provisioners["chef_zero"])
	}
	if provisioners["chef_solo"] != float64(5) {
		t.Errorf("provisioner_counts[chef_solo] = %v, want 5", provisioners["chef_solo"])
	}
}

func TestKitchenAnalysisSummary_MarshalJSON_EmptyMaps(t *testing.T) {
	s := KitchenAnalysisSummary{
		DriverCounts:      map[string]int{},
		TransportCounts:   map[string]int{},
		ProvisionerCounts: map[string]int{},
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Empty maps should serialise as empty objects, not null.
	for _, key := range []string{"driver_counts", "transport_counts", "provisioner_counts"} {
		v, ok := m[key]
		if !ok {
			t.Errorf("%s missing from JSON output", key)
			continue
		}
		obj, ok := v.(map[string]any)
		if !ok {
			t.Errorf("%s = %T, want map", key, v)
			continue
		}
		if len(obj) != 0 {
			t.Errorf("%s has %d entries, want 0", key, len(obj))
		}
	}
}

func TestKitchenAnalysisSummary_UnmarshalJSON(t *testing.T) {
	input := `{
		"total_scanned": 100,
		"total_without_kitchen": 20,
		"total_with_local_override": 8,
		"total_with_conflicts": 3,
		"driver_counts": {"vagrant": 60, "dokken": 40},
		"transport_counts": {"ssh": 80, "winrm": 20},
		"provisioner_counts": {"chef_zero": 100},
		"platform_count": 42
	}`

	var s KitchenAnalysisSummary
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if s.TotalScanned != 100 {
		t.Errorf("TotalScanned = %d, want 100", s.TotalScanned)
	}
	if s.TotalWithoutKitchen != 20 {
		t.Errorf("TotalWithoutKitchen = %d, want 20", s.TotalWithoutKitchen)
	}
	if s.TotalWithLocalOverride != 8 {
		t.Errorf("TotalWithLocalOverride = %d, want 8", s.TotalWithLocalOverride)
	}
	if s.TotalWithConflicts != 3 {
		t.Errorf("TotalWithConflicts = %d, want 3", s.TotalWithConflicts)
	}
	if s.PlatformCount != 42 {
		t.Errorf("PlatformCount = %d, want 42", s.PlatformCount)
	}
	if s.DriverCounts["vagrant"] != 60 {
		t.Errorf("DriverCounts[vagrant] = %d, want 60", s.DriverCounts["vagrant"])
	}
	if s.DriverCounts["dokken"] != 40 {
		t.Errorf("DriverCounts[dokken] = %d, want 40", s.DriverCounts["dokken"])
	}
	if s.TransportCounts["ssh"] != 80 {
		t.Errorf("TransportCounts[ssh] = %d, want 80", s.TransportCounts["ssh"])
	}
	if s.ProvisionerCounts["chef_zero"] != 100 {
		t.Errorf("ProvisionerCounts[chef_zero] = %d, want 100", s.ProvisionerCounts["chef_zero"])
	}
}

// ---------------------------------------------------------------------------
// UpsertKitchenAnalysisResultParams — zero-value defaults
// ---------------------------------------------------------------------------

func TestUpsertKitchenAnalysisResultParams_Defaults(t *testing.T) {
	var p UpsertKitchenAnalysisResultParams
	if p.GitRepoName != "" {
		t.Errorf("zero-value GitRepoName should be empty, got %q", p.GitRepoName)
	}
	if p.GitRepoURL != "" {
		t.Errorf("zero-value GitRepoURL should be empty, got %q", p.GitRepoURL)
	}
	if p.HeadCommitSHA != "" {
		t.Errorf("zero-value HeadCommitSHA should be empty, got %q", p.HeadCommitSHA)
	}
	if !p.AnalysedAt.IsZero() {
		t.Error("zero-value AnalysedAt should be zero time")
	}
	if p.HasLocalOverride {
		t.Error("zero-value HasLocalOverride should be false")
	}
	if p.RequireChefOmnibus != nil {
		t.Error("zero-value RequireChefOmnibus should be nil")
	}
	if p.KitchenFiles != nil {
		t.Error("zero-value KitchenFiles should be nil")
	}
	if p.Platforms != nil {
		t.Error("zero-value Platforms should be nil")
	}
	if p.Suites != nil {
		t.Error("zero-value Suites should be nil")
	}
}
