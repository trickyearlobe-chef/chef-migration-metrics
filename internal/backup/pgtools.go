// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

// ParseConnString extracts connection parameters from a PostgreSQL URL.
func ParseConnString(connURL string) (ConnParams, error) {
	u, err := url.Parse(connURL)
	if err != nil {
		return ConnParams{}, fmt.Errorf("backup: parse connection URL: %w", err)
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "5432"
	}

	user := u.User.Username()
	password, _ := u.User.Password()
	dbName := strings.TrimPrefix(u.Path, "/")

	sslMode := u.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "prefer"
	}

	return ConnParams{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		DBName:   dbName,
		SSLMode:  sslMode,
	}, nil
}

// Env returns the environment variables for pg_dump/pg_restore.
func (c ConnParams) Env() []string {
	env := []string{
		"PGHOST=" + c.Host,
		"PGPORT=" + c.Port,
		"PGUSER=" + c.User,
		"PGDATABASE=" + c.DBName,
		"PGSSLMODE=" + c.SSLMode,
	}
	if c.Password != "" {
		env = append(env, "PGPASSWORD="+c.Password)
	}
	return env
}

// PgTools implements CommandExecutor using real pg_dump/pg_restore binaries.
type PgTools struct {
	PgDumpPath    string
	PgRestorePath string
	warnings      string // non-fatal warnings from last pg_restore
}

// NewPgTools creates a PgTools with the given binary paths. If paths are empty,
// it looks up pg_dump and pg_restore in PATH.
func NewPgTools(pgDumpPath, pgRestorePath string) (*PgTools, error) {
	if pgDumpPath == "" {
		p, err := exec.LookPath("pg_dump")
		if err != nil {
			return nil, fmt.Errorf("backup: pg_dump not found in PATH: %w", err)
		}
		pgDumpPath = p
	}
	if pgRestorePath == "" {
		p, err := exec.LookPath("pg_restore")
		if err != nil {
			return nil, fmt.Errorf("backup: pg_restore not found in PATH: %w", err)
		}
		pgRestorePath = p
	}
	return &PgTools{PgDumpPath: pgDumpPath, PgRestorePath: pgRestorePath}, nil
}

func (t *PgTools) PgDump(ctx context.Context, conn ConnParams, outputPath string) error {
	cmd := exec.CommandContext(ctx, t.PgDumpPath, "-Fc", "-f", outputPath)
	cmd.Env = append(cmd.Environ(), conn.Env()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("backup: pg_dump failed: %w\n%s", err, string(out))
	}
	return nil
}

func (t *PgTools) PgRestore(ctx context.Context, conn ConnParams, inputPath string) error {
	cmd := exec.CommandContext(ctx, t.PgRestorePath,
		"--clean", "--if-exists",
		"-d", conn.DBName, inputPath)
	cmd.Env = append(cmd.Environ(), conn.Env()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := string(out)
		// pg_restore returns exit code 1 for non-fatal warnings (e.g.
		// version-specific SET parameters). Only treat as failure if the
		// output contains fatal errors beyond expected warnings.
		if isFatalRestoreError(output) {
			return fmt.Errorf("backup: pg_restore failed: %w\n%s", err, output)
		}
		// Non-fatal warnings only — log but don't fail
		t.warnings = output
	}
	return nil
}

// Warnings returns any non-fatal warnings from the last pg_restore run.
func (t *PgTools) Warnings() string {
	return t.warnings
}

// isFatalRestoreError checks if pg_restore output indicates a genuinely
// fatal error vs non-fatal warnings about version-specific parameters.
func isFatalRestoreError(output string) bool {
	// If output contains connection-level failures, it's truly fatal
	if strings.Contains(output, "FATAL:") ||
		strings.Contains(output, "could not connect") ||
		strings.Contains(output, "role") && strings.Contains(output, "does not exist") ||
		strings.Contains(output, "database") && strings.Contains(output, "does not exist") {
		return true
	}
	// Count actual errors vs known-harmless ones
	lines := strings.Split(output, "\n")
	fatalCount := 0
	for _, line := range lines {
		if !strings.Contains(line, "error") && !strings.Contains(line, "ERROR") {
			continue
		}
		// Known harmless: unrecognized configuration parameter (version mismatch)
		if strings.Contains(line, "unrecognized configuration parameter") {
			continue
		}
		// Known harmless: object does not exist (from --clean)
		if strings.Contains(line, "does not exist, skipping") {
			continue
		}
		// Known harmless: pg_restore summary line "errors ignored on restore: N"
		if strings.Contains(line, "errors ignored on restore") {
			continue
		}
		fatalCount++
	}
	return fatalCount > 0
}

func (t *PgTools) PgDumpVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, t.PgDumpPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("backup: pg_dump --version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (t *PgTools) PgServerVersion(ctx context.Context, conn ConnParams) (string, error) {
	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return "", fmt.Errorf("backup: psql not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, psqlPath, "-t", "-c", "SHOW server_version;")
	cmd.Env = append(cmd.Environ(), conn.Env()...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("backup: query server_version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
