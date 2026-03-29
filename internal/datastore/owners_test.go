// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// parseBlockingCookbookNames — pure function tests
// ---------------------------------------------------------------------------

func TestParseBlockingCookbookNames_LegacyStringArray(t *testing.T) {
	raw := []byte(`["apt","nginx","users"]`)
	names := parseBlockingCookbookNames(raw)
	if len(names) != 3 {
		t.Fatalf("len = %d, want 3", len(names))
	}
	want := []string{"apt", "nginx", "users"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("names[%d] = %q, want %q", i, names[i], w)
		}
	}
}

func TestParseBlockingCookbookNames_StructuredArray(t *testing.T) {
	raw := []byte(`[{"name":"apt","version":"7.4.0","reason":"incompatible"},{"name":"nginx","version":"2.0.0","reason":"untested"}]`)
	names := parseBlockingCookbookNames(raw)
	if len(names) != 2 {
		t.Fatalf("len = %d, want 2", len(names))
	}
	if names[0] != "apt" {
		t.Errorf("names[0] = %q, want %q", names[0], "apt")
	}
	if names[1] != "nginx" {
		t.Errorf("names[1] = %q, want %q", names[1], "nginx")
	}
}

func TestParseBlockingCookbookNames_StructuredArray_SkipsEmptyNames(t *testing.T) {
	raw := []byte(`[{"name":"apt"},{"name":""},{"name":"nginx"}]`)
	names := parseBlockingCookbookNames(raw)
	if len(names) != 2 {
		t.Fatalf("len = %d, want 2", len(names))
	}
	if names[0] != "apt" {
		t.Errorf("names[0] = %q, want %q", names[0], "apt")
	}
	if names[1] != "nginx" {
		t.Errorf("names[1] = %q, want %q", names[1], "nginx")
	}
}

func TestParseBlockingCookbookNames_EmptyArray(t *testing.T) {
	raw := []byte(`[]`)
	names := parseBlockingCookbookNames(raw)
	if len(names) != 0 {
		t.Errorf("len = %d, want 0", len(names))
	}
}

func TestParseBlockingCookbookNames_Nil(t *testing.T) {
	names := parseBlockingCookbookNames(nil)
	if names != nil {
		t.Errorf("expected nil, got %v", names)
	}
}

func TestParseBlockingCookbookNames_EmptyBytes(t *testing.T) {
	names := parseBlockingCookbookNames([]byte{})
	if names != nil {
		t.Errorf("expected nil, got %v", names)
	}
}

func TestParseBlockingCookbookNames_InvalidJSON(t *testing.T) {
	names := parseBlockingCookbookNames([]byte(`not json`))
	if names != nil {
		t.Errorf("expected nil for invalid JSON, got %v", names)
	}
}

func TestParseBlockingCookbookNames_SingleElement(t *testing.T) {
	raw := []byte(`["apt"]`)
	names := parseBlockingCookbookNames(raw)
	if len(names) != 1 {
		t.Fatalf("len = %d, want 1", len(names))
	}
	if names[0] != "apt" {
		t.Errorf("names[0] = %q, want %q", names[0], "apt")
	}
}

func TestParseBlockingCookbookNames_StructuredWithExtraFields(t *testing.T) {
	raw := []byte(`[{"name":"apt","version":"7.4.0","reason":"incompatible","source":"chef_server","complexity_score":35,"complexity_label":"high"}]`)
	names := parseBlockingCookbookNames(raw)
	if len(names) != 1 {
		t.Fatalf("len = %d, want 1", len(names))
	}
	if names[0] != "apt" {
		t.Errorf("names[0] = %q, want %q", names[0], "apt")
	}
}

// ---------------------------------------------------------------------------
// InsertOwner — parameter validation
// ---------------------------------------------------------------------------

func TestInsertOwner_MissingName(t *testing.T) {
	db := &DB{}
	_, err := db.insertOwner(context.TODO(), nil, InsertOwnerParams{
		OwnerType: "team",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if got := err.Error(); got != "datastore: owner name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInsertOwner_MissingOwnerType(t *testing.T) {
	db := &DB{}
	_, err := db.insertOwner(context.TODO(), nil, InsertOwnerParams{
		Name: "team-a",
	})
	if err == nil {
		t.Fatal("expected error for missing owner_type")
	}
	if got := err.Error(); got != "datastore: owner_type is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInsertOwner_ValidationOrder(t *testing.T) {
	db := &DB{}

	// All fields missing — should fail on name first.
	_, err := db.insertOwner(context.TODO(), nil, InsertOwnerParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: owner name is required" {
		t.Errorf("expected name error first, got: %v", err)
	}

	// Name present — should fail on owner_type.
	_, err = db.insertOwner(context.TODO(), nil, InsertOwnerParams{
		Name: "team-a",
	})
	if err == nil {
		t.Fatal("expected error for missing owner_type")
	}
	if got := err.Error(); got != "datastore: owner_type is required" {
		t.Errorf("expected owner_type error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// InsertOwnerParams — zero-value defaults
// ---------------------------------------------------------------------------

func TestInsertOwnerParams_Defaults(t *testing.T) {
	var p InsertOwnerParams
	if p.Name != "" {
		t.Errorf("zero-value Name should be empty, got %q", p.Name)
	}
	if p.OwnerType != "" {
		t.Errorf("zero-value OwnerType should be empty, got %q", p.OwnerType)
	}
	if p.ContactEmail != "" {
		t.Errorf("zero-value ContactEmail should be empty, got %q", p.ContactEmail)
	}
	if p.ContactChannel != "" {
		t.Errorf("zero-value ContactChannel should be empty, got %q", p.ContactChannel)
	}
}

// ---------------------------------------------------------------------------
// Owner struct — zero value
// ---------------------------------------------------------------------------

func TestOwner_ZeroValue(t *testing.T) {
	var o Owner
	if o.Name != "" {
		t.Errorf("zero-value Name should be empty, got %q", o.Name)
	}
	if o.OwnerType != "" {
		t.Errorf("zero-value OwnerType should be empty, got %q", o.OwnerType)
	}
}
