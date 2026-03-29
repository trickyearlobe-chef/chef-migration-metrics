// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// validateAnalysisParams — pure function tests
// ---------------------------------------------------------------------------

func TestValidateAnalysisParams_Valid(t *testing.T) {
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationID:  "org-1",
		CollectionRunID: "run-1",
		AnalysedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAnalysisParams_MissingOrganisationID(t *testing.T) {
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		CollectionRunID: "run-1",
		AnalysedAt:      time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for missing organisation ID")
	}
	if got := err.Error(); got != "datastore: organisation ID is required for cookbook usage analysis" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAnalysisParams_MissingCollectionRunID(t *testing.T) {
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationID: "org-1",
		AnalysedAt:     time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for missing collection run ID")
	}
	if got := err.Error(); got != "datastore: collection run ID is required for cookbook usage analysis" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAnalysisParams_MissingAnalysedAt(t *testing.T) {
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationID:  "org-1",
		CollectionRunID: "run-1",
	})
	if err == nil {
		t.Fatal("expected error for missing analysed_at")
	}
	if got := err.Error(); got != "datastore: analysed_at timestamp is required for cookbook usage analysis" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAnalysisParams_ValidationOrder(t *testing.T) {
	// All fields missing — should fail on organisation ID first.
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: organisation ID is required for cookbook usage analysis" {
		t.Errorf("expected organisation ID error first, got: %v", err)
	}

	// Organisation ID present — should fail on collection run ID.
	err = validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationID: "org-1",
	})
	if err == nil {
		t.Fatal("expected error for missing collection run ID")
	}
	if got := err.Error(); got != "datastore: collection run ID is required for cookbook usage analysis" {
		t.Errorf("expected collection run ID error, got: %v", err)
	}

	// Organisation ID + collection run ID — should fail on analysed_at.
	err = validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationID:  "org-1",
		CollectionRunID: "run-1",
	})
	if err == nil {
		t.Fatal("expected error for missing analysed_at")
	}
	if got := err.Error(); got != "datastore: analysed_at timestamp is required for cookbook usage analysis" {
		t.Errorf("expected analysed_at error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// validateDetailParams — pure function tests
// ---------------------------------------------------------------------------

func TestValidateDetailParams_Valid(t *testing.T) {
	err := validateDetailParams(InsertCookbookUsageDetailParams{
		AnalysisID:      "analysis-1",
		OrganisationID:  "org-1",
		CookbookName:    "apt",
		CookbookVersion: "7.4.0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDetailParams_MissingAnalysisID(t *testing.T) {
	err := validateDetailParams(InsertCookbookUsageDetailParams{
		OrganisationID:  "org-1",
		CookbookName:    "apt",
		CookbookVersion: "7.4.0",
	})
	if err == nil {
		t.Fatal("expected error for missing analysis ID")
	}
	if got := err.Error(); got != "datastore: analysis ID is required for cookbook usage detail" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDetailParams_MissingOrganisationID(t *testing.T) {
	err := validateDetailParams(InsertCookbookUsageDetailParams{
		AnalysisID:      "analysis-1",
		CookbookName:    "apt",
		CookbookVersion: "7.4.0",
	})
	if err == nil {
		t.Fatal("expected error for missing organisation ID")
	}
	if got := err.Error(); got != "datastore: organisation ID is required for cookbook usage detail" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDetailParams_MissingCookbookName(t *testing.T) {
	err := validateDetailParams(InsertCookbookUsageDetailParams{
		AnalysisID:      "analysis-1",
		OrganisationID:  "org-1",
		CookbookVersion: "7.4.0",
	})
	if err == nil {
		t.Fatal("expected error for missing cookbook name")
	}
	if got := err.Error(); got != "datastore: cookbook name is required for cookbook usage detail" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDetailParams_MissingCookbookVersion(t *testing.T) {
	err := validateDetailParams(InsertCookbookUsageDetailParams{
		AnalysisID:     "analysis-1",
		OrganisationID: "org-1",
		CookbookName:   "apt",
	})
	if err == nil {
		t.Fatal("expected error for missing cookbook version")
	}
	if got := err.Error(); got != "datastore: cookbook version is required for cookbook usage detail" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDetailParams_ValidationOrder(t *testing.T) {
	// All fields missing — should fail on analysis ID first.
	err := validateDetailParams(InsertCookbookUsageDetailParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: analysis ID is required for cookbook usage detail" {
		t.Errorf("expected analysis ID error first, got: %v", err)
	}

	// Analysis ID present — should fail on organisation ID.
	err = validateDetailParams(InsertCookbookUsageDetailParams{
		AnalysisID: "analysis-1",
	})
	if err == nil {
		t.Fatal("expected error for missing organisation ID")
	}
	if got := err.Error(); got != "datastore: organisation ID is required for cookbook usage detail" {
		t.Errorf("expected organisation ID error, got: %v", err)
	}

	// Analysis ID + organisation ID — should fail on cookbook name.
	err = validateDetailParams(InsertCookbookUsageDetailParams{
		AnalysisID:     "analysis-1",
		OrganisationID: "org-1",
	})
	if err == nil {
		t.Fatal("expected error for missing cookbook name")
	}
	if got := err.Error(); got != "datastore: cookbook name is required for cookbook usage detail" {
		t.Errorf("expected cookbook name error, got: %v", err)
	}

	// Analysis ID + organisation ID + cookbook name — should fail on cookbook version.
	err = validateDetailParams(InsertCookbookUsageDetailParams{
		AnalysisID:     "analysis-1",
		OrganisationID: "org-1",
		CookbookName:   "apt",
	})
	if err == nil {
		t.Fatal("expected error for missing cookbook version")
	}
	if got := err.Error(); got != "datastore: cookbook version is required for cookbook usage detail" {
		t.Errorf("expected cookbook version error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// InsertCookbookUsageAnalysisParams — zero-value defaults
// ---------------------------------------------------------------------------

func TestInsertCookbookUsageAnalysisParams_Defaults(t *testing.T) {
	var p InsertCookbookUsageAnalysisParams
	if p.OrganisationID != "" {
		t.Errorf("zero-value OrganisationID should be empty, got %q", p.OrganisationID)
	}
	if p.CollectionRunID != "" {
		t.Errorf("zero-value CollectionRunID should be empty, got %q", p.CollectionRunID)
	}
	if !p.AnalysedAt.IsZero() {
		t.Error("zero-value AnalysedAt should be zero time")
	}
}

// ---------------------------------------------------------------------------
// InsertCookbookUsageDetailParams — zero-value defaults
// ---------------------------------------------------------------------------

func TestInsertCookbookUsageDetailParams_Defaults(t *testing.T) {
	var p InsertCookbookUsageDetailParams
	if p.AnalysisID != "" {
		t.Errorf("zero-value AnalysisID should be empty, got %q", p.AnalysisID)
	}
	if p.OrganisationID != "" {
		t.Errorf("zero-value OrganisationID should be empty, got %q", p.OrganisationID)
	}
	if p.CookbookName != "" {
		t.Errorf("zero-value CookbookName should be empty, got %q", p.CookbookName)
	}
	if p.CookbookVersion != "" {
		t.Errorf("zero-value CookbookVersion should be empty, got %q", p.CookbookVersion)
	}
	if p.NodeCount != 0 {
		t.Errorf("zero-value NodeCount should be 0, got %d", p.NodeCount)
	}
	if p.IsActive {
		t.Error("zero-value IsActive should be false")
	}
}
