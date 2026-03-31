// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"testing"
)

func TestPurgeStaleTargetVersionData_EmptyVersions_NoOp(t *testing.T) {
	// Empty activeVersions should be a no-op safety check — never delete everything.
	db := &DB{pool: nil}
	result, err := db.PurgeStaleTargetVersionData(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total() != 0 {
		t.Errorf("expected 0 total, got %d", result.Total())
	}
}

func TestPurgeStaleTargetVersionData_EmptySlice_NoOp(t *testing.T) {
	db := &DB{pool: nil}
	result, err := db.PurgeStaleTargetVersionData(context.Background(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total() != 0 {
		t.Errorf("expected 0 total, got %d", result.Total())
	}
}

func TestPurgeStaleTargetVersionResult_Total(t *testing.T) {
	r := PurgeStaleTargetVersionResult{
		NodeReadiness:                     10,
		ServerCookbookCookstyleResults:    20,
		ServerCookbookComplexity:          5,
		ServerCookbookAutocorrectPreviews: 3,
		GitRepoCookstyleResults:           7,
		GitRepoComplexity:                 2,
		GitRepoAutocorrectPreviews:        1,
		GitRepoTestKitchenResults:         4,
	}
	if r.Total() != 52 {
		t.Errorf("expected total 52, got %d", r.Total())
	}
}

func TestPurgeStaleTargetVersionResult_Total_Zero(t *testing.T) {
	r := PurgeStaleTargetVersionResult{}
	if r.Total() != 0 {
		t.Errorf("expected total 0, got %d", r.Total())
	}
}
