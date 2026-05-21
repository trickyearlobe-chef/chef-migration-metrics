// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndReadManifest(t *testing.T) {
	dir := t.TempDir()

	m := Manifest{
		ID:              "test-backup-001",
		Filename:        "test-backup-001.dump",
		SizeBytes:       12345,
		SHA256:          "abc123def456",
		CreatedAt:       time.Date(2025, 5, 20, 12, 0, 0, 0, time.UTC),
		AppVersion:      "1.0.0",
		SchemaVersion:   30,
		PgServerVersion: "17.2",
		PgDumpVersion:   "17.2",
		Status:          StatusSucceeded,
		InitiatedBy:     "user@example.com",
	}

	if err := WriteManifest(dir, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	// Verify file exists with correct permissions
	info, err := os.Stat(filepath.Join(dir, "test-backup-001.json"))
	if err != nil {
		t.Fatalf("manifest file not found: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("manifest permissions = %o, want 0600", perm)
	}

	// Read it back
	got, err := ReadManifest(dir, "test-backup-001")
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	if got.ID != m.ID {
		t.Errorf("ID = %q, want %q", got.ID, m.ID)
	}
	if got.SizeBytes != m.SizeBytes {
		t.Errorf("SizeBytes = %d, want %d", got.SizeBytes, m.SizeBytes)
	}
	if got.SHA256 != m.SHA256 {
		t.Errorf("SHA256 = %q, want %q", got.SHA256, m.SHA256)
	}
	if got.Status != StatusSucceeded {
		t.Errorf("Status = %q, want %q", got.Status, StatusSucceeded)
	}
	if got.InitiatedBy != m.InitiatedBy {
		t.Errorf("InitiatedBy = %q, want %q", got.InitiatedBy, m.InitiatedBy)
	}
}

func TestReadManifest_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadManifest(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestListManifests(t *testing.T) {
	dir := t.TempDir()

	// Write 3 manifests with different timestamps
	for i, ts := range []time.Time{
		time.Date(2025, 5, 18, 12, 0, 0, 0, time.UTC),
		time.Date(2025, 5, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2025, 5, 19, 12, 0, 0, 0, time.UTC),
	} {
		m := Manifest{
			ID:        fmt.Sprintf("backup-%d", i),
			Filename:  fmt.Sprintf("backup-%d.dump", i),
			CreatedAt: ts,
			Status:    StatusSucceeded,
		}
		if err := WriteManifest(dir, m); err != nil {
			t.Fatalf("WriteManifest[%d]: %v", i, err)
		}
	}

	manifests, err := ListManifests(dir)
	if err != nil {
		t.Fatalf("ListManifests: %v", err)
	}

	if len(manifests) != 3 {
		t.Fatalf("got %d manifests, want 3", len(manifests))
	}

	// Should be sorted newest first
	if manifests[0].ID != "backup-1" {
		t.Errorf("first manifest = %q, want backup-1 (newest)", manifests[0].ID)
	}
	if manifests[2].ID != "backup-0" {
		t.Errorf("last manifest = %q, want backup-0 (oldest)", manifests[2].ID)
	}
}

func TestListManifests_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	manifests, err := ListManifests(dir)
	if err != nil {
		t.Fatalf("ListManifests: %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("got %d manifests, want 0", len(manifests))
	}
}

func TestListManifests_NonexistentDir(t *testing.T) {
	manifests, err := ListManifests("/nonexistent/path")
	if err != nil {
		t.Fatalf("ListManifests: %v", err)
	}
	if manifests != nil {
		t.Errorf("got %v, want nil", manifests)
	}
}

func TestDeleteManifest(t *testing.T) {
	dir := t.TempDir()

	m := Manifest{ID: "to-delete", Filename: "to-delete.dump", Status: StatusSucceeded}
	if err := WriteManifest(dir, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if err := DeleteManifest(dir, "to-delete"); err != nil {
		t.Fatalf("DeleteManifest: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "to-delete.json")); !os.IsNotExist(err) {
		t.Error("manifest file still exists after delete")
	}
}

func TestDeleteManifest_NotFound(t *testing.T) {
	dir := t.TempDir()
	// Should not error on missing file
	if err := DeleteManifest(dir, "nonexistent"); err != nil {
		t.Errorf("DeleteManifest on missing file: %v", err)
	}
}
