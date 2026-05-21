// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LogFunc is a callback for structured logging from the backup service.
type LogFunc func(level, msg string)

// Service orchestrates backup creation, restoration, listing, deletion, and pruning.
type Service struct {
	dir            string
	conn           ConnParams
	executor       CommandExecutor
	appVersion     string
	schemaVersion  int
	maxGenerations int
	logFunc        LogFunc
	mu             sync.Mutex
	activeJob      *Manifest
}

// NewService creates a backup service. The directory is created with 0700 if it
// does not exist.
func NewService(dir string, conn ConnParams, executor CommandExecutor, appVersion string, schemaVersion int, maxGenerations int) (*Service, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("backup: create dir %s: %w", dir, err)
	}
	if maxGenerations <= 0 {
		maxGenerations = 7
	}
	return &Service{
		dir:            dir,
		conn:           conn,
		executor:       executor,
		appVersion:     appVersion,
		schemaVersion:  schemaVersion,
		maxGenerations: maxGenerations,
	}, nil
}

// SetLogFunc sets the logging callback for the service.
func (s *Service) SetLogFunc(fn LogFunc) {
	s.logFunc = fn
}

// Dir returns the backup storage directory path.
func (s *Service) Dir() string {
	return s.dir
}

func (s *Service) log(level, msg string) {
	if s.logFunc != nil {
		s.logFunc(level, msg)
	}
}

// Create initiates a new backup. It returns the manifest immediately with
// status=pending. The caller should run RunCreate in a goroutine.
func (s *Service) Create(ctx context.Context, initiatedBy string) (Manifest, error) {
	s.mu.Lock()
	if s.activeJob != nil {
		s.mu.Unlock()
		return Manifest{}, fmt.Errorf("backup: another operation in progress (id=%s)", s.activeJob.ID)
	}

	id := uuid.New().String()
	m := Manifest{
		ID:            id,
		Filename:      id + ".dump",
		CreatedAt:     time.Now().UTC(),
		AppVersion:    s.appVersion,
		SchemaVersion: s.schemaVersion,
		Status:        StatusRunning,
		InitiatedBy:   initiatedBy,
	}
	s.activeJob = &m
	s.mu.Unlock()

	if err := WriteManifest(s.dir, m); err != nil {
		s.clearActiveJob()
		return Manifest{}, err
	}

	return m, nil
}

// RunCreate performs the actual pg_dump. Call after Create returns the manifest.
func (s *Service) RunCreate(ctx context.Context, m *Manifest) {
	defer s.clearActiveJob()

	tmpPath := filepath.Join(s.dir, m.Filename+".tmp")
	finalPath := filepath.Join(s.dir, m.Filename)

	s.log("info", fmt.Sprintf("starting backup %s (initiated_by=%s)", m.ID, m.InitiatedBy))

	// Get pg versions
	if ver, err := s.executor.PgDumpVersion(ctx); err == nil {
		m.PgDumpVersion = ver
	}
	if ver, err := s.executor.PgServerVersion(ctx, s.conn); err == nil {
		m.PgServerVersion = ver
	}

	// Run pg_dump
	if err := s.executor.PgDump(ctx, s.conn, tmpPath); err != nil {
		m.Status = StatusFailed
		m.Error = err.Error()
		m.CompletedAt = time.Now().UTC()
		_ = WriteManifest(s.dir, *m)
		_ = os.Remove(tmpPath)
		s.log("error", fmt.Sprintf("backup %s failed: %s", m.ID, m.Error))
		return
	}

	// Rename to final
	if err := os.Rename(tmpPath, finalPath); err != nil {
		m.Status = StatusFailed
		m.Error = fmt.Sprintf("rename: %v", err)
		m.CompletedAt = time.Now().UTC()
		_ = WriteManifest(s.dir, *m)
		_ = os.Remove(tmpPath)
		s.log("error", fmt.Sprintf("backup %s failed: %s", m.ID, m.Error))
		return
	}

	// Set permissions
	_ = os.Chmod(finalPath, 0600)

	// Compute checksum and size
	hash, size, err := checksumFile(finalPath)
	if err != nil {
		m.Status = StatusFailed
		m.Error = fmt.Sprintf("checksum: %v", err)
		m.CompletedAt = time.Now().UTC()
		_ = WriteManifest(s.dir, *m)
		s.log("error", fmt.Sprintf("backup %s failed: %s", m.ID, m.Error))
		return
	}

	m.SHA256 = hash
	m.SizeBytes = size
	m.Status = StatusSucceeded
	m.CompletedAt = time.Now().UTC()
	_ = WriteManifest(s.dir, *m)
	s.log("info", fmt.Sprintf("backup %s completed (size=%d, path=%s)", m.ID, m.SizeBytes, finalPath))
}

// List returns all backup manifests from the filesystem, sorted newest first.
func (s *Service) List() ([]Manifest, error) {
	return ListManifests(s.dir)
}

// Get returns a single backup manifest by ID.
func (s *Service) Get(id string) (Manifest, error) {
	return ReadManifest(s.dir, id)
}

// Delete removes a backup's dump file and manifest.
func (s *Service) Delete(id string) error {
	s.mu.Lock()
	if s.activeJob != nil && s.activeJob.ID == id {
		s.mu.Unlock()
		return fmt.Errorf("backup: cannot delete active backup %s", id)
	}
	s.mu.Unlock()

	dumpPath := filepath.Join(s.dir, id+".dump")
	if err := os.Remove(dumpPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup: delete dump %s: %w", dumpPath, err)
	}
	return DeleteManifest(s.dir, id)
}

// Prune removes the oldest backups beyond maxGenerations, keeping only
// successful ones in the count.
func (s *Service) Prune() ([]string, error) {
	manifests, err := ListManifests(s.dir)
	if err != nil {
		return nil, err
	}

	var successful []Manifest
	for _, m := range manifests {
		if m.Status == StatusSucceeded {
			successful = append(successful, m)
		}
	}

	if len(successful) <= s.maxGenerations {
		return nil, nil
	}

	var pruned []string
	for _, m := range successful[s.maxGenerations:] {
		if err := s.Delete(m.ID); err == nil {
			pruned = append(pruned, m.ID)
		}
	}
	return pruned, nil
}

// VerifyChecksum validates the SHA-256 of a backup file against its manifest.
func (s *Service) VerifyChecksum(id string) error {
	m, err := ReadManifest(s.dir, id)
	if err != nil {
		return err
	}
	if m.SHA256 == "" {
		return fmt.Errorf("backup: no checksum recorded for %s", id)
	}

	dumpPath := filepath.Join(s.dir, m.Filename)
	hash, _, err := checksumFile(dumpPath)
	if err != nil {
		return err
	}
	if hash != m.SHA256 {
		return fmt.Errorf("backup: checksum mismatch for %s (got %s, want %s)", id, hash, m.SHA256)
	}
	return nil
}

// IsActive returns true if a backup or restore operation is in progress.
func (s *Service) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeJob != nil
}

// ActiveJob returns the currently running job manifest, or nil.
func (s *Service) ActiveJob() *Manifest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeJob == nil {
		return nil
	}
	copy := *s.activeJob
	return &copy
}

func (s *Service) clearActiveJob() {
	s.mu.Lock()
	s.activeJob = nil
	s.mu.Unlock()
}

// RunRestore performs the actual pg_restore from a backup. The caller is
// responsible for stopping background workers before calling this method.
func (s *Service) RunRestore(ctx context.Context, id string) error {
	s.mu.Lock()
	if s.activeJob != nil {
		s.mu.Unlock()
		return fmt.Errorf("backup: another operation in progress (id=%s)", s.activeJob.ID)
	}

	m, err := ReadManifest(s.dir, id)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("backup: read manifest: %w", err)
	}

	m.Status = StatusRestoring
	s.activeJob = &m
	s.mu.Unlock()

	defer s.clearActiveJob()

	s.log("info", fmt.Sprintf("starting restore from backup %s", id))

	// Verify checksum before restore
	if err := s.VerifyChecksum(id); err != nil {
		s.log("error", fmt.Sprintf("restore %s aborted: checksum verification failed: %v", id, err))
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	dumpPath := filepath.Join(s.dir, m.Filename)
	if err := s.executor.PgRestore(ctx, s.conn, dumpPath); err != nil {
		s.log("error", fmt.Sprintf("restore %s failed: %v", id, err))
		return fmt.Errorf("pg_restore failed: %w", err)
	}

	s.log("info", fmt.Sprintf("restore %s completed successfully", id))
	return nil
}

func checksumFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("backup: open for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("backup: read for checksum: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
