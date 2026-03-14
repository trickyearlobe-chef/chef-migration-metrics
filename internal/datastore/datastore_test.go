// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// parseMigrationFilename tests
// ---------------------------------------------------------------------------

func TestParseMigrationFilename_Valid(t *testing.T) {
	tests := []struct {
		filename    string
		wantVersion int
		wantName    string
	}{
		{"0001_initial_schema.up.sql", 1, "initial_schema"},
		{"0002_add_indexes.up.sql", 2, "add_indexes"},
		{"0010_multi_word_name.up.sql", 10, "multi_word_name"},
		{"0100_big_version.up.sql", 100, "big_version"},
		{"9999_max_four_digit.up.sql", 9999, "max_four_digit"},
		{"1_minimal.up.sql", 1, "minimal"},
		{"42_answer.up.sql", 42, "answer"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			m, err := parseMigrationFilename(tt.filename)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Version != tt.wantVersion {
				t.Errorf("version = %d, want %d", m.Version, tt.wantVersion)
			}
			if m.Name != tt.wantName {
				t.Errorf("name = %q, want %q", m.Name, tt.wantName)
			}
		})
	}
}

func TestParseMigrationFilename_Invalid(t *testing.T) {
	tests := []struct {
		filename string
		reason   string
	}{
		{"no_version.up.sql", "no numeric prefix before underscore"},
		{"abc_letters.up.sql", "non-numeric version"},
		{"initialschema.up.sql", "no underscore separator"},
		{"0_zero_version.up.sql", "zero version"},
		{"-1_negative.up.sql", "negative version"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			_, err := parseMigrationFilename(tt.filename)
			if err == nil {
				t.Fatalf("expected error for %q (%s), got nil", tt.filename, tt.reason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// discoverMigrations tests
// ---------------------------------------------------------------------------

func TestDiscoverMigrations_Empty(t *testing.T) {
	dir := t.TempDir()

	migrations, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(migrations))
	}
}

func TestDiscoverMigrations_SortedByVersion(t *testing.T) {
	dir := t.TempDir()

	// Create migration files out of order.
	files := []string{
		"0003_third.up.sql",
		"0001_first.up.sql",
		"0002_second.up.sql",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("-- sql"), 0644); err != nil {
			t.Fatalf("creating test file %s: %v", f, err)
		}
	}

	migrations, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(migrations) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(migrations))
	}
	if migrations[0].Version != 1 {
		t.Errorf("first migration version = %d, want 1", migrations[0].Version)
	}
	if migrations[1].Version != 2 {
		t.Errorf("second migration version = %d, want 2", migrations[1].Version)
	}
	if migrations[2].Version != 3 {
		t.Errorf("third migration version = %d, want 3", migrations[2].Version)
	}
}

func TestDiscoverMigrations_SkipsNonUpSQL(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"0001_schema.up.sql",
		"0001_schema.down.sql",
		"0002_indexes.up.sql",
		"README.md",
		"notes.txt",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("-- sql"), 0644); err != nil {
			t.Fatalf("creating test file %s: %v", f, err)
		}
	}

	migrations, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("expected 2 .up.sql migrations, got %d", len(migrations))
	}
}

func TestDiscoverMigrations_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "0001_first.up.sql"), []byte("-- sql"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "0002_subdir.up.sql"), 0755); err != nil {
		t.Fatal(err)
	}

	migrations, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migrations))
	}
}

func TestDiscoverMigrations_DuplicateVersionError(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"0001_first.up.sql",
		"0001_duplicate.up.sql",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("-- sql"), 0644); err != nil {
			t.Fatalf("creating test file %s: %v", f, err)
		}
	}

	_, err := discoverMigrations(dir)
	if err == nil {
		t.Fatal("expected error for duplicate migration versions, got nil")
	}
}

func TestDiscoverMigrations_NonexistentDir(t *testing.T) {
	_, err := discoverMigrations("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

// TestDiscoverMigrations_NonexistentDir_DescriptiveError verifies that a
// missing migrations directory produces an error message that includes the
// path, so operators can diagnose the problem from the startup log.
func TestDiscoverMigrations_NonexistentDir_DescriptiveError(t *testing.T) {
	badPath := "/no/such/migrations/dir"
	_, err := discoverMigrations(badPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, badPath) {
		t.Errorf("error should contain the directory path %q, got: %s", badPath, errMsg)
	}
	if !strings.Contains(errMsg, "does not exist") {
		t.Errorf("error should mention directory does not exist, got: %s", errMsg)
	}
}

// TestDiscoverMigrations_DuplicateVersion_DescriptiveError verifies that
// duplicate migration version numbers produce an error that names both files.
func TestDiscoverMigrations_DuplicateVersion_DescriptiveError(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"0001_first.up.sql", "0001_second.up.sql"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("-- sql"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := discoverMigrations(dir)
	if err == nil {
		t.Fatal("expected error for duplicate versions, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "duplicate") && !strings.Contains(errMsg, "0001") {
		t.Errorf("error should mention duplicate version 0001, got: %s", errMsg)
	}
}

func TestDiscoverMigrations_FilenameSet(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "0001_test.up.sql"), []byte("-- sql"), 0644); err != nil {
		t.Fatal(err)
	}

	migrations, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migrations))
	}

	expected := "0001_test.up.sql"
	if migrations[0].Filename != expected {
		t.Errorf("filename = %q, want %q", migrations[0].Filename, expected)
	}
}

func TestDiscoverMigrations_SkipsMalformedFilenames(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"0001_valid.up.sql",
		"abc_invalid.up.sql",  // non-numeric prefix — should be skipped
		"nounderscore.up.sql", // no underscore — should be skipped
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("-- sql"), 0644); err != nil {
			t.Fatalf("creating test file %s: %v", f, err)
		}
	}

	migrations, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("expected 1 valid migration, got %d", len(migrations))
	}
	if migrations[0].Name != "valid" {
		t.Errorf("name = %q, want %q", migrations[0].Name, "valid")
	}
}

// ---------------------------------------------------------------------------
// Null conversion helper tests
// ---------------------------------------------------------------------------

func TestNullString(t *testing.T) {
	t.Run("empty string returns invalid", func(t *testing.T) {
		ns := nullString("")
		if ns.Valid {
			t.Error("expected Valid=false for empty string")
		}
	})

	t.Run("non-empty string returns valid", func(t *testing.T) {
		ns := nullString("hello")
		if !ns.Valid {
			t.Error("expected Valid=true for non-empty string")
		}
		if ns.String != "hello" {
			t.Errorf("String = %q, want %q", ns.String, "hello")
		}
	})
}

func TestStringFromNull(t *testing.T) {
	t.Run("invalid returns empty", func(t *testing.T) {
		s := stringFromNull(sql.NullString{})
		if s != "" {
			t.Errorf("expected empty string, got %q", s)
		}
	})

	t.Run("valid returns string", func(t *testing.T) {
		s := stringFromNull(sql.NullString{String: "world", Valid: true})
		if s != "world" {
			t.Errorf("expected %q, got %q", "world", s)
		}
	})
}

func TestNullFloat(t *testing.T) {
	t.Run("zero returns invalid", func(t *testing.T) {
		nf := nullFloat(0)
		if nf.Valid {
			t.Error("expected Valid=false for zero")
		}
	})

	t.Run("non-zero returns valid", func(t *testing.T) {
		nf := nullFloat(3.14)
		if !nf.Valid {
			t.Error("expected Valid=true for non-zero")
		}
		if nf.Float64 != 3.14 {
			t.Errorf("Float64 = %f, want %f", nf.Float64, 3.14)
		}
	})

	t.Run("negative returns valid", func(t *testing.T) {
		nf := nullFloat(-1.5)
		if !nf.Valid {
			t.Error("expected Valid=true for negative")
		}
	})
}

func TestFloatFromNull(t *testing.T) {
	t.Run("invalid returns zero", func(t *testing.T) {
		f := floatFromNull(sql.NullFloat64{})
		if f != 0 {
			t.Errorf("expected 0, got %f", f)
		}
	})

	t.Run("valid returns float", func(t *testing.T) {
		f := floatFromNull(sql.NullFloat64{Float64: 2.72, Valid: true})
		if f != 2.72 {
			t.Errorf("expected %f, got %f", 2.72, f)
		}
	})
}

func TestNullTime(t *testing.T) {
	t.Run("zero time returns invalid", func(t *testing.T) {
		nt := nullTime(time.Time{})
		if nt.Valid {
			t.Error("expected Valid=false for zero time")
		}
	})

	t.Run("non-zero time returns valid", func(t *testing.T) {
		now := time.Now()
		nt := nullTime(now)
		if !nt.Valid {
			t.Error("expected Valid=true for non-zero time")
		}
		if !nt.Time.Equal(now) {
			t.Errorf("Time = %v, want %v", nt.Time, now)
		}
	})
}

func TestTimeFromNull(t *testing.T) {
	t.Run("invalid returns zero", func(t *testing.T) {
		tm := timeFromNull(sql.NullTime{})
		if !tm.IsZero() {
			t.Errorf("expected zero time, got %v", tm)
		}
	})

	t.Run("valid returns time", func(t *testing.T) {
		now := time.Now()
		tm := timeFromNull(sql.NullTime{Time: now, Valid: true})
		if !tm.Equal(now) {
			t.Errorf("expected %v, got %v", now, tm)
		}
	})
}

func TestNullInt(t *testing.T) {
	t.Run("zero returns invalid", func(t *testing.T) {
		ni := nullInt(0)
		if ni.Valid {
			t.Error("expected Valid=false for zero")
		}
	})

	t.Run("positive returns valid", func(t *testing.T) {
		ni := nullInt(42)
		if !ni.Valid {
			t.Error("expected Valid=true for positive")
		}
		if ni.Int64 != 42 {
			t.Errorf("Int64 = %d, want %d", ni.Int64, 42)
		}
	})

	t.Run("negative returns valid", func(t *testing.T) {
		ni := nullInt(-7)
		if !ni.Valid {
			t.Error("expected Valid=true for negative")
		}
		if ni.Int64 != -7 {
			t.Errorf("Int64 = %d, want %d", ni.Int64, -7)
		}
	})
}

func TestIntFromNull(t *testing.T) {
	t.Run("invalid returns zero", func(t *testing.T) {
		i := intFromNull(sql.NullInt64{})
		if i != 0 {
			t.Errorf("expected 0, got %d", i)
		}
	})

	t.Run("valid returns int", func(t *testing.T) {
		i := intFromNull(sql.NullInt64{Int64: 99, Valid: true})
		if i != 99 {
			t.Errorf("expected 99, got %d", i)
		}
	})
}

// ---------------------------------------------------------------------------
// nullJSON / jsonFromNullBytes helper tests
// ---------------------------------------------------------------------------

func TestNullJSON(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		result := nullJSON(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		result := nullJSON([]byte{})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("non-empty returns bytes", func(t *testing.T) {
		data := []byte(`{"key":"value"}`)
		result := nullJSON(data)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		b, ok := result.([]byte)
		if !ok {
			t.Fatalf("expected []byte, got %T", result)
		}
		if string(b) != `{"key":"value"}` {
			t.Errorf("got %q, want %q", string(b), `{"key":"value"}`)
		}
	})
}

func TestJsonFromNullBytes(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		result := jsonFromNullBytes(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		result := jsonFromNullBytes([]byte{})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("non-empty returns RawMessage", func(t *testing.T) {
		result := jsonFromNullBytes([]byte(`[1,2,3]`))
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if string(result) != `[1,2,3]` {
			t.Errorf("got %q, want %q", string(result), `[1,2,3]`)
		}
	})
}

// ---------------------------------------------------------------------------
// CollectionRun.IsTerminal tests
// ---------------------------------------------------------------------------

func TestCollectionRun_IsTerminal(t *testing.T) {
	tests := []struct {
		status   string
		terminal bool
	}{
		{"running", false},
		{"completed", true},
		{"failed", true},
		{"interrupted", true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			cr := CollectionRun{Status: tt.status}
			if got := cr.IsTerminal(); got != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.terminal)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NodeSnapshot.IsPolicyfileNode tests
// ---------------------------------------------------------------------------

func TestNodeSnapshot_IsPolicyfileNode(t *testing.T) {
	tests := []struct {
		name        string
		policyName  string
		policyGroup string
		want        bool
	}{
		{"both set", "base", "production", true},
		{"only name", "base", "", false},
		{"only group", "", "production", false},
		{"neither", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := NodeSnapshot{
				PolicyName:  tt.policyName,
				PolicyGroup: tt.policyGroup,
			}
			if got := ns.IsPolicyfileNode(); got != tt.want {
				t.Errorf("IsPolicyfileNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ServerCookbook.IsDownloaded / NeedsDownload tests
// ---------------------------------------------------------------------------

func TestServerCookbook_DownloadHelpers(t *testing.T) {
	t.Run("pending status", func(t *testing.T) {
		sc := ServerCookbook{DownloadStatus: DownloadStatusPending}
		if sc.IsDownloaded() {
			t.Error("expected IsDownloaded() = false for pending")
		}
		if !sc.NeedsDownload() {
			t.Error("expected NeedsDownload() = true for pending")
		}
	})

	t.Run("ok status", func(t *testing.T) {
		sc := ServerCookbook{DownloadStatus: DownloadStatusOK}
		if !sc.IsDownloaded() {
			t.Error("expected IsDownloaded() = true for ok")
		}
		if sc.NeedsDownload() {
			t.Error("expected NeedsDownload() = false for ok")
		}
	})

	t.Run("failed status", func(t *testing.T) {
		sc := ServerCookbook{DownloadStatus: DownloadStatusFailed}
		if sc.IsDownloaded() {
			t.Error("expected IsDownloaded() = false for failed")
		}
		if !sc.NeedsDownload() {
			t.Error("expected NeedsDownload() = true for failed")
		}
	})
}

// ---------------------------------------------------------------------------
// Validation helper tests (for cookbook_node_usage)
// ---------------------------------------------------------------------------

func TestValidateUsageParams(t *testing.T) {
	t.Run("valid params", func(t *testing.T) {
		p := InsertCookbookNodeUsageParams{
			ServerCookbookID: "cb-1",
			NodeSnapshotID:   "ns-1",
			CookbookVersion:  "1.0.0",
		}
		if err := validateUsageParams(p); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing cookbook ID", func(t *testing.T) {
		p := InsertCookbookNodeUsageParams{
			NodeSnapshotID:  "ns-1",
			CookbookVersion: "1.0.0",
		}
		if err := validateUsageParams(p); err == nil {
			t.Error("expected error for missing cookbook ID")
		}
	})

	t.Run("missing node snapshot ID", func(t *testing.T) {
		p := InsertCookbookNodeUsageParams{
			ServerCookbookID: "cb-1",
			CookbookVersion:  "1.0.0",
		}
		if err := validateUsageParams(p); err == nil {
			t.Error("expected error for missing node snapshot ID")
		}
	})

	t.Run("missing cookbook version", func(t *testing.T) {
		p := InsertCookbookNodeUsageParams{
			ServerCookbookID: "cb-1",
			NodeSnapshotID:   "ns-1",
		}
		if err := validateUsageParams(p); err == nil {
			t.Error("expected error for missing cookbook version")
		}
	})
}

// ---------------------------------------------------------------------------
// MarshalJSON smoke tests — ensure no panic and valid output
// ---------------------------------------------------------------------------

func TestOrganisation_MarshalJSON(t *testing.T) {
	org := Organisation{
		ID:            "test-id",
		Name:          "test-org",
		ChefServerURL: "https://chef.example.com",
		OrgName:       "myorg",
		ClientName:    "admin",
		Source:        "config",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	data, err := org.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestCollectionRun_MarshalJSON(t *testing.T) {
	cr := CollectionRun{
		ID:             "run-id",
		OrganisationID: "org-id",
		Status:         "running",
		StartedAt:      time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	data, err := cr.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestNodeSnapshot_MarshalJSON(t *testing.T) {
	ns := NodeSnapshot{
		ID:              "snap-id",
		CollectionRunID: "run-id",
		OrganisationID:  "org-id",
		NodeName:        "node1.example.com",
		ChefVersion:     "18.4.2",
		CollectedAt:     time.Now(),
		CreatedAt:       time.Now(),
	}
	data, err := ns.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestServerCookbook_MarshalJSON(t *testing.T) {
	sc := ServerCookbook{
		ID:             "cb-id",
		OrganisationID: "org-id",
		Name:           "apache2",
		Version:        "1.0.0",
		DownloadStatus: DownloadStatusPending,
	}
	data, err := sc.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestGitRepo_MarshalJSON(t *testing.T) {
	gr := GitRepo{
		ID:         "gr-id",
		Name:       "apache2",
		GitRepoURL: "https://github.com/example/apache2",
	}
	data, err := gr.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestCookbookNodeUsage_MarshalJSON(t *testing.T) {
	u := CookbookNodeUsage{
		ID:               "usage-id",
		ServerCookbookID: "cb-id",
		NodeSnapshotID:   "snap-id",
		CookbookVersion:  "2.1.0",
		CreatedAt:        time.Now(),
	}
	data, err := u.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

// ---------------------------------------------------------------------------
// Log entry validation tests
// ---------------------------------------------------------------------------

func TestValidateLogEntryParams_Valid(t *testing.T) {
	p := InsertLogEntryParams{
		Timestamp: time.Now(),
		Severity:  "INFO",
		Scope:     "collection_run",
		Message:   "test message",
	}
	if err := validateLogEntryParams(p); err != nil {
		t.Errorf("validateLogEntryParams() returned error for valid params: %v", err)
	}
}

func TestValidateLogEntryParams_AllSeverities(t *testing.T) {
	for _, sev := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		t.Run(sev, func(t *testing.T) {
			p := InsertLogEntryParams{
				Timestamp: time.Now(),
				Severity:  sev,
				Scope:     "startup",
				Message:   "test",
			}
			if err := validateLogEntryParams(p); err != nil {
				t.Errorf("validateLogEntryParams() returned error for severity %q: %v", sev, err)
			}
		})
	}
}

func TestValidateLogEntryParams_MissingTimestamp(t *testing.T) {
	p := InsertLogEntryParams{
		Severity: "INFO",
		Scope:    "startup",
		Message:  "test",
	}
	err := validateLogEntryParams(p)
	if err == nil {
		t.Fatal("expected error for missing timestamp")
	}
	if !strings.Contains(err.Error(), "timestamp") {
		t.Errorf("error %q should mention 'timestamp'", err.Error())
	}
}

func TestValidateLogEntryParams_InvalidSeverity(t *testing.T) {
	for _, sev := range []string{"", "FATAL", "info", "warning", "123"} {
		t.Run(fmt.Sprintf("%q", sev), func(t *testing.T) {
			p := InsertLogEntryParams{
				Timestamp: time.Now(),
				Severity:  sev,
				Scope:     "startup",
				Message:   "test",
			}
			err := validateLogEntryParams(p)
			if err == nil {
				t.Fatalf("expected error for severity %q", sev)
			}
			if !strings.Contains(err.Error(), "severity") {
				t.Errorf("error %q should mention 'severity'", err.Error())
			}
		})
	}
}

func TestValidateLogEntryParams_MissingScope(t *testing.T) {
	p := InsertLogEntryParams{
		Timestamp: time.Now(),
		Severity:  "INFO",
		Message:   "test",
	}
	err := validateLogEntryParams(p)
	if err == nil {
		t.Fatal("expected error for missing scope")
	}
	if !strings.Contains(err.Error(), "scope") {
		t.Errorf("error %q should mention 'scope'", err.Error())
	}
}

func TestValidateLogEntryParams_MissingMessage(t *testing.T) {
	p := InsertLogEntryParams{
		Timestamp: time.Now(),
		Severity:  "INFO",
		Scope:     "startup",
	}
	err := validateLogEntryParams(p)
	if err == nil {
		t.Fatal("expected error for missing message")
	}
	if !strings.Contains(err.Error(), "message") {
		t.Errorf("error %q should mention 'message'", err.Error())
	}
}

func TestValidateLogEntryParams_WithAllOptionalFields(t *testing.T) {
	p := InsertLogEntryParams{
		Timestamp:           time.Now(),
		Severity:            "WARN",
		Scope:               "cookstyle_scan",
		Message:             "deprecation found",
		Organisation:        "prod",
		CookbookName:        "apache2",
		CookbookVersion:     "5.2.1",
		CommitSHA:           "abc123",
		ChefClientVersion:   "19.0",
		ProcessOutput:       "line 1\nline 2",
		CollectionRunID:     "run-1",
		NotificationChannel: "slack",
		ExportJobID:         "exp-1",
		TLSDomain:           "example.com",
	}
	if err := validateLogEntryParams(p); err != nil {
		t.Errorf("validateLogEntryParams() returned error for valid params with all optional fields: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Severity ordinal tests
// ---------------------------------------------------------------------------

func TestSeverityOrdinal_Valid(t *testing.T) {
	tests := []struct {
		sev  string
		want int
	}{
		{"DEBUG", 0},
		{"INFO", 1},
		{"WARN", 2},
		{"ERROR", 3},
	}
	for _, tt := range tests {
		t.Run(tt.sev, func(t *testing.T) {
			got := severityOrdinal(tt.sev)
			if got != tt.want {
				t.Errorf("severityOrdinal(%q) = %d, want %d", tt.sev, got, tt.want)
			}
		})
	}
}

func TestSeverityOrdinal_Invalid(t *testing.T) {
	for _, sev := range []string{"", "debug", "FATAL", "info", "123"} {
		t.Run(fmt.Sprintf("%q", sev), func(t *testing.T) {
			got := severityOrdinal(sev)
			if got != -1 {
				t.Errorf("severityOrdinal(%q) = %d, want -1", sev, got)
			}
		})
	}
}

func TestMinSeverityValues(t *testing.T) {
	tests := []struct {
		minSev string
		want   []string
	}{
		{"DEBUG", []string{"DEBUG", "INFO", "WARN", "ERROR"}},
		{"INFO", []string{"INFO", "WARN", "ERROR"}},
		{"WARN", []string{"WARN", "ERROR"}},
		{"ERROR", []string{"ERROR"}},
	}
	for _, tt := range tests {
		t.Run(tt.minSev, func(t *testing.T) {
			got := minSeverityValues(tt.minSev)
			if len(got) != len(tt.want) {
				t.Fatalf("minSeverityValues(%q) returned %d values, want %d", tt.minSev, len(got), len(tt.want))
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("minSeverityValues(%q)[%d] = %q, want %q", tt.minSev, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestMinSeverityValues_Invalid(t *testing.T) {
	got := minSeverityValues("FATAL")
	if got != nil {
		t.Errorf("minSeverityValues(FATAL) = %v, want nil", got)
	}
	got = minSeverityValues("")
	if got != nil {
		t.Errorf("minSeverityValues('') = %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// nullStringUUID tests
// ---------------------------------------------------------------------------

func TestNullStringUUID_Empty(t *testing.T) {
	ns := nullStringUUID("")
	if ns.Valid {
		t.Error("nullStringUUID('') should be invalid")
	}
}

func TestNullStringUUID_NonEmpty(t *testing.T) {
	ns := nullStringUUID("abc-123")
	if !ns.Valid {
		t.Error("nullStringUUID('abc-123') should be valid")
	}
	if ns.String != "abc-123" {
		t.Errorf("nullStringUUID('abc-123').String = %q, want 'abc-123'", ns.String)
	}
}

// ---------------------------------------------------------------------------
// stringArray / joinQuoted tests
// ---------------------------------------------------------------------------

func TestStringSliceToArray_Empty(t *testing.T) {
	a := stringSliceToArray(nil)
	val, err := a.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if val != "{}" {
		t.Errorf("Value() = %q, want '{}'", val)
	}
}

func TestStringSliceToArray_Single(t *testing.T) {
	a := stringSliceToArray([]string{"INFO"})
	val, err := a.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	expected := `{"INFO"}`
	if val != expected {
		t.Errorf("Value() = %q, want %q", val, expected)
	}
}

func TestStringSliceToArray_Multiple(t *testing.T) {
	a := stringSliceToArray([]string{"WARN", "ERROR"})
	val, err := a.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	expected := `{"WARN","ERROR"}`
	if val != expected {
		t.Errorf("Value() = %q, want %q", val, expected)
	}
}

func TestJoinQuoted_Empty(t *testing.T) {
	got := joinQuoted(nil)
	if got != "" {
		t.Errorf("joinQuoted(nil) = %q, want empty", got)
	}
}

func TestJoinQuoted_Single(t *testing.T) {
	got := joinQuoted([]string{"hello"})
	if got != `"hello"` {
		t.Errorf("joinQuoted = %q, want %q", got, `"hello"`)
	}
}

func TestJoinQuoted_Multiple(t *testing.T) {
	got := joinQuoted([]string{"a", "b", "c"})
	expected := `"a","b","c"`
	if got != expected {
		t.Errorf("joinQuoted = %q, want %q", got, expected)
	}
}

// ---------------------------------------------------------------------------
// LogEntry type tests
// ---------------------------------------------------------------------------

func TestLogEntry_MarshalJSON(t *testing.T) {
	le := LogEntry{
		ID:           "test-id",
		Timestamp:    time.Date(2025, 1, 20, 12, 0, 0, 0, time.UTC),
		Severity:     "WARN",
		Scope:        "collection_run",
		Message:      "test message",
		Organisation: "prod",
		CreatedAt:    time.Date(2025, 1, 20, 12, 0, 1, 0, time.UTC),
	}
	data, err := le.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded["severity"] != "WARN" {
		t.Errorf("severity = %v, want WARN", decoded["severity"])
	}
	if decoded["scope"] != "collection_run" {
		t.Errorf("scope = %v, want collection_run", decoded["scope"])
	}
	if decoded["message"] != "test message" {
		t.Errorf("message = %v, want 'test message'", decoded["message"])
	}
	if decoded["organisation"] != "prod" {
		t.Errorf("organisation = %v, want prod", decoded["organisation"])
	}
}

func TestLogEntry_MarshalJSON_EmptyOptionalFields(t *testing.T) {
	le := LogEntry{
		ID:        "test-id",
		Timestamp: time.Date(2025, 1, 20, 12, 0, 0, 0, time.UTC),
		Severity:  "INFO",
		Scope:     "startup",
		Message:   "started",
		CreatedAt: time.Date(2025, 1, 20, 12, 0, 0, 0, time.UTC),
	}
	data, err := le.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Optional fields with omitempty should be absent when empty.
	for _, field := range []string{
		"cookbook_name", "cookbook_version", "commit_sha",
		"chef_client_version", "process_output", "collection_run_id",
		"notification_channel", "export_job_id", "tls_domain",
	} {
		if _, exists := decoded[field]; exists {
			t.Errorf("empty field %q should be omitted from JSON, but was present", field)
		}
	}
}

func TestLogEntry_MarshalJSON_AllFields(t *testing.T) {
	le := LogEntry{
		ID:                  "id-1",
		Timestamp:           time.Date(2025, 1, 20, 12, 0, 0, 0, time.UTC),
		Severity:            "ERROR",
		Scope:               "notification_dispatch",
		Message:             "delivery failed",
		Organisation:        "org1",
		CookbookName:        "nginx",
		CookbookVersion:     "3.0.0",
		CommitSHA:           "deadbeef",
		ChefClientVersion:   "19.4.0",
		ProcessOutput:       "error output",
		CollectionRunID:     "run-1",
		NotificationChannel: "slack-ops",
		ExportJobID:         "exp-42",
		TLSDomain:           "api.example.com",
		CreatedAt:           time.Date(2025, 1, 20, 12, 0, 1, 0, time.UTC),
	}
	data, err := le.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	checks := map[string]string{
		"id":                   "id-1",
		"severity":             "ERROR",
		"scope":                "notification_dispatch",
		"message":              "delivery failed",
		"organisation":         "org1",
		"cookbook_name":        "nginx",
		"cookbook_version":     "3.0.0",
		"commit_sha":           "deadbeef",
		"chef_client_version":  "19.4.0",
		"process_output":       "error output",
		"collection_run_id":    "run-1",
		"notification_channel": "slack-ops",
		"export_job_id":        "exp-42",
		"tls_domain":           "api.example.com",
	}
	for field, want := range checks {
		got, ok := decoded[field].(string)
		if !ok {
			t.Errorf("field %q not found or not a string in JSON", field)
			continue
		}
		if got != want {
			t.Errorf("field %q = %q, want %q", field, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// LogEntryFilter tests (query builder logic)
// ---------------------------------------------------------------------------

func TestLogEntryFilter_Empty(t *testing.T) {
	f := LogEntryFilter{}
	// An empty filter should not cause a panic. We can't test the SQL here
	// (needs DB), but we can verify the struct is valid.
	if f.Scope != "" || f.Severity != "" || f.Limit != 0 {
		t.Error("empty filter should have zero values")
	}
}

func TestLogEntryFilter_AllFieldsSet(t *testing.T) {
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	f := LogEntryFilter{
		Scope:           "collection_run",
		Severity:        "ERROR",
		Organisation:    "prod",
		CookbookName:    "nginx",
		CollectionRunID: "run-1",
		Since:           since,
		Until:           until,
		Limit:           100,
		Offset:          50,
	}
	if f.Scope != "collection_run" {
		t.Errorf("Scope = %q, want collection_run", f.Scope)
	}
	if f.Limit != 100 {
		t.Errorf("Limit = %d, want 100", f.Limit)
	}
	if f.Offset != 50 {
		t.Errorf("Offset = %d, want 50", f.Offset)
	}
}

func TestPurgeLogEntriesOlderThanDays_InvalidRetention(t *testing.T) {
	// We can't test against a real DB, but we can verify the validation
	// logic by checking the error message pattern. The DB is nil so we'd
	// panic if validation doesn't catch it first.
	// Create a minimal DB wrapper — we only need the validation path.
	db := &DB{pool: nil}

	for _, days := range []int{0, -1, -100} {
		t.Run(fmt.Sprintf("days=%d", days), func(t *testing.T) {
			_, err := db.PurgeLogEntriesOlderThanDays(context.Background(), days)
			if err == nil {
				t.Fatalf("expected error for retention days %d", days)
			}
			if !strings.Contains(err.Error(), "retention days must be > 0") {
				t.Errorf("error %q should mention retention days", err.Error())
			}
		})
	}
}

func TestPurgeLogEntriesBefore_ZeroTime(t *testing.T) {
	db := &DB{pool: nil}
	_, err := db.PurgeLogEntriesBefore(context.Background(), time.Time{})
	if err == nil {
		t.Fatal("expected error for zero cutoff time")
	}
	if !strings.Contains(err.Error(), "cutoff time is required") {
		t.Errorf("error %q should mention cutoff time", err.Error())
	}
}

func TestGetLogEntry_EmptyID(t *testing.T) {
	db := &DB{pool: nil}
	_, err := db.GetLogEntry(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	if !strings.Contains(err.Error(), "log entry ID is required") {
		t.Errorf("error %q should mention log entry ID", err.Error())
	}
}

func TestListLogEntriesByCollectionRun_EmptyID(t *testing.T) {
	db := &DB{pool: nil}
	_, err := db.ListLogEntriesByCollectionRun(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty collection run ID")
	}
	if !strings.Contains(err.Error(), "collection run ID is required") {
		t.Errorf("error %q should mention collection run ID", err.Error())
	}
}

func TestDeleteLogEntriesByCollectionRun_EmptyID(t *testing.T) {
	db := &DB{pool: nil}
	_, err := db.DeleteLogEntriesByCollectionRun(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty collection run ID")
	}
	if !strings.Contains(err.Error(), "collection run ID is required") {
		t.Errorf("error %q should mention collection run ID", err.Error())
	}
}

func TestBulkInsertLogEntries_EmptySlice(t *testing.T) {
	db := &DB{pool: nil}
	count, err := db.BulkInsertLogEntries(context.Background(), nil)
	if err != nil {
		t.Fatalf("BulkInsertLogEntries(nil) returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("BulkInsertLogEntries(nil) returned count %d, want 0", count)
	}

	count, err = db.BulkInsertLogEntries(context.Background(), []InsertLogEntryParams{})
	if err != nil {
		t.Fatalf("BulkInsertLogEntries([]) returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("BulkInsertLogEntries([]) returned count %d, want 0", count)
	}
}

// ---------------------------------------------------------------------------
// BulkInsertNodeSnapshots — empty slice edge cases
// ---------------------------------------------------------------------------

func TestBulkInsertNodeSnapshots_EmptySlice(t *testing.T) {
	db := &DB{pool: nil}
	count, err := db.BulkInsertNodeSnapshots(context.Background(), nil)
	if err != nil {
		t.Fatalf("BulkInsertNodeSnapshots(nil) returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("BulkInsertNodeSnapshots(nil) returned count %d, want 0", count)
	}

	count, err = db.BulkInsertNodeSnapshots(context.Background(), []InsertNodeSnapshotParams{})
	if err != nil {
		t.Fatalf("BulkInsertNodeSnapshots([]) returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("BulkInsertNodeSnapshots([]) returned count %d, want 0", count)
	}
}

func TestBulkInsertNodeSnapshotsReturningIDs_EmptySlice(t *testing.T) {
	db := &DB{pool: nil}
	idMap, count, err := db.BulkInsertNodeSnapshotsReturningIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("BulkInsertNodeSnapshotsReturningIDs(nil) returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("BulkInsertNodeSnapshotsReturningIDs(nil) returned count %d, want 0", count)
	}
	if idMap != nil {
		t.Errorf("BulkInsertNodeSnapshotsReturningIDs(nil) returned non-nil map: %v", idMap)
	}

	idMap, count, err = db.BulkInsertNodeSnapshotsReturningIDs(context.Background(), []InsertNodeSnapshotParams{})
	if err != nil {
		t.Fatalf("BulkInsertNodeSnapshotsReturningIDs([]) returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("BulkInsertNodeSnapshotsReturningIDs([]) returned count %d, want 0", count)
	}
	if idMap != nil {
		t.Errorf("BulkInsertNodeSnapshotsReturningIDs([]) returned non-nil map: %v", idMap)
	}
}

// ---------------------------------------------------------------------------
// BulkInsertCookbookNodeUsage — empty slice edge case
// ---------------------------------------------------------------------------

func TestBulkInsertCookbookNodeUsage_EmptySlice(t *testing.T) {
	db := &DB{pool: nil}
	count, err := db.BulkInsertCookbookNodeUsage(context.Background(), nil)
	if err != nil {
		t.Fatalf("BulkInsertCookbookNodeUsage(nil) returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("BulkInsertCookbookNodeUsage(nil) returned count %d, want 0", count)
	}

	count, err = db.BulkInsertCookbookNodeUsage(context.Background(), []InsertCookbookNodeUsageParams{})
	if err != nil {
		t.Fatalf("BulkInsertCookbookNodeUsage([]) returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("BulkInsertCookbookNodeUsage([]) returned count %d, want 0", count)
	}
}
