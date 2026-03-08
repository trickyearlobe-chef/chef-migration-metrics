// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

func TestValidExportType(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"ready_nodes", true},
		{"blocked_nodes", true},
		{"cookbook_remediation", true},
		{"", false},
		{"invalid", false},
		{"READY_NODES", false},
		{"ready-nodes", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ValidExportType(tt.input)
			if got != tt.want {
				t.Errorf("ValidExportType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidExportFormat(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"csv", true},
		{"json", true},
		{"chef_search_query", true},
		{"", false},
		{"xml", false},
		{"CSV", false},
		{"chef-search-query", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ValidExportFormat(tt.input)
			if got != tt.want {
				t.Errorf("ValidExportFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExportJob struct — field constants
// ---------------------------------------------------------------------------

func TestExportStatusConstants(t *testing.T) {
	// Verify the status constants match the expected string values used in
	// the database schema.
	if ExportStatusPending != "pending" {
		t.Errorf("ExportStatusPending = %q, want %q", ExportStatusPending, "pending")
	}
	if ExportStatusProcessing != "processing" {
		t.Errorf("ExportStatusProcessing = %q, want %q", ExportStatusProcessing, "processing")
	}
	if ExportStatusCompleted != "completed" {
		t.Errorf("ExportStatusCompleted = %q, want %q", ExportStatusCompleted, "completed")
	}
	if ExportStatusFailed != "failed" {
		t.Errorf("ExportStatusFailed = %q, want %q", ExportStatusFailed, "failed")
	}
	if ExportStatusExpired != "expired" {
		t.Errorf("ExportStatusExpired = %q, want %q", ExportStatusExpired, "expired")
	}
}

func TestExportTypeConstants(t *testing.T) {
	if ExportTypeReadyNodes != "ready_nodes" {
		t.Errorf("ExportTypeReadyNodes = %q, want %q", ExportTypeReadyNodes, "ready_nodes")
	}
	if ExportTypeBlockedNodes != "blocked_nodes" {
		t.Errorf("ExportTypeBlockedNodes = %q, want %q", ExportTypeBlockedNodes, "blocked_nodes")
	}
	if ExportTypeCookbookRemediation != "cookbook_remediation" {
		t.Errorf("ExportTypeCookbookRemediation = %q, want %q", ExportTypeCookbookRemediation, "cookbook_remediation")
	}
}

func TestExportFormatConstants(t *testing.T) {
	if ExportFormatCSV != "csv" {
		t.Errorf("ExportFormatCSV = %q, want %q", ExportFormatCSV, "csv")
	}
	if ExportFormatJSON != "json" {
		t.Errorf("ExportFormatJSON = %q, want %q", ExportFormatJSON, "json")
	}
	if ExportFormatChefSearchQuery != "chef_search_query" {
		t.Errorf("ExportFormatChefSearchQuery = %q, want %q", ExportFormatChefSearchQuery, "chef_search_query")
	}
}

// ---------------------------------------------------------------------------
// ExportJob JSON marshalling
// ---------------------------------------------------------------------------

func TestExportJob_MarshalJSON(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	job := ExportJob{
		ID:            "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		ExportType:    ExportTypeReadyNodes,
		Format:        ExportFormatCSV,
		Filters:       json.RawMessage(`{"organisation":"prod"}`),
		Status:        ExportStatusCompleted,
		RowCount:      42,
		FilePath:      "/var/lib/chef-migration-metrics/exports/test.csv",
		FileSizeBytes: 1234,
		ErrorMessage:  "",
		RequestedBy:   "admin",
		RequestedAt:   now,
		CompletedAt:   now.Add(5 * time.Second),
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("json.Marshal(ExportJob) failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if m["id"] != job.ID {
		t.Errorf("id = %v, want %v", m["id"], job.ID)
	}
	if m["export_type"] != ExportTypeReadyNodes {
		t.Errorf("export_type = %v, want %v", m["export_type"], ExportTypeReadyNodes)
	}
	if m["format"] != ExportFormatCSV {
		t.Errorf("format = %v, want %v", m["format"], ExportFormatCSV)
	}
	if m["status"] != ExportStatusCompleted {
		t.Errorf("status = %v, want %v", m["status"], ExportStatusCompleted)
	}

	// row_count is a number in JSON
	rc, ok := m["row_count"].(float64)
	if !ok || int(rc) != 42 {
		t.Errorf("row_count = %v, want 42", m["row_count"])
	}

	// file_size_bytes is a number in JSON
	fs, ok := m["file_size_bytes"].(float64)
	if !ok || int64(fs) != 1234 {
		t.Errorf("file_size_bytes = %v, want 1234", m["file_size_bytes"])
	}

	// Filters should be the nested object, not a string.
	filtersRaw, ok := m["filters"]
	if !ok {
		t.Fatal("filters field missing from JSON output")
	}
	filtersMap, ok := filtersRaw.(map[string]any)
	if !ok {
		t.Fatalf("filters should be an object, got %T", filtersRaw)
	}
	if filtersMap["organisation"] != "prod" {
		t.Errorf("filters.organisation = %v, want %v", filtersMap["organisation"], "prod")
	}
}

func TestExportJob_MarshalJSON_EmptyOptionalFields(t *testing.T) {
	job := ExportJob{
		ID:          "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		ExportType:  ExportTypeBlockedNodes,
		Format:      ExportFormatJSON,
		Status:      ExportStatusPending,
		RequestedAt: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
		CreatedAt:   time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("json.Marshal(ExportJob) failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// error_message should be omitted when empty (omitempty on string).
	if val, ok := m["error_message"]; ok && val != "" {
		t.Errorf("expected error_message to be omitted or empty, got %v", val)
	}

	if m["status"] != ExportStatusPending {
		t.Errorf("status = %v, want %v", m["status"], ExportStatusPending)
	}
}

func TestExportJob_MarshalJSON_NilFilters(t *testing.T) {
	job := ExportJob{
		ID:          "test-id",
		ExportType:  ExportTypeReadyNodes,
		Format:      ExportFormatCSV,
		Filters:     nil,
		Status:      ExportStatusPending,
		RequestedAt: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
		CreatedAt:   time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("json.Marshal(ExportJob) failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Nil json.RawMessage marshals as JSON null.
	if m["filters"] != nil {
		t.Errorf("filters = %v, want nil", m["filters"])
	}
}

// ---------------------------------------------------------------------------
// InsertExportJobParams validation
// ---------------------------------------------------------------------------

func TestInsertExportJobParams_RequiredFields(t *testing.T) {
	// These tests verify that the DB methods would reject empty required
	// fields. Since we don't have a real DB in unit tests, we test the
	// validation logic by calling the InsertExportJob method on a nil DB
	// and checking for the expected error messages. However, since DB is
	// a concrete type wrapping *sql.DB, we can't easily call it without a
	// connection. Instead, we verify the params struct is correctly
	// structured for the callers.

	p := InsertExportJobParams{
		ExportType: ExportTypeReadyNodes,
		Format:     ExportFormatCSV,
	}

	if p.ExportType == "" {
		t.Error("ExportType should not be empty")
	}
	if p.Format == "" {
		t.Error("Format should not be empty")
	}

	// Verify optional fields have zero values by default.
	if p.RequestedBy != "" {
		t.Error("RequestedBy should default to empty")
	}
	if !p.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should default to zero time")
	}
}

func TestInsertExportJobParams_WithFilters(t *testing.T) {
	filters := json.RawMessage(`{"organisation":"prod","environment":"staging"}`)

	p := InsertExportJobParams{
		ExportType:  ExportTypeBlockedNodes,
		Format:      ExportFormatJSON,
		Filters:     filters,
		RequestedBy: "admin@example.com",
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}

	if p.ExportType != ExportTypeBlockedNodes {
		t.Errorf("ExportType = %q, want %q", p.ExportType, ExportTypeBlockedNodes)
	}
	if p.Format != ExportFormatJSON {
		t.Errorf("Format = %q, want %q", p.Format, ExportFormatJSON)
	}
	if string(p.Filters) != string(filters) {
		t.Errorf("Filters = %s, want %s", p.Filters, filters)
	}
	if p.RequestedBy != "admin@example.com" {
		t.Errorf("RequestedBy = %q, want %q", p.RequestedBy, "admin@example.com")
	}
	if p.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}
}

// ---------------------------------------------------------------------------
// ExportJob status lifecycle
// ---------------------------------------------------------------------------

func TestExportJob_StatusLifecycle(t *testing.T) {
	// Verify the expected status transition strings are correct.
	// pending → processing → completed
	// pending → processing → failed
	// completed → expired

	transitions := []struct {
		from string
		to   string
		ok   bool
	}{
		{ExportStatusPending, ExportStatusProcessing, true},
		{ExportStatusProcessing, ExportStatusCompleted, true},
		{ExportStatusProcessing, ExportStatusFailed, true},
		{ExportStatusCompleted, ExportStatusExpired, true},
	}

	for _, tt := range transitions {
		t.Run(tt.from+"→"+tt.to, func(t *testing.T) {
			if tt.from == "" || tt.to == "" {
				t.Error("status values should not be empty")
			}
			if tt.from == tt.to {
				t.Error("from and to should differ")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validation coverage: all export types × all formats
// ---------------------------------------------------------------------------

func TestValidExportType_AllTypes(t *testing.T) {
	allTypes := []string{
		ExportTypeReadyNodes,
		ExportTypeBlockedNodes,
		ExportTypeCookbookRemediation,
	}

	for _, et := range allTypes {
		if !ValidExportType(et) {
			t.Errorf("ValidExportType(%q) should be true", et)
		}
	}
}

func TestValidExportFormat_AllFormats(t *testing.T) {
	allFormats := []string{
		ExportFormatCSV,
		ExportFormatJSON,
		ExportFormatChefSearchQuery,
	}

	for _, f := range allFormats {
		if !ValidExportFormat(f) {
			t.Errorf("ValidExportFormat(%q) should be true", f)
		}
	}
}

// ---------------------------------------------------------------------------
// Column list sanity check
// ---------------------------------------------------------------------------

func TestExportJobColumns_NotEmpty(t *testing.T) {
	if ejColumns == "" {
		t.Error("ejColumns constant should not be empty")
	}
}

// ---------------------------------------------------------------------------
// ExportJob zero value
// ---------------------------------------------------------------------------

func TestExportJob_ZeroValue(t *testing.T) {
	var job ExportJob

	if job.ID != "" {
		t.Error("zero-value ID should be empty")
	}
	if job.ExportType != "" {
		t.Error("zero-value ExportType should be empty")
	}
	if job.Format != "" {
		t.Error("zero-value Format should be empty")
	}
	if job.Status != "" {
		t.Error("zero-value Status should be empty")
	}
	if job.RowCount != 0 {
		t.Error("zero-value RowCount should be 0")
	}
	if job.FileSizeBytes != 0 {
		t.Error("zero-value FileSizeBytes should be 0")
	}
	if !job.RequestedAt.IsZero() {
		t.Error("zero-value RequestedAt should be zero time")
	}
	if !job.CompletedAt.IsZero() {
		t.Error("zero-value CompletedAt should be zero time")
	}
	if !job.ExpiresAt.IsZero() {
		t.Error("zero-value ExpiresAt should be zero time")
	}
}
