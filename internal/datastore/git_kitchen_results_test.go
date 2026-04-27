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
		passed := true
		duration := 120
		started := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
		completed := time.Date(2025, 1, 15, 10, 2, 0, 0, time.UTC)

		original := GitKitchenResult{
			ID:                "result-001",
			GitRepoName:       "my-repo",
			GitRepoURL:        "https://example.com/my-repo.git",
			TargetChefVersion: "18.4.12",
			CommitSHA:         "abc123def456",
			PlatformName:      "ubuntu-22.04",
			SuiteName:         "default",
			InstanceName:      "default-ubuntu-2204",
			DriverUsed:        "dokken",
			Passed:            &passed,
			TimedOut:          true,
			Output:            "kitchen test output",
			DurationSeconds:   &duration,
			ErrorMessage:      "something went wrong",
			StartedAt:         &started,
			CompletedAt:       &completed,
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
		if restored.GitRepoName != original.GitRepoName {
			t.Errorf("GitRepoName = %q, want %q", restored.GitRepoName, original.GitRepoName)
		}
		if restored.InstanceName != original.InstanceName {
			t.Errorf("InstanceName = %q, want %q", restored.InstanceName, original.InstanceName)
		}
		if restored.DriverUsed != original.DriverUsed {
			t.Errorf("DriverUsed = %q, want %q", restored.DriverUsed, original.DriverUsed)
		}
		if restored.Passed == nil || *restored.Passed != true {
			t.Errorf("Passed = %v, want true", restored.Passed)
		}
		if !restored.TimedOut {
			t.Error("TimedOut should be true")
		}
		if restored.Output != original.Output {
			t.Errorf("Output = %q, want %q", restored.Output, original.Output)
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
			"driver_used",
			"output",
			"duration_seconds",
			"error_message",
			"started_at",
			"completed_at",
		}
		for _, field := range absentFields {
			if strings.Contains(raw, field) {
				t.Errorf("expected omitempty field %q to be absent from JSON, got: %s", field, raw)
			}
		}
	})

	t.Run("nullable_passed", func(t *testing.T) {
		r := GitKitchenResult{
			ID:                "result-003",
			GitRepoName:       "repo",
			GitRepoURL:        "https://example.com/repo.git",
			TargetChefVersion: "18.4.12",
			PlatformName:      "ubuntu-22.04",
			SuiteName:         "default",
			Passed:            nil,
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

		if m["passed"] != nil {
			t.Errorf("passed should be null when nil, got %v", m["passed"])
		}

		trueVal := true
		r.Passed = &trueVal

		data, err = json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal with set bool: %v", err)
		}

		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal to map: %v", err)
		}

		if m["passed"] != true {
			t.Errorf("passed should be true, got %v", m["passed"])
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
	if r.Passed != nil {
		t.Errorf("Passed should be nil, got %v", r.Passed)
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
	if r.ErrorMessage != "" {
		t.Errorf("ErrorMessage should be empty, got %q", r.ErrorMessage)
	}
}
