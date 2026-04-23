// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestUpsertGitKitchenResultParams_Validation(t *testing.T) {
	db := &DB{}
	ctx := context.Background()

	allPopulated := UpsertGitKitchenResultParams{
		GitRepoName:       "my-repo",
		GitRepoURL:        "https://example.com/my-repo.git",
		TargetChefVersion: "18.4.12",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	}

	tests := []struct {
		name    string
		modify  func(*UpsertGitKitchenResultParams)
		wantErr string
	}{
		{
			name:    "empty git_repo_name",
			modify:  func(p *UpsertGitKitchenResultParams) { p.GitRepoName = "" },
			wantErr: "git_repo_name is required",
		},
		{
			name:    "empty git_repo_url",
			modify:  func(p *UpsertGitKitchenResultParams) { p.GitRepoURL = "" },
			wantErr: "git_repo_url is required",
		},
		{
			name:    "empty target_chef_version",
			modify:  func(p *UpsertGitKitchenResultParams) { p.TargetChefVersion = "" },
			wantErr: "target_chef_version is required",
		},
		{
			name:    "empty platform_name",
			modify:  func(p *UpsertGitKitchenResultParams) { p.PlatformName = "" },
			wantErr: "platform_name is required",
		},
		{
			name:    "empty suite_name",
			modify:  func(p *UpsertGitKitchenResultParams) { p.SuiteName = "" },
			wantErr: "suite_name is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := allPopulated
			tc.modify(&p)
			_, err := db.upsertGitKitchenResult(ctx, nil, p)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestGitKitchenResult_JSONRoundTrip(t *testing.T) {
	t.Run("all_fields_populated", func(t *testing.T) {
		convergePassed := true
		testsPassed := false
		duration := 120
		started := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
		completed := time.Date(2025, 1, 15, 10, 2, 0, 0, time.UTC)

		original := GitKitchenResult{
			ID:                "result-001",
			BatchID:           "batch-001",
			GitRepoName:       "my-repo",
			GitRepoURL:        "https://example.com/my-repo.git",
			TargetChefVersion: "18.4.12",
			CommitSHA:         "abc123def456",
			PlatformName:      "ubuntu-22.04",
			SuiteName:         "default",
			TemplateUsed:      "kitchen-dokken",
			DriverUsed:        "dokken",
			ConvergePassed:    &convergePassed,
			TestsPassed:       &testsPassed,
			TimedOut:          true,
			ConvergeOutput:    "converge ok",
			VerifyOutput:      "verify ok",
			DestroyOutput:     "destroy ok",
			DurationSeconds:   &duration,
			ErrorMessage:      "something went wrong",
			StartedAt:         &started,
			CompletedAt:       &completed,
			VMTrackingID:      "vm-001",
			CreatedAt:         time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC),
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var restored GitKitchenResult
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if restored.ID != original.ID {
			t.Errorf("ID = %q, want %q", restored.ID, original.ID)
		}
		if restored.BatchID != original.BatchID {
			t.Errorf("BatchID = %q, want %q", restored.BatchID, original.BatchID)
		}
		if restored.GitRepoName != original.GitRepoName {
			t.Errorf("GitRepoName = %q, want %q", restored.GitRepoName, original.GitRepoName)
		}
		if restored.GitRepoURL != original.GitRepoURL {
			t.Errorf("GitRepoURL = %q, want %q", restored.GitRepoURL, original.GitRepoURL)
		}
		if restored.TargetChefVersion != original.TargetChefVersion {
			t.Errorf("TargetChefVersion = %q, want %q", restored.TargetChefVersion, original.TargetChefVersion)
		}
		if restored.CommitSHA != original.CommitSHA {
			t.Errorf("CommitSHA = %q, want %q", restored.CommitSHA, original.CommitSHA)
		}
		if restored.PlatformName != original.PlatformName {
			t.Errorf("PlatformName = %q, want %q", restored.PlatformName, original.PlatformName)
		}
		if restored.SuiteName != original.SuiteName {
			t.Errorf("SuiteName = %q, want %q", restored.SuiteName, original.SuiteName)
		}
		if restored.TemplateUsed != original.TemplateUsed {
			t.Errorf("TemplateUsed = %q, want %q", restored.TemplateUsed, original.TemplateUsed)
		}
		if restored.DriverUsed != original.DriverUsed {
			t.Errorf("DriverUsed = %q, want %q", restored.DriverUsed, original.DriverUsed)
		}
		if restored.ConvergePassed == nil || *restored.ConvergePassed != true {
			t.Errorf("ConvergePassed = %v, want true", restored.ConvergePassed)
		}
		if restored.TestsPassed == nil || *restored.TestsPassed != false {
			t.Errorf("TestsPassed = %v, want false", restored.TestsPassed)
		}
		if !restored.TimedOut {
			t.Error("TimedOut should be true")
		}
		if restored.ConvergeOutput != original.ConvergeOutput {
			t.Errorf("ConvergeOutput = %q, want %q", restored.ConvergeOutput, original.ConvergeOutput)
		}
		if restored.VerifyOutput != original.VerifyOutput {
			t.Errorf("VerifyOutput = %q, want %q", restored.VerifyOutput, original.VerifyOutput)
		}
		if restored.DestroyOutput != original.DestroyOutput {
			t.Errorf("DestroyOutput = %q, want %q", restored.DestroyOutput, original.DestroyOutput)
		}
		if restored.DurationSeconds == nil || *restored.DurationSeconds != 120 {
			t.Errorf("DurationSeconds = %v, want 120", restored.DurationSeconds)
		}
		if restored.ErrorMessage != original.ErrorMessage {
			t.Errorf("ErrorMessage = %q, want %q", restored.ErrorMessage, original.ErrorMessage)
		}
		if restored.StartedAt == nil || !restored.StartedAt.Equal(started) {
			t.Errorf("StartedAt = %v, want %v", restored.StartedAt, started)
		}
		if restored.CompletedAt == nil || !restored.CompletedAt.Equal(completed) {
			t.Errorf("CompletedAt = %v, want %v", restored.CompletedAt, completed)
		}
		if restored.VMTrackingID != original.VMTrackingID {
			t.Errorf("VMTrackingID = %q, want %q", restored.VMTrackingID, original.VMTrackingID)
		}
		if !restored.CreatedAt.Equal(original.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", restored.CreatedAt, original.CreatedAt)
		}
	})

	t.Run("omitempty_fields", func(t *testing.T) {
		minimal := GitKitchenResult{
			ID:                "result-002",
			GitRepoName:       "repo",
			GitRepoURL:        "https://example.com/repo.git",
			TargetChefVersion: "18.4.12",
			PlatformName:      "ubuntu-22.04",
			SuiteName:         "default",
			CreatedAt:         time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC),
		}

		data, err := json.Marshal(minimal)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		raw := string(data)
		absentFields := []string{
			"batch_id",
			"template_used",
			"driver_used",
			"converge_output",
			"verify_output",
			"destroy_output",
			"duration_seconds",
			"error_message",
			"started_at",
			"completed_at",
			"vm_tracking_id",
		}
		for _, field := range absentFields {
			if strings.Contains(raw, field) {
				t.Errorf("expected omitempty field %q to be absent from JSON, got: %s", field, raw)
			}
		}
	})

	t.Run("nullable_bools", func(t *testing.T) {
		// nil *bool marshals as null
		r := GitKitchenResult{
			ID:                "result-003",
			GitRepoName:       "repo",
			GitRepoURL:        "https://example.com/repo.git",
			TargetChefVersion: "18.4.12",
			PlatformName:      "ubuntu-22.04",
			SuiteName:         "default",
			ConvergePassed:    nil,
			TestsPassed:       nil,
			CreatedAt:         time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC),
		}

		data, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal to map: %v", err)
		}

		if m["converge_passed"] != nil {
			t.Errorf("converge_passed should be null when nil, got %v", m["converge_passed"])
		}
		if m["tests_passed"] != nil {
			t.Errorf("tests_passed should be null when nil, got %v", m["tests_passed"])
		}

		// set *bool values and verify they marshal correctly
		trueVal := true
		falseVal := false
		r.ConvergePassed = &trueVal
		r.TestsPassed = &falseVal

		data, err = json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal with set bools: %v", err)
		}

		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal to map: %v", err)
		}

		if m["converge_passed"] != true {
			t.Errorf("converge_passed should be true, got %v", m["converge_passed"])
		}
		if m["tests_passed"] != false {
			t.Errorf("tests_passed should be false, got %v", m["tests_passed"])
		}
	})
}

func TestGkrColumns_NotEmpty(t *testing.T) {
	if gkrColumns == "" {
		t.Error("gkrColumns constant should not be empty")
	}
}

func TestGitKitchenResult_Defaults(t *testing.T) {
	var r GitKitchenResult
	if r.ConvergePassed != nil {
		t.Errorf("ConvergePassed should be nil, got %v", r.ConvergePassed)
	}
	if r.TestsPassed != nil {
		t.Errorf("TestsPassed should be nil, got %v", r.TestsPassed)
	}
	if r.TimedOut {
		t.Error("TimedOut should be false")
	}
	if r.DurationSeconds != nil {
		t.Errorf("DurationSeconds should be nil, got %v", r.DurationSeconds)
	}
	if r.StartedAt != nil {
		t.Errorf("StartedAt should be nil, got %v", r.StartedAt)
	}
	if r.CompletedAt != nil {
		t.Errorf("CompletedAt should be nil, got %v", r.CompletedAt)
	}
	if r.ID != "" {
		t.Errorf("ID should be empty, got %q", r.ID)
	}
	if r.BatchID != "" {
		t.Errorf("BatchID should be empty, got %q", r.BatchID)
	}
	if r.ErrorMessage != "" {
		t.Errorf("ErrorMessage should be empty, got %q", r.ErrorMessage)
	}
	if r.VMTrackingID != "" {
		t.Errorf("VMTrackingID should be empty, got %q", r.VMTrackingID)
	}
}
