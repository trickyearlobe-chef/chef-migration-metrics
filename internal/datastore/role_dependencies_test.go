// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"testing"
)

// ---------------------------------------------------------------------------
// validateRoleDependencyParams — pure function tests
// ---------------------------------------------------------------------------

func TestValidateRoleDependencyParams_Valid_Cookbook(t *testing.T) {
	err := validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
		RoleName:         "webserver",
		DependencyType:   "cookbook",
		DependencyName:   "apache2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRoleDependencyParams_Valid_Role(t *testing.T) {
	err := validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
		RoleName:         "webserver",
		DependencyType:   "role",
		DependencyName:   "base",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRoleDependencyParams_MissingOrganisationName(t *testing.T) {
	err := validateRoleDependencyParams(InsertRoleDependencyParams{
		RoleName:       "webserver",
		DependencyType: "cookbook",
		DependencyName: "apache2",
	})
	if err == nil {
		t.Fatal("expected error for missing organisation name")
	}
	if got := err.Error(); got != "datastore: organisation name is required for role dependency" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRoleDependencyParams_MissingRoleName(t *testing.T) {
	err := validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
		DependencyType:   "cookbook",
		DependencyName:   "apache2",
	})
	if err == nil {
		t.Fatal("expected error for missing role name")
	}
	if got := err.Error(); got != "datastore: role name is required for role dependency" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRoleDependencyParams_InvalidDependencyType(t *testing.T) {
	err := validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
		RoleName:         "webserver",
		DependencyType:   "environment",
		DependencyName:   "production",
	})
	if err == nil {
		t.Fatal("expected error for invalid dependency type")
	}
	expected := `datastore: dependency type must be 'role' or 'cookbook', got "environment"`
	if got := err.Error(); got != expected {
		t.Errorf("unexpected error: %q, want %q", got, expected)
	}
}

func TestValidateRoleDependencyParams_EmptyDependencyType(t *testing.T) {
	err := validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
		RoleName:         "webserver",
		DependencyName:   "apache2",
	})
	if err == nil {
		t.Fatal("expected error for empty dependency type")
	}
	expected := `datastore: dependency type must be 'role' or 'cookbook', got ""`
	if got := err.Error(); got != expected {
		t.Errorf("unexpected error: %q, want %q", got, expected)
	}
}

func TestValidateRoleDependencyParams_MissingDependencyName(t *testing.T) {
	err := validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
		RoleName:         "webserver",
		DependencyType:   "cookbook",
	})
	if err == nil {
		t.Fatal("expected error for missing dependency name")
	}
	if got := err.Error(); got != "datastore: dependency name is required for role dependency" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRoleDependencyParams_ValidationOrder(t *testing.T) {
	// All fields missing — should fail on organisation name first.
	err := validateRoleDependencyParams(InsertRoleDependencyParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: organisation name is required for role dependency" {
		t.Errorf("expected organisation name error first, got: %v", err)
	}

	// Organisation name present — should fail on role name.
	err = validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
	})
	if err == nil {
		t.Fatal("expected error for missing role name")
	}
	if got := err.Error(); got != "datastore: role name is required for role dependency" {
		t.Errorf("expected role name error, got: %v", err)
	}

	// Organisation name + role name — should fail on dependency type.
	err = validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
		RoleName:         "webserver",
	})
	if err == nil {
		t.Fatal("expected error for missing dependency type")
	}
	expected := `datastore: dependency type must be 'role' or 'cookbook', got ""`
	if got := err.Error(); got != expected {
		t.Errorf("expected dependency type error, got: %v", err)
	}

	// Organisation name + role name + dependency type — should fail on dependency name.
	err = validateRoleDependencyParams(InsertRoleDependencyParams{
		OrganisationName: "org-1",
		RoleName:         "webserver",
		DependencyType:   "role",
	})
	if err == nil {
		t.Fatal("expected error for missing dependency name")
	}
	if got := err.Error(); got != "datastore: dependency name is required for role dependency" {
		t.Errorf("expected dependency name error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// InsertRoleDependencyParams — zero-value defaults
// ---------------------------------------------------------------------------

func TestInsertRoleDependencyParams_Defaults(t *testing.T) {
	var p InsertRoleDependencyParams
	if p.OrganisationName != "" {
		t.Errorf("zero-value OrganisationName should be empty, got %q", p.OrganisationName)
	}
	if p.RoleName != "" {
		t.Errorf("zero-value RoleName should be empty, got %q", p.RoleName)
	}
	if p.DependencyType != "" {
		t.Errorf("zero-value DependencyType should be empty, got %q", p.DependencyType)
	}
	if p.DependencyName != "" {
		t.Errorf("zero-value DependencyName should be empty, got %q", p.DependencyName)
	}
}

// ---------------------------------------------------------------------------
// RoleDependency struct — zero value
// ---------------------------------------------------------------------------

func TestRoleDependency_ZeroValue(t *testing.T) {
	var rd RoleDependency
	if rd.OrganisationName != "" {
		t.Errorf("zero-value OrganisationName should be empty, got %q", rd.OrganisationName)
	}
	if rd.RoleName != "" {
		t.Errorf("zero-value RoleName should be empty, got %q", rd.RoleName)
	}
	if rd.DependencyType != "" {
		t.Errorf("zero-value DependencyType should be empty, got %q", rd.DependencyType)
	}
	if rd.DependencyName != "" {
		t.Errorf("zero-value DependencyName should be empty, got %q", rd.DependencyName)
	}
}
