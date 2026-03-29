// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"encoding/json"
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// ProductionPlatformRow — struct basics
// ---------------------------------------------------------------------------

func TestProductionPlatformRow_ZeroValue(t *testing.T) {
	var r ProductionPlatformRow

	if r.Platform != "" {
		t.Errorf("zero-value Platform should be empty, got %q", r.Platform)
	}
	if r.PlatformVersion != "" {
		t.Errorf("zero-value PlatformVersion should be empty, got %q", r.PlatformVersion)
	}
	if r.PlatformFamily != "" {
		t.Errorf("zero-value PlatformFamily should be empty, got %q", r.PlatformFamily)
	}
	if r.NodeCount != 0 {
		t.Errorf("zero-value NodeCount should be 0, got %d", r.NodeCount)
	}
}

func TestProductionPlatformRow_FieldAssignment(t *testing.T) {
	r := ProductionPlatformRow{
		Platform:        "ubuntu",
		PlatformVersion: "22.04",
		PlatformFamily:  "debian",
		NodeCount:       47,
	}

	if r.Platform != "ubuntu" {
		t.Errorf("Platform = %q, want %q", r.Platform, "ubuntu")
	}
	if r.PlatformVersion != "22.04" {
		t.Errorf("PlatformVersion = %q, want %q", r.PlatformVersion, "22.04")
	}
	if r.PlatformFamily != "debian" {
		t.Errorf("PlatformFamily = %q, want %q", r.PlatformFamily, "debian")
	}
	if r.NodeCount != 47 {
		t.Errorf("NodeCount = %d, want %d", r.NodeCount, 47)
	}
}

// ---------------------------------------------------------------------------
// ProductionPlatformRow — JSON marshalling
// ---------------------------------------------------------------------------

func TestProductionPlatformRow_JSONRoundTrip(t *testing.T) {
	original := ProductionPlatformRow{
		Platform:        "centos",
		PlatformVersion: "7.9.2009",
		PlatformFamily:  "rhel",
		NodeCount:       12,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal(ProductionPlatformRow) failed: %v", err)
	}

	var decoded ProductionPlatformRow
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(ProductionPlatformRow) failed: %v", err)
	}

	if decoded.Platform != original.Platform {
		t.Errorf("Platform = %q, want %q", decoded.Platform, original.Platform)
	}
	if decoded.PlatformVersion != original.PlatformVersion {
		t.Errorf("PlatformVersion = %q, want %q", decoded.PlatformVersion, original.PlatformVersion)
	}
	if decoded.PlatformFamily != original.PlatformFamily {
		t.Errorf("PlatformFamily = %q, want %q", decoded.PlatformFamily, original.PlatformFamily)
	}
	if decoded.NodeCount != original.NodeCount {
		t.Errorf("NodeCount = %d, want %d", decoded.NodeCount, original.NodeCount)
	}
}

// ---------------------------------------------------------------------------
// GetProductionPlatformsForCookbook — input validation
// ---------------------------------------------------------------------------

func TestGetProductionPlatformsForCookbook_EmptyCookbookName(t *testing.T) {
	db := &DB{pool: nil}

	_, err := db.GetProductionPlatformsForCookbook(nil, "")
	if err == nil {
		t.Fatal("expected error for empty cookbook name, got nil")
	}

	want := "datastore: cookbook_name is required"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// scanProductionPlatformRow — via mock scanner
// ---------------------------------------------------------------------------

// mockRowScanner implements the Scan(dest ...any) error interface for testing
// the row scanning helper without a real database.
type mockRowScanner struct {
	values []any
	err    error
}

func (m *mockRowScanner) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *string:
			*ptr = m.values[i].(string)
		case *int:
			*ptr = m.values[i].(int)
		}
	}
	return nil
}

func TestScanProductionPlatformRow_Success(t *testing.T) {
	scanner := &mockRowScanner{
		values: []any{"rocky", "9.3", "rhel", 8},
	}

	r, err := scanProductionPlatformRow(scanner)
	if err != nil {
		t.Fatalf("scanProductionPlatformRow failed: %v", err)
	}

	if r.Platform != "rocky" {
		t.Errorf("Platform = %q, want %q", r.Platform, "rocky")
	}
	if r.PlatformVersion != "9.3" {
		t.Errorf("PlatformVersion = %q, want %q", r.PlatformVersion, "9.3")
	}
	if r.PlatformFamily != "rhel" {
		t.Errorf("PlatformFamily = %q, want %q", r.PlatformFamily, "rhel")
	}
	if r.NodeCount != 8 {
		t.Errorf("NodeCount = %d, want %d", r.NodeCount, 8)
	}
}

func TestScanProductionPlatformRow_Error(t *testing.T) {
	scanErr := fmt.Errorf("scan failed")
	scanner := &mockRowScanner{err: scanErr}

	_, err := scanProductionPlatformRow(scanner)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != scanErr {
		t.Errorf("error = %v, want %v", err, scanErr)
	}
}

// ---------------------------------------------------------------------------
// Slice equality helper
// ---------------------------------------------------------------------------

func TestProductionPlatformRow_SliceEquality(t *testing.T) {
	a := []ProductionPlatformRow{
		{Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian", NodeCount: 47},
		{Platform: "centos", PlatformVersion: "7.9.2009", PlatformFamily: "rhel", NodeCount: 12},
		{Platform: "rocky", PlatformVersion: "9.3", PlatformFamily: "rhel", NodeCount: 8},
	}
	b := []ProductionPlatformRow{
		{Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian", NodeCount: 47},
		{Platform: "centos", PlatformVersion: "7.9.2009", PlatformFamily: "rhel", NodeCount: 12},
		{Platform: "rocky", PlatformVersion: "9.3", PlatformFamily: "rhel", NodeCount: 8},
	}

	if len(a) != len(b) {
		t.Fatalf("slice lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("index %d: got %+v, want %+v", i, a[i], b[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Edge case: empty platform fields
// ---------------------------------------------------------------------------

func TestProductionPlatformRow_EmptyFields(t *testing.T) {
	r := ProductionPlatformRow{
		Platform:        "",
		PlatformVersion: "",
		PlatformFamily:  "",
		NodeCount:       1,
	}

	if r.Platform != "" {
		t.Errorf("Platform = %q, want empty", r.Platform)
	}
	if r.PlatformVersion != "" {
		t.Errorf("PlatformVersion = %q, want empty", r.PlatformVersion)
	}
	if r.PlatformFamily != "" {
		t.Errorf("PlatformFamily = %q, want empty", r.PlatformFamily)
	}
	if r.NodeCount != 1 {
		t.Errorf("NodeCount = %d, want %d", r.NodeCount, 1)
	}
}
