// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// validParams returns a fully populated UpsertNodeKitchenRunParams suitable
// for tests that need a valid baseline.
// ---------------------------------------------------------------------------

func validNodeKitchenRunParams(t *testing.T) UpsertNodeKitchenRunParams {
	t.Helper()
	return UpsertNodeKitchenRunParams{
		NodeName:          "web01.example.com",
		OrganisationName:  "myorg",
		TargetChefVersion: "18.4.12",
		CookbookSource:    CookbookSourceServer,
		PlatformName:      "ubuntu-22.04",
	}
}

// ---------------------------------------------------------------------------
// validateUpsertNodeKitchenRunParams — required field checks
// ---------------------------------------------------------------------------

func TestValidateUpsertNodeKitchenRunParams_Valid(t *testing.T) {
	if err := validateUpsertNodeKitchenRunParams(validNodeKitchenRunParams(t)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateUpsertNodeKitchenRunParams_MissingNodeName(t *testing.T) {
	p := validNodeKitchenRunParams(t)
	p.NodeName = ""
	err := validateUpsertNodeKitchenRunParams(p)
	if err == nil {
		t.Fatal("expected error for missing node_name")
	}
	if got := err.Error(); got != "datastore: node_name is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateUpsertNodeKitchenRunParams_MissingOrganisationName(t *testing.T) {
	p := validNodeKitchenRunParams(t)
	p.OrganisationName = ""
	err := validateUpsertNodeKitchenRunParams(p)
	if err == nil {
		t.Fatal("expected error for missing organisation_name")
	}
	if got := err.Error(); got != "datastore: organisation_name is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateUpsertNodeKitchenRunParams_MissingTargetChefVersion(t *testing.T) {
	p := validNodeKitchenRunParams(t)
	p.TargetChefVersion = ""
	err := validateUpsertNodeKitchenRunParams(p)
	if err == nil {
		t.Fatal("expected error for missing target_chef_version")
	}
	if got := err.Error(); got != "datastore: target_chef_version is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateUpsertNodeKitchenRunParams_MissingPlatformName(t *testing.T) {
	p := validNodeKitchenRunParams(t)
	p.PlatformName = ""
	err := validateUpsertNodeKitchenRunParams(p)
	if err == nil {
		t.Fatal("expected error for missing platform_name")
	}
	if got := err.Error(); got != "datastore: platform_name is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateUpsertNodeKitchenRunParams_InvalidCookbookSource(t *testing.T) {
	for _, src := range []string{"", "policyfile", "supermarket", "SERVER", "unknown"} {
		t.Run(src, func(t *testing.T) {
			p := validNodeKitchenRunParams(t)
			p.CookbookSource = src
			err := validateUpsertNodeKitchenRunParams(p)
			if err == nil {
				t.Fatalf("expected error for cookbook_source %q", src)
			}
			if !strings.Contains(err.Error(), src) {
				t.Errorf("error should contain source value %q, got: %v", src, err)
			}
		})
	}
}

func TestValidateUpsertNodeKitchenRunParams_ValidCookbookSources(t *testing.T) {
	for _, src := range []string{CookbookSourceServer, CookbookSourceGit, CookbookSourceHybrid} {
		t.Run(src, func(t *testing.T) {
			p := validNodeKitchenRunParams(t)
			p.CookbookSource = src
			if err := validateUpsertNodeKitchenRunParams(p); err != nil {
				t.Errorf("unexpected error for cookbook_source %q: %v", src, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// jsonWithDefault
// ---------------------------------------------------------------------------

func TestJsonWithDefault(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		def  string
		want string
	}{
		{"nil_returns_default_array", nil, "[]", "[]"},
		{"nil_returns_default_object", nil, "{}", "{}"},
		{"empty_returns_default", json.RawMessage{}, "[]", "[]"},
		{"non_empty_returns_input", json.RawMessage(`{"a":1}`), "[]", `{"a":1}`},
		{"array_input_returned", json.RawMessage(`["x"]`), "[]", `["x"]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(jsonWithDefault(tc.data, tc.def))
			if got != tc.want {
				t.Errorf("jsonWithDefault(%v, %q) = %q, want %q", tc.data, tc.def, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// boolPtrFromNull
// ---------------------------------------------------------------------------

func TestBoolPtrFromNull(t *testing.T) {
	t.Run("null_returns_nil", func(t *testing.T) {
		got := boolPtrFromNull(sql.NullBool{})
		if got != nil {
			t.Errorf("expected nil, got %v", *got)
		}
	})
	t.Run("valid_true", func(t *testing.T) {
		got := boolPtrFromNull(sql.NullBool{Bool: true, Valid: true})
		if got == nil {
			t.Fatal("expected non-nil pointer")
		}
		if *got != true {
			t.Errorf("expected true, got %v", *got)
		}
	})
	t.Run("valid_false", func(t *testing.T) {
		got := boolPtrFromNull(sql.NullBool{Bool: false, Valid: true})
		if got == nil {
			t.Fatal("expected non-nil pointer")
		}
		if *got != false {
			t.Errorf("expected false, got %v", *got)
		}
	})
}

// ---------------------------------------------------------------------------
// intPtrFromNull
// ---------------------------------------------------------------------------

func TestIntPtrFromNull(t *testing.T) {
	t.Run("null_returns_nil", func(t *testing.T) {
		got := intPtrFromNull(sql.NullInt64{})
		if got != nil {
			t.Errorf("expected nil, got %v", *got)
		}
	})
	t.Run("valid_positive", func(t *testing.T) {
		got := intPtrFromNull(sql.NullInt64{Int64: 42, Valid: true})
		if got == nil {
			t.Fatal("expected non-nil pointer")
		}
		if *got != 42 {
			t.Errorf("expected 42, got %d", *got)
		}
	})
	t.Run("valid_zero", func(t *testing.T) {
		got := intPtrFromNull(sql.NullInt64{Int64: 0, Valid: true})
		if got == nil {
			t.Fatal("expected non-nil pointer")
		}
		if *got != 0 {
			t.Errorf("expected 0, got %d", *got)
		}
	})
}

// ---------------------------------------------------------------------------
// nullTimePtr
// ---------------------------------------------------------------------------

func TestNullTimePtr(t *testing.T) {
	t.Run("nil_returns_invalid", func(t *testing.T) {
		got := nullTimePtr(nil)
		if got.Valid {
			t.Errorf("expected invalid NullTime, got valid %v", got.Time)
		}
	})
	t.Run("non_nil_returns_valid", func(t *testing.T) {
		now := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)
		got := nullTimePtr(&now)
		if !got.Valid {
			t.Fatal("expected valid NullTime")
		}
		if !got.Time.Equal(now) {
			t.Errorf("time = %v, want %v", got.Time, now)
		}
	})
}

// ---------------------------------------------------------------------------
// timePtrFromNull
// ---------------------------------------------------------------------------

func TestTimePtrFromNull(t *testing.T) {
	t.Run("null_returns_nil", func(t *testing.T) {
		got := timePtrFromNull(sql.NullTime{})
		if got != nil {
			t.Errorf("expected nil, got %v", *got)
		}
	})
	t.Run("valid_returns_pointer", func(t *testing.T) {
		now := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)
		got := timePtrFromNull(sql.NullTime{Time: now, Valid: true})
		if got == nil {
			t.Fatal("expected non-nil pointer")
		}
		if !got.Equal(now) {
			t.Errorf("time = %v, want %v", *got, now)
		}
	})
}

// ---------------------------------------------------------------------------
// Round-trip: value → NullX → value
// ---------------------------------------------------------------------------

func TestNullTimePtr_RoundTrip(t *testing.T) {
	now := time.Date(2025, 7, 1, 15, 30, 0, 0, time.UTC)
	got := timePtrFromNull(nullTimePtr(&now))
	if got == nil {
		t.Fatal("expected non-nil after round-trip")
	}
	if !got.Equal(now) {
		t.Errorf("round-trip time = %v, want %v", *got, now)
	}
}

func TestNullTimePtr_RoundTrip_Nil(t *testing.T) {
	got := timePtrFromNull(nullTimePtr(nil))
	if got != nil {
		t.Errorf("expected nil after nil round-trip, got %v", *got)
	}
}

func TestBoolPtrFromNull_RoundTrip(t *testing.T) {
	v := true
	nb := nullBoolPtr(&v)
	got := boolPtrFromNull(nb)
	if got == nil || *got != true {
		t.Errorf("round-trip bool failed, got %v", got)
	}
}

func TestIntPtrFromNull_RoundTrip(t *testing.T) {
	v := 99
	ni := nullIntPtr(&v)
	got := intPtrFromNull(ni)
	if got == nil || *got != 99 {
		t.Errorf("round-trip int failed, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Cookbook source constants and map completeness
// ---------------------------------------------------------------------------

func TestValidCookbookSources_Completeness(t *testing.T) {
	expected := []string{CookbookSourceServer, CookbookSourceGit, CookbookSourceHybrid}
	if len(validCookbookSources) != len(expected) {
		t.Errorf("validCookbookSources has %d entries, want %d", len(validCookbookSources), len(expected))
	}
	for _, s := range expected {
		if !validCookbookSources[s] {
			t.Errorf("validCookbookSources missing %q", s)
		}
	}
}

func TestValidCookbookSources_InvalidNotPresent(t *testing.T) {
	for _, s := range []string{"", "policyfile", "supermarket", "SERVER"} {
		if validCookbookSources[s] {
			t.Errorf("validCookbookSources should not contain %q", s)
		}
	}
}

func TestCookbookSourceConstants_Values(t *testing.T) {
	if CookbookSourceServer != "server" {
		t.Errorf("CookbookSourceServer = %q, want %q", CookbookSourceServer, "server")
	}
	if CookbookSourceGit != "git" {
		t.Errorf("CookbookSourceGit = %q, want %q", CookbookSourceGit, "git")
	}
	if CookbookSourceHybrid != "hybrid" {
		t.Errorf("CookbookSourceHybrid = %q, want %q", CookbookSourceHybrid, "hybrid")
	}
}

// ---------------------------------------------------------------------------
// nodeKitchenRunColumns sanity check
// ---------------------------------------------------------------------------

func TestNodeKitchenRunColumns_NotEmpty(t *testing.T) {
	if nodeKitchenRunColumns == "" {
		t.Error("nodeKitchenRunColumns constant should not be empty")
	}
}
