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
		OrganisationName: "org-1",
		CollectionRunOrg: "run-1",
		AnalysedAt:       time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAnalysisParams_MissingOrganisationName(t *testing.T) {
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		CollectionRunOrg: "run-1",
		AnalysedAt:       time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for missing organisation name")
	}
	if got := err.Error(); got != "datastore: organisation name is required for cookbook usage analysis" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAnalysisParams_MissingCollectionRunOrg(t *testing.T) {
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationName: "org-1",
		AnalysedAt:       time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for missing collection run org")
	}
	if got := err.Error(); got != "datastore: collection run org is required for cookbook usage analysis" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAnalysisParams_MissingAnalysedAt(t *testing.T) {
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationName: "org-1",
		CollectionRunOrg: "run-1",
	})
	if err == nil {
		t.Fatal("expected error for missing analysed_at")
	}
	if got := err.Error(); got != "datastore: analysed_at timestamp is required for cookbook usage analysis" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAnalysisParams_ValidationOrder(t *testing.T) {
	// All fields missing — should fail on organisation name first.
	err := validateAnalysisParams(InsertCookbookUsageAnalysisParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: organisation name is required for cookbook usage analysis" {
		t.Errorf("expected organisation name error first, got: %v", err)
	}

	// Organisation name present — should fail on collection run org.
	err = validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationName: "org-1",
	})
	if err == nil {
		t.Fatal("expected error for missing collection run org")
	}
	if got := err.Error(); got != "datastore: collection run org is required for cookbook usage analysis" {
		t.Errorf("expected collection run org error, got: %v", err)
	}

	// Organisation name + collection run org — should fail on analysed_at.
	err = validateAnalysisParams(InsertCookbookUsageAnalysisParams{
		OrganisationName: "org-1",
		CollectionRunOrg: "run-1",
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
		OrganisationName: "org-1",
		CookbookName:     "apt",
		CookbookVersion:  "7.4.0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDetailParams_MissingOrganisationName(t *testing.T) {
	err := validateDetailParams(InsertCookbookUsageDetailParams{
		CookbookName:    "apt",
		CookbookVersion: "7.4.0",
	})
	if err == nil {
		t.Fatal("expected error for missing organisation name")
	}
	if got := err.Error(); got != "datastore: organisation name is required for cookbook usage detail" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDetailParams_MissingCookbookName(t *testing.T) {
	err := validateDetailParams(InsertCookbookUsageDetailParams{
		OrganisationName: "org-1",
		CookbookVersion:  "7.4.0",
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
		OrganisationName: "org-1",
		CookbookName:     "apt",
	})
	if err == nil {
		t.Fatal("expected error for missing cookbook version")
	}
	if got := err.Error(); got != "datastore: cookbook version is required for cookbook usage detail" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDetailParams_ValidationOrder(t *testing.T) {
	// All fields missing — should fail on organisation name first.
	err := validateDetailParams(InsertCookbookUsageDetailParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: organisation name is required for cookbook usage detail" {
		t.Errorf("expected organisation name error first, got: %v", err)
	}

	// Organisation name present — should fail on cookbook name.
	err = validateDetailParams(InsertCookbookUsageDetailParams{
		OrganisationName: "org-1",
	})
	if err == nil {
		t.Fatal("expected error for missing cookbook name")
	}
	if got := err.Error(); got != "datastore: cookbook name is required for cookbook usage detail" {
		t.Errorf("expected cookbook name error, got: %v", err)
	}

	// Organisation name + cookbook name — should fail on cookbook version.
	err = validateDetailParams(InsertCookbookUsageDetailParams{
		OrganisationName: "org-1",
		CookbookName:     "apt",
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
	if p.OrganisationName != "" {
		t.Errorf("zero-value OrganisationName should be empty, got %q", p.OrganisationName)
	}
	if p.CollectionRunOrg != "" {
		t.Errorf("zero-value CollectionRunOrg should be empty, got %q", p.CollectionRunOrg)
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
	if p.OrganisationName != "" {
		t.Errorf("zero-value OrganisationName should be empty, got %q", p.OrganisationName)
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
