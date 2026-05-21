// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package backup provides database backup and restore operations using
// PostgreSQL's pg_dump and pg_restore utilities.
package backup

import (
	"context"
	"time"
)

// Status represents the state of a backup operation.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusDeleting  Status = "deleting"
	StatusRestoring Status = "restoring"
)

// Manifest holds metadata about a backup file. Stored as a sidecar JSON file
// next to the dump and used as the source of truth for available backups.
type Manifest struct {
	ID              string    `json:"id"`
	Filename        string    `json:"filename"`
	SizeBytes       int64     `json:"size_bytes"`
	SHA256          string    `json:"sha256"`
	CreatedAt       time.Time `json:"created_at"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	AppVersion      string    `json:"app_version"`
	SchemaVersion   int       `json:"schema_version"`
	PgServerVersion string    `json:"pg_server_version"`
	PgDumpVersion   string    `json:"pg_dump_version"`
	Status          Status    `json:"status"`
	Error           string    `json:"error,omitempty"`
	InitiatedBy     string    `json:"initiated_by,omitempty"`
}

// ConnParams holds parsed PostgreSQL connection parameters for use with
// pg_dump/pg_restore via environment variables.
type ConnParams struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// CommandExecutor abstracts the execution of pg_dump/pg_restore for testing.
type CommandExecutor interface {
	// PgDump runs pg_dump and writes the output to the given path.
	PgDump(ctx context.Context, conn ConnParams, outputPath string) error
	// PgRestore runs pg_restore from the given dump file.
	PgRestore(ctx context.Context, conn ConnParams, inputPath string) error
	// PgDumpVersion returns the version string of pg_dump.
	PgDumpVersion(ctx context.Context) (string, error)
	// PgServerVersion returns the PostgreSQL server version.
	PgServerVersion(ctx context.Context, conn ConnParams) (string, error)
}
