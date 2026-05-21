// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mockExecutor is a test double for CommandExecutor.
type mockExecutor struct {
	pgDumpFn        func(ctx context.Context, conn ConnParams, outputPath string) error
	pgRestoreFn     func(ctx context.Context, conn ConnParams, inputPath string) error
	pgDumpVersion   string
	pgServerVersion string
}

func (m *mockExecutor) PgDump(ctx context.Context, conn ConnParams, outputPath string) error {
	if m.pgDumpFn != nil {
		return m.pgDumpFn(ctx, conn, outputPath)
	}
	// Default: write some dummy data to simulate pg_dump output
	return os.WriteFile(outputPath, []byte("fake-pg-dump-data"), 0600)
}

func (m *mockExecutor) PgRestore(ctx context.Context, conn ConnParams, inputPath string) error {
	if m.pgRestoreFn != nil {
		return m.pgRestoreFn(ctx, conn, inputPath)
	}
	return nil
}

func (m *mockExecutor) PgDumpVersion(_ context.Context) (string, error) {
	return m.pgDumpVersion, nil
}

func (m *mockExecutor) PgServerVersion(_ context.Context, _ ConnParams) (string, error) {
	return m.pgServerVersion, nil
}

func newTestService(t *testing.T, exec *mockExecutor) *Service {
	t.Helper()
	dir := t.TempDir()
	conn := ConnParams{Host: "localhost", Port: "5432", User: "test", DBName: "testdb"}
	svc, err := NewService(dir, conn, exec, "1.0.0-test", 30, 3)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestService_CreateAndRun(t *testing.T) {
	exec := &mockExecutor{
		pgDumpVersion:   "pg_dump (PostgreSQL) 17.2",
		pgServerVersion: "17.2",
	}
	svc := newTestService(t, exec)

	ctx := context.Background()
	m, err := svc.Create(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if m.Status != StatusRunning {
		t.Errorf("Status = %q, want %q", m.Status, StatusRunning)
	}
	if m.ID == "" {
		t.Error("ID should not be empty")
	}

	// Run the backup
	svc.RunCreate(ctx, &m)

	// Verify manifest was updated
	got, err := svc.Get(m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusSucceeded {
		t.Errorf("Status = %q, want %q", got.Status, StatusSucceeded)
	}
	if got.SHA256 == "" {
		t.Error("SHA256 should be set after successful backup")
	}
	if got.SizeBytes == 0 {
		t.Error("SizeBytes should be > 0")
	}
	if got.PgDumpVersion != "pg_dump (PostgreSQL) 17.2" {
		t.Errorf("PgDumpVersion = %q", got.PgDumpVersion)
	}

	// Verify dump file exists
	dumpPath := filepath.Join(svc.dir, m.Filename)
	if _, err := os.Stat(dumpPath); err != nil {
		t.Errorf("dump file missing: %v", err)
	}
}

func TestService_CreateFailure(t *testing.T) {
	exec := &mockExecutor{
		pgDumpFn: func(_ context.Context, _ ConnParams, _ string) error {
			return fmt.Errorf("pg_dump: connection refused")
		},
	}
	svc := newTestService(t, exec)

	ctx := context.Background()
	m, err := svc.Create(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	svc.RunCreate(ctx, &m)

	got, err := svc.Get(m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, StatusFailed)
	}
	if got.Error == "" {
		t.Error("Error should be set on failure")
	}

	// Temp file should be cleaned up
	tmpPath := filepath.Join(svc.dir, m.Filename+".tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up on failure")
	}
}

func TestService_ConcurrentCreate(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)
	ctx := context.Background()

	// Start first backup
	_, err := svc.Create(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// Second should fail
	_, err = svc.Create(ctx, "user@example.com")
	if err == nil {
		t.Error("expected error for concurrent create")
	}
}

func TestService_List(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)
	ctx := context.Background()

	// Create 2 backups
	for i := 0; i < 2; i++ {
		m, err := svc.Create(ctx, "user@example.com")
		if err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
		svc.RunCreate(ctx, &m)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("got %d backups, want 2", len(list))
	}
}

func TestService_Delete(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)
	ctx := context.Background()

	m, err := svc.Create(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.RunCreate(ctx, &m)

	if err := svc.Delete(m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Should be gone
	_, err = svc.Get(m.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestService_Prune(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec) // maxGenerations=3
	ctx := context.Background()

	// Create 5 backups
	for i := 0; i < 5; i++ {
		m, err := svc.Create(ctx, "user@example.com")
		if err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
		svc.RunCreate(ctx, &m)
	}

	pruned, err := svc.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(pruned) != 2 {
		t.Errorf("pruned %d, want 2", len(pruned))
	}

	remaining, _ := svc.List()
	succeeded := 0
	for _, m := range remaining {
		if m.Status == StatusSucceeded {
			succeeded++
		}
	}
	if succeeded != 3 {
		t.Errorf("remaining succeeded = %d, want 3", succeeded)
	}
}

func TestService_VerifyChecksum(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)
	ctx := context.Background()

	m, err := svc.Create(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.RunCreate(ctx, &m)

	if err := svc.VerifyChecksum(m.ID); err != nil {
		t.Errorf("VerifyChecksum: %v", err)
	}

	// Corrupt the file
	dumpPath := filepath.Join(svc.dir, m.Filename)
	if err := os.WriteFile(dumpPath, []byte("corrupted"), 0600); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}

	if err := svc.VerifyChecksum(m.ID); err == nil {
		t.Error("expected checksum mismatch error")
	}
}

func TestService_IsActive(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)
	ctx := context.Background()

	if svc.IsActive() {
		t.Error("should not be active initially")
	}

	_, err := svc.Create(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !svc.IsActive() {
		t.Error("should be active after Create")
	}
}

func writeTestFile(path string) error {
	return os.WriteFile(path, []byte("test-dump-data"), 0600)
}

func TestService_RunRestore(t *testing.T) {
	var restoreCalled bool
	exec := &mockExecutor{
		pgRestoreFn: func(_ context.Context, _ ConnParams, inputPath string) error {
			restoreCalled = true
			if _, err := os.Stat(inputPath); err != nil {
				return fmt.Errorf("input file missing: %w", err)
			}
			return nil
		},
	}
	svc := newTestService(t, exec)

	// Create a backup first
	ctx := context.Background()
	m, err := svc.Create(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.RunCreate(ctx, &m)
	if m.Status != StatusSucceeded {
		t.Fatalf("backup status = %q, want succeeded", m.Status)
	}

	// Restore from the backup
	err = svc.RunRestore(ctx, m.ID)
	if err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if !restoreCalled {
		t.Error("pg_restore was not called")
	}
}

func TestService_RunRestore_BadChecksum(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)

	ctx := context.Background()
	m, err := svc.Create(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.RunCreate(ctx, &m)

	// Corrupt the dump file
	dumpPath := filepath.Join(svc.Dir(), m.Filename)
	if err := os.WriteFile(dumpPath, []byte("corrupted"), 0600); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}

	err = svc.RunRestore(ctx, m.ID)
	if err == nil {
		t.Fatal("expected error for corrupted backup")
	}
	if !contains(err.Error(), "checksum") {
		t.Errorf("error should mention checksum, got: %v", err)
	}
}

func TestService_RunRestore_WhileActive(t *testing.T) {
	exec := &mockExecutor{
		pgDumpFn: func(ctx context.Context, _ ConnParams, outputPath string) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	svc := newTestService(t, exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m, err := svc.Create(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	go svc.RunCreate(ctx, &m)

	// Try to restore while backup is active
	err = svc.RunRestore(ctx, "some-id")
	if err == nil {
		t.Fatal("expected error when another operation is active")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
