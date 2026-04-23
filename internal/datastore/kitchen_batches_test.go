// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCreateKitchenBatchParams_Validation(t *testing.T) {
	db := &DB{}
	_, err := db.createKitchenBatch(context.Background(), nil, CreateKitchenBatchParams{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' in error, got: %v", err)
	}
}

func TestUpdateKitchenBatchParams_Validation(t *testing.T) {
	db := &DB{}
	_, err := db.updateKitchenBatch(context.Background(), nil, "some-id", UpdateKitchenBatchParams{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' in error, got: %v", err)
	}
}

func TestBatchFilters_JSONRoundTrip(t *testing.T) {
	t.Run("all_fields_populated", func(t *testing.T) {
		hasTests := true
		original := BatchFilters{
			CookbookNames:      []string{"apache2", "nginx"},
			Platforms:          []string{"ubuntu-22.04", "centos-8"},
			ExcludeCookbooks:   []string{"legacy"},
			HasTestSuite:       &hasTests,
			PreviousStatus:     "passed",
			TargetChefVersions: []string{"18.4.12", "17.10.0"},
			IncludeExcluded:    true,
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var restored BatchFilters
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if len(restored.CookbookNames) != 2 || restored.CookbookNames[0] != "apache2" || restored.CookbookNames[1] != "nginx" {
			t.Errorf("CookbookNames mismatch: %v", restored.CookbookNames)
		}
		if len(restored.Platforms) != 2 || restored.Platforms[0] != "ubuntu-22.04" {
			t.Errorf("Platforms mismatch: %v", restored.Platforms)
		}
		if len(restored.ExcludeCookbooks) != 1 || restored.ExcludeCookbooks[0] != "legacy" {
			t.Errorf("ExcludeCookbooks mismatch: %v", restored.ExcludeCookbooks)
		}
		if restored.HasTestSuite == nil || *restored.HasTestSuite != true {
			t.Errorf("HasTestSuite mismatch: %v", restored.HasTestSuite)
		}
		if restored.PreviousStatus != "passed" {
			t.Errorf("PreviousStatus = %q, want %q", restored.PreviousStatus, "passed")
		}
		if len(restored.TargetChefVersions) != 2 || restored.TargetChefVersions[0] != "18.4.12" {
			t.Errorf("TargetChefVersions mismatch: %v", restored.TargetChefVersions)
		}
		if !restored.IncludeExcluded {
			t.Error("IncludeExcluded should be true")
		}
	})

	t.Run("empty_marshals_to_empty_object", func(t *testing.T) {
		data, err := json.Marshal(BatchFilters{})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(data) != "{}" {
			t.Errorf("empty BatchFilters marshalled to %s, want {}", string(data))
		}
	})
}

func TestBatchStatusConstants(t *testing.T) {
	statuses := []string{
		BatchStatusDraft,
		BatchStatusPreviewing,
		BatchStatusRunning,
		BatchStatusCompleted,
		BatchStatusCancelled,
	}

	for _, s := range statuses {
		if s == "" {
			t.Errorf("batch status constant is empty")
		}
	}

	seen := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate batch status constant: %q", s)
		}
		seen[s] = true
	}
}

func TestKitchenBatchColumns_NotEmpty(t *testing.T) {
	if kitchenBatchColumns == "" {
		t.Error("kitchenBatchColumns constant should not be empty")
	}
}

func TestBatchFilters_EmptyDefaults(t *testing.T) {
	var f BatchFilters
	if f.CookbookNames != nil {
		t.Errorf("CookbookNames should be nil, got %v", f.CookbookNames)
	}
	if f.Platforms != nil {
		t.Errorf("Platforms should be nil, got %v", f.Platforms)
	}
	if f.ExcludeCookbooks != nil {
		t.Errorf("ExcludeCookbooks should be nil, got %v", f.ExcludeCookbooks)
	}
	if f.HasTestSuite != nil {
		t.Errorf("HasTestSuite should be nil, got %v", f.HasTestSuite)
	}
	if f.PreviousStatus != "" {
		t.Errorf("PreviousStatus should be empty, got %q", f.PreviousStatus)
	}
	if f.TargetChefVersions != nil {
		t.Errorf("TargetChefVersions should be nil, got %v", f.TargetChefVersions)
	}
	if f.IncludeExcluded {
		t.Error("IncludeExcluded should be false")
	}
}
